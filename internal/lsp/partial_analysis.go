package lsp

import (
	"sort"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/diagnostics"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/lexer"
)

// maxSalvageRounds - сколько раз analyzeSalvaging повторяет анализ, каждый
// раз отбрасывая инструкции/объявления с ошибками. Каждый раунд убирает
// хотя бы одну, так что предел нужен только против патологически длинных
// каскадов. Первая половина раундов убирает отдельные инструкции тел,
// вторая - тела целиком (fineSalvageRounds): каскад внутри одного тела
// (сломанное объявление переменной - ошибки на каждом её использовании)
// сходится за один раунд.
const (
	maxSalvageRounds  = 32
	fineSalvageRounds = maxSalvageRounds / 2
)

// declSpan - начало одного объявления файла или инструкции верхнего уровня
// тела (строка) и способ выключить его из анализа: stub - оставить
// сигнатуру, выкинуть тело (функция/метод/конструктор); remove - убрать
// объявление/инструкцию целиком. У объявлений без тела и у инструкций
// stub == nil, сразу remove.
type declSpan struct {
	line   int
	stub   func()
	remove func()
	// name - имя, которое объявление/инструкция вводит ("" - никакого):
	// после remove ошибки, называющие его, - следствие удаления, а не
	// самостоятельные (см. analyzeSalvaging).
	name string
}

// analyzeSalvaging - семантический анализ, переживающий ошибки: семантика
// сама по себе либо проходит целиком, либо нет, поэтому при ошибке
// инструкция тела, в которой она находится, убирается (ошибка в сигнатуре
// - объявление: сначала только тело, при повторной ошибке в том же месте -
// целиком), и анализ повторяется. Результат - IR всего остального файла:
// hover/completion/definition по исправным инструкциям и объявлениям
// работают, пока в другом месте файла есть ошибка. analyze - без проверок
// потока управления (их ошибки уже показал полный анализ, а выкинутое тело
// функции с типом результата иначе само стало бы ошибкой "нет вернуть").
// prog не меняется - анализируется поверхностная копия.
//
// later - ошибки, которые нашлись только в повторных раундах (семантика
// останавливается на первой ошибке фазы - например, ошибки сигнатур не
// видны, пока есть ошибка сущности): самостоятельные ошибки, кроме
// следствий самого спасения - упоминающих в кавычках имя убранного
// объявления или переменной (неизвестный тип "Точка" после удаления
// сущности Точка). lines - текст документа для уточнения столбцов.
func analyzeSalvaging(prog *ast.Program, analyze func(*ast.Program) (*ir.Program, error), lines []string) (irProg *ir.Program, ok bool, later []Diagnostic) {
	work := cloneProgramShallow(prog)
	stubbed := map[int]bool{}
	removed := map[string]bool{}
	for round := range maxSalvageRounds {
		irProg, err := analyze(work)
		if err == nil {
			return irProg, true, later
		}
		spans := declSpans(work, round < fineSalvageRounds)
		progress := false
		for _, dg := range semanticDiagnosticsFor(err, lines) {
			if round > 0 && !mentionsAny(dg.Message, removed) {
				later = append(later, dg)
			}
			s, ok := spanAt(spans, dg.Line)
			if !ok {
				continue
			}
			if s.stub != nil && !stubbed[s.line] {
				stubbed[s.line] = true
				s.stub()
			} else {
				s.remove()
				if s.name != "" {
					removed[s.name] = true
				}
			}
			progress = true
		}
		if !progress {
			return nil, false, later
		}
	}
	return nil, false, later
}

// mentionsAny - msg называет в кавычках одно из names.
func mentionsAny(msg string, names map[string]bool) bool {
	for name := range names {
		if strings.Contains(msg, "\""+name+"\"") {
			return true
		}
	}
	return false
}

// spanAt - объявление, внутри которого находится строка line: ближайшее
// начало объявления не позже line. Метод сущности - отдельный span со своей
// строкой, поэтому ошибка в его теле попадает в него, а не в сущность.
func spanAt(spans []declSpan, line int) (declSpan, bool) {
	i := sort.Search(len(spans), func(i int) bool { return spans[i].line > line }) - 1
	if i < 0 {
		return declSpan{}, false
	}
	return spans[i], true
}

// cloneProgramShallow копирует срезы программы и сущности (их Methods/
// Ctor меняет stub/remove) - сами узлы тел не копируются, они только
// отбрасываются, не правятся.
func cloneProgramShallow(p *ast.Program) *ast.Program {
	c := *p
	c.Functions = make([]*ast.FunctionDecl, len(p.Functions))
	for i, f := range p.Functions {
		fc := *f
		c.Functions[i] = &fc
	}
	c.Entities = make([]*ast.EntityDecl, len(p.Entities))
	for i, e := range p.Entities {
		ec := *e
		ec.Methods = make([]*ast.MethodDecl, len(e.Methods))
		for j, m := range e.Methods {
			mc := *m
			ec.Methods[j] = &mc
		}
		if e.Ctor != nil {
			ctor := *e.Ctor
			ec.Ctor = &ctor
		}
		c.Entities[i] = &ec
	}
	c.Interfaces = append([]*ast.InterfaceDecl(nil), p.Interfaces...)
	c.Enums = append([]*ast.EnumDecl(nil), p.Enums...)
	c.Consts = append([]*ast.ConstDecl(nil), p.Consts...)
	c.ExternFuncs = append([]*ast.ExternFuncDecl(nil), p.ExternFuncs...)
	c.ExternEntities = append([]*ast.ExternEntityDecl(nil), p.ExternEntities...)
	return &c
}

