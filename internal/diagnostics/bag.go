package diagnostics

import (
	"fmt"
	"sort"
)

// Bag - накопитель диагностик одного прогона компиляции (парсинг всех
// файлов программы, семантический анализ). Позволяет собрать несколько
// независимых ошибок за один прогон вместо остановки на первой (см.
// internal/parser.ParseProgramLenient - тот же принцип, здесь - его
// эквивалент для внутреннего накопления, не только для LSP).
type Bag struct {
	diags []*Diagnostic
}

// NewBag - пустой накопитель.
func NewBag() *Bag { return &Bag{} }

// Add добавляет диагностику. d == nil - молча игнорируется (удобно в
// цепочках вида bag.Add(tryParse()), где tryParse может вернуть nil при
// успехе).
func (b *Bag) Add(d *Diagnostic) {
	if d == nil {
		return
	}
	b.diags = append(b.diags, d)
}

// AddErr - как Add, но принимает произвольный error: если это уже
// *Diagnostic (напрямую или через обёртку %w - см. errors.As), добавляет
// его как есть; иначе оборачивает в Diagnostic без кода и без позиции
// (до-миграционный путь - см. package doc codes.go про RECH9000 и
// диагностики без кода).
func (b *Bag) AddErr(err error) {
	if err == nil {
		return
	}
	b.diags = append(b.diags, AsDiagnostic(err))
}

// Len - число накопленных диагностик.
func (b *Bag) Len() int { return len(b.diags) }

// Error делает *Bag самостоятельным error (не только контейнером) - место,
// откуда диагностики СОБИРАЮТСЯ (например, internal/parser.ParseProgramLenient
// через defaultFrontend.Parse - см. package doc), может вернуть *Bag прямо
// как error существующим вызывающим сторонам без изменения их сигнатуры
// (module.Frontend/compiler.Frontend остаются Parse(src string) (*ast.Program,
// error)) - им не нужно знать про Bag, если они просто пробрасывают err
// дальше или матчат err.Error() подстрокой (тесты) - тот же текст, что и у
// единственной диагностики, когда она одна. cmd/rech достаёт полный список
// через errors.As(err, &bag), чтобы напечатать все диагностики со сниппетами
// (см. RenderAll), а не только эту сводку.
func (b *Bag) Error() string {
	switch len(b.diags) {
	case 0:
		return "нет диагностик"
	case 1:
		return b.diags[0].Error()
	default:
		return fmt.Sprintf("%s (и ещё диагностик: %d)", b.diags[0].Error(), len(b.diags)-1)
	}
}

// SetFileIfEmpty проставляет Pos.File КАЖДОЙ диагностике в Bag, у которой
// он ещё не установлен - аналог diagnostics.WithFile для случая, когда err
// внутри себя несёт не одну диагностику, а целый Bag (см. WithFile).
func (b *Bag) SetFileIfEmpty(file string) {
	for _, d := range b.diags {
		if d.Pos.File == "" {
			d.Pos.File = file
		}
	}
}

// Diagnostics возвращает накопленные диагностики, отсортированные по
// (File, Line, Col) - детерминированный порядок вывода независимо от
// порядка добавления (порядок добавления обычно совпадает с порядком по
// файлу, но collectEntities/collectFuncSigs и т.п. проходят по картам
// Go - порядок карт не детерминирован, см. их собственную сортировку по
// имени, но это не гарантирует сортировку ПОЗИЦИЙ ошибок).
func (b *Bag) Diagnostics() []*Diagnostic {
	out := make([]*Diagnostic, len(b.diags))
	copy(out, b.diags)
	sort.SliceStable(out, func(i, j int) bool {
		a, c := out[i].Pos, out[j].Pos
		if a.File != c.File {
			return a.File < c.File
		}
		if a.Line != c.Line {
			return a.Line < c.Line
		}
		return a.Col < c.Col
	})
	return out
}

// Errs возвращает диагностики как []error - для мест, ещё ожидающих
// стандартный интерфейс error (см. package doc про постепенную миграцию
// вызывающих сторон).
func (b *Bag) Errs() []error {
	if len(b.diags) == 0 {
		return nil
	}
	out := make([]error, len(b.diags))
	for i, d := range b.diags {
		out[i] = d
	}
	return out
}

// AsError возвращает Bag как error - nil, если диагностик нет вообще (Bag
// пуст - "ошибок не было", а не "одна ошибка с пустым текстом"); саму
// *Diagnostic напрямую (без обёртки Bag), если она ровно одна - тот же
// объект, что видели бы вызывающие стороны ДО появления Bag (errors.As
// работает без изменений); Bag как есть, если их несколько (cmd/rech
// достаёт полный список через errors.As(err, &bag), см. package doc
// Bag.Error).
func (b *Bag) AsError() error {
	switch len(b.diags) {
	case 0:
		return nil
	case 1:
		return b.diags[0]
	default:
		return b
	}
}
