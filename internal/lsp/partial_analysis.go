package lsp

import (
	"sort"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/diagnostics"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/lexer"
	"github.com/IlyaFeoktistov/Rech/internal/semantic"
)

// maxSalvageRounds - сколько раз analyzeSalvaging повторяет анализ, каждый
// раз отбрасывая объявления с ошибками. Каждый раунд убирает хотя бы одно
// объявление, так что предел нужен только против патологически длинных
// каскадов.
const maxSalvageRounds = 32

// declSpan - начало одного объявления файла (строка) и способ выключить
// его из анализа: stub - оставить сигнатуру, выкинуть тело (функция/метод/
// конструктор); remove - убрать объявление целиком. У объявлений без тела
// stub == nil, сразу remove.
type declSpan struct {
	line   int
	stub   func()
	remove func()
}

// analyzeSalvaging - семантический анализ, переживающий ошибки: семантика
// сама по себе либо проходит целиком, либо нет, поэтому при ошибке
// объявление, в котором она находится, выключается (сначала - только тело,
// при повторной ошибке в том же месте - целиком), и анализ повторяется.
// Результат - IR всего остального файла: hover/completion/definition по
// исправным объявлениям работают, пока в другом месте файла есть ошибка.
// prog не меняется - анализируется поверхностная копия.
func analyzeSalvaging(prog *ast.Program) (*ir.Program, bool) {
	work := cloneProgramShallow(prog)
	stubbed := map[int]bool{}
	for range maxSalvageRounds {
		irProg, err := semantic.Analyze(work)
		if err == nil {
			return irProg, true
		}
		spans := declSpans(work)
		progress := false
		for _, dg := range semanticDiagnosticsFor(err, nil) {
			s, ok := spanAt(spans, dg.Line)
			if !ok {
				continue
			}
			if s.stub != nil && !stubbed[s.line] {
				stubbed[s.line] = true
				s.stub()
			} else {
				s.remove()
			}
			progress = true
		}
		if !progress {
			return nil, false
		}
	}
	return nil, false
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

// declSpans - все объявления work, отсортированные по строке начала.
func declSpans(work *ast.Program) []declSpan {
	var spans []declSpan
	for _, f := range work.Functions {
		spans = append(spans, declSpan{
			line:   f.Line,
			stub:   func() { f.Body = nil },
			remove: func() { work.Functions = removeItem(work.Functions, f) },
		})
	}
	for _, e := range work.Entities {
		spans = append(spans, declSpan{
			line:   e.Line,
			remove: func() { work.Entities = removeItem(work.Entities, e) },
		})
		for _, m := range e.Methods {
			spans = append(spans, declSpan{
				line:   m.Line,
				stub:   func() { m.Body = nil },
				remove: func() { e.Methods = removeItem(e.Methods, m) },
			})
		}
		if ctor := e.Ctor; ctor != nil {
			spans = append(spans, declSpan{
				line:   ctor.Line,
				stub:   func() { ctor.Body = nil },
				remove: func() { work.Entities = removeItem(work.Entities, e) },
			})
		}
	}
	for _, x := range work.Interfaces {
		spans = append(spans, declSpan{line: x.Line, remove: func() { work.Interfaces = removeItem(work.Interfaces, x) }})
	}
	for _, x := range work.Enums {
		spans = append(spans, declSpan{line: x.Line, remove: func() { work.Enums = removeItem(work.Enums, x) }})
	}
	for _, x := range work.Consts {
		spans = append(spans, declSpan{line: x.Line, remove: func() { work.Consts = removeItem(work.Consts, x) }})
	}
	for _, x := range work.ExternFuncs {
		spans = append(spans, declSpan{line: x.Line, remove: func() { work.ExternFuncs = removeItem(work.ExternFuncs, x) }})
	}
	for _, x := range work.ExternEntities {
		spans = append(spans, declSpan{line: x.Line, remove: func() { work.ExternEntities = removeItem(work.ExternEntities, x) }})
	}
	sort.SliceStable(spans, func(i, j int) bool { return spans[i].line < spans[j].line })
	return spans
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