// declSpans - все объявления work, отсортированные по строке начала, а с
// stmts - ещё и инструкции верхнего уровня их тел (stmtSpans).
func declSpans(work *ast.Program, stmts bool) []declSpan {
	var spans []declSpan
	for _, f := range work.Functions {
		spans = append(spans, declSpan{
			line:   f.Line,
			stub:   func() { f.Body = nil },
			remove: func() { work.Functions = removeItem(work.Functions, f) },
			name:   f.Name,
		})
		if stmts {
			spans = stmtSpans(spans, &f.Body)
		}
	}
	for _, e := range work.Entities {
		spans = append(spans, declSpan{
			line:   e.Line,
			remove: func() { work.Entities = removeItem(work.Entities, e) },
			name:   e.Name,
		})
		for _, m := range e.Methods {
			spans = append(spans, declSpan{
				line:   m.Line,
				stub:   func() { m.Body = nil },
				remove: func() { e.Methods = removeItem(e.Methods, m) },
				name:   m.Name,
			})
			if stmts {
				spans = stmtSpans(spans, &m.Body)
			}
		}
		if ctor := e.Ctor; ctor != nil {
			spans = append(spans, declSpan{
				line:   ctor.Line,
				stub:   func() { ctor.Body = nil },
				remove: func() { work.Entities = removeItem(work.Entities, e) },
				name:   e.Name,
			})
			if stmts {
				spans = stmtSpans(spans, &ctor.Body)
			}
		}
	}
	for _, x := range work.Interfaces {
		spans = append(spans, declSpan{line: x.Line, remove: func() { work.Interfaces = removeItem(work.Interfaces, x) }, name: x.Name})
	}
	for _, x := range work.Enums {
		spans = append(spans, declSpan{line: x.Line, remove: func() { work.Enums = removeItem(work.Enums, x) }, name: x.Name})
	}
	for _, x := range work.Consts {
		spans = append(spans, declSpan{line: x.Line, remove: func() { work.Consts = removeItem(work.Consts, x) }, name: x.Name})
	}
	for _, x := range work.ExternFuncs {
		spans = append(spans, declSpan{line: x.Line, remove: func() { work.ExternFuncs = removeItem(work.ExternFuncs, x) }, name: x.Alias})
	}
	for _, x := range work.ExternEntities {
		spans = append(spans, declSpan{line: x.Line, remove: func() { work.ExternEntities = removeItem(work.ExternEntities, x) }, name: x.Alias})
	}
	sort.SliceStable(spans, func(i, j int) bool { return spans[i].line < spans[j].line })
	return spans
}

// stmtSpans - spans плюс по span на каждую инструкцию верхнего уровня
// *body с известной строкой: ошибка во вложенном блоке ("если"/"пока")
// убирает всю инструкцию верхнего уровня, в которую он входит.
func stmtSpans(spans []declSpan, body *[]ast.Stmt) []declSpan {
	for _, st := range *body {
		if line := astStmtLine(st); line > 0 {
			spans = append(spans, declSpan{line: line, remove: func() { *body = removeItem(*body, st) }, name: stmtDeclName(st)})
		}
	}
	return spans
}

// stmtDeclName - имя переменной/константы, которую объявляет st, иначе "".
func stmtDeclName(st ast.Stmt) string {
	switch s := st.(type) {
	case *ast.VarDecl:
		return s.Name
	case *ast.ConstDecl:
		return s.Name
	}
	return ""
}

// removeItem - s без элемента x (сравнение по указателю).
func removeItem[T comparable](s []T, x T) []T {
	out := s[:0:0]
	for _, v := range s {
		if v != x {
			out = append(out, v)
		}
	}
	return out
}

// tokenizeSalvaging - лексический разбор, переживающий ошибки: символ, на
// котором лексер споткнулся (незакрытая кавычка/комментарий, неожиданный
// символ), заменяется пробелом, при повторной ошибке на той же строке -
// вся строка, и разбор повторяется. Длина строк не меняется, поэтому
// позиции остальных токенов совпадают с исходным текстом. Ошибка без
// позиции (кодировка файла) не лечится - ok == false.
func tokenizeSalvaging(text string) ([]lexer.Token, bool) {
	lines := strings.Split(text, "\n")
	blankedLine := map[int]bool{}
	for range maxSalvageRounds {
		toks, err := lexer.New(strings.Join(lines, "\n")).Tokenize()
		if err == nil {
			return toks, true
		}
		dg := diagnostics.AsDiagnostic(err)
		if dg == nil || dg.Pos.Line < 1 || dg.Pos.Line > len(lines) {
			return nil, false
		}
		ln := dg.Pos.Line - 1
		runes := []rune(lines[ln])
		col := dg.Pos.Col - 1
		if blankedLine[ln] || col < 0 || col >= len(runes) || runes[col] == ' ' {
			lines[ln] = strings.Repeat(" ", len(runes))
		} else {
			runes[col] = ' '
			lines[ln] = string(runes)
		}
		blankedLine[ln] = true
	}
	return nil, false
}
