// "textDocument/signatureHelp" - подсказка текущего параметра при наборе
// вызова функции/метода/конструктора ("f(1, |" -> сигнатура f, второй
// параметр подсвечен). Работает над теми же d.tokens/d.entities/d.funcs,
// что hover.go/completion.go - без собственного разбора вызова через
// ast/ir, только сдвиг по уже готовым токенам (та же граница "однофайловый
// документ", что и у остального пакета).
package lsp

import (
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/builtins"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/lexer"
)

// SignatureHelp - один найденный вызов под курсором - ParamLabels[i] уже
// отформатирован ("имя: Тип" для пользовательских функций/методов/
// конструкторов, голый "Тип" для встроенных - internal/builtins.Func не
// несёт имён параметров), ActiveParam - индекс подсвечиваемого (может выйти
// за длину ParamLabels, если пользователь напечатал больше аргументов, чем
// объявлено - сервер сам не клэмпит, это забота клиента).
type SignatureHelp struct {
	Label         string
	ParamLabels   []string
	ActiveParam   int
	Documentation string
}

// signatureHelp ищет вызов, В КОТОРОМ находится курсор (line, col, оба
// 1-индексные), и строит его сигнатуру - false, если курсор не внутри
// круглых скобок вызова вообще (обычный текст/группирующие скобки без
// имени перед ними).
func (d *doc) signatureHelp(line, col int) (SignatureHelp, bool) {
	if d.irProg == nil {
		return SignatureHelp{}, false
	}
	openIdx, ok := d.enclosingCallOpenParen(line, col)
	if !ok {
		return SignatureHelp{}, false
	}
	sig, ok := d.callSignatureAt(openIdx, line)
	if !ok {
		return SignatureHelp{}, false
	}
	sig.ActiveParam = d.activeParamAt(openIdx, line, col)
	return sig, true
}

// enclosingCallOpenParen ищет индекс токена LPAREN, ближайшего "открытого"
// на момент курсора - идёт назад от последнего токена ДО (line, col)
// включительно, считая глубину скобок (и круглых, и квадратных - список
// литерала внутри аргумента вызова не должен закрыть его раньше времени,
// см. package doc). "", false - такого LPAREN нет (курсор не внутри вызова)
// либо он не непосредственно после IDENT/KwThis/точки цепочки (обычная
// группирующая скобка "(1 + 2)", не вызов).
func (d *doc) enclosingCallOpenParen(line, col int) (int, bool) {
	start := d.lastTokenAtOrBefore(line, col)
	if start < 0 {
		return -1, false
	}
	depth := 0
	for i := start; i >= 0; i-- {
		switch d.tokens[i].Type {
		case lexer.RPAREN, lexer.RBRACKET:
			depth++
		case lexer.LBRACKET:
			if depth > 0 {
				depth--
			}
		case lexer.LPAREN:
			if depth == 0 {
				if i == 0 {
					return -1, false
				}
				prev := d.tokens[i-1].Type
				if prev != lexer.IDENT && prev != lexer.KwThis {
					return -1, false
				}
				return i, true
			}
			depth--
		}
	}
	return -1, false
}

// lastTokenAtOrBefore - индекс последнего токена d.tokens, начинающегося
// СТРОГО раньше (line, col) (оба 1-индексные) - -1, если курсор раньше
// первого токена документа. Токен, начинающийся РОВНО в (line, col),
// намеренно исключён - курсор стоит непосредственно ПЕРЕД ним (ещё не
// "прошёл" его), тем же граничным условием, что и activeParamAt ниже:
// иначе курсор, поставленный сразу после последнего введённого аргумента
// (перед ещё не напечатанной закрывающей скобкой), при следующем токене,
// СЛУЧАЙНО начинающемся ровно в этой позиции (например, RPAREN уже
// существующего в буфере вызова), принял бы эту закрывающую скобку за уже
// "пройденную" и решил бы, что вызов закрыт раньше времени.
func (d *doc) lastTokenAtOrBefore(line, col int) int {
	idx := -1
	for i, tok := range d.tokens {
		if tok.Line > line || (tok.Line == line && tok.Col >= col) {
			break
		}
		idx = i
	}
	return idx
}

// activeParamAt считает запятые на "верхнем уровне" этого вызова (глубина 0
// относительно openIdx - вложенные скобки/списки не считаются) от openIdx+1
// до курсора - индекс параметра, на котором сейчас стоит курсор.
func (d *doc) activeParamAt(openIdx, line, col int) int {
	depth := 0
	active := 0
	for i := openIdx + 1; i < len(d.tokens); i++ {
		tok := d.tokens[i]
		if tok.Line > line || (tok.Line == line && tok.Col >= col) {
			break
		}
		switch tok.Type {
		case lexer.LPAREN, lexer.LBRACKET:
			depth++
		case lexer.RPAREN, lexer.RBRACKET:
			if depth == 0 {
				return active
			}
			depth--
		case lexer.COMMA:
			if depth == 0 {
				active++
			}
		}
	}
	return active
}

// callSignatureAt разбирает, ЧТО вызывается перед LPAREN с индексом
// openIdx - конструктор ("новый Тип(...)"), метод сущности
// ("receiver.метод(...)") или обычная функция (объявленная в документе или
// встроенная).
func (d *doc) callSignatureAt(openIdx, line int) (SignatureHelp, bool) {
	nameEnd := openIdx - 1
	chain := d.chainBefore(nameEnd)
	if len(chain) == 0 {
		return SignatureHelp{}, false
	}
	chainStart := nameEnd - 2*(len(chain)-1)
	if len(chain) == 1 && chainStart-1 >= 0 && d.tokens[chainStart-1].Type == lexer.KwNew {
		ent := d.entities[chain[0]]
		if ent == nil {
			return SignatureHelp{}, false
		}
		params, ok := ctorParamList(ent)
		if !ok {
			return SignatureHelp{}, false
		}
		entName := d.disp(ent.Name)
		name := entityCtorKeyword(entName) + " " + entName
		labels := appendParamDefaults(d.paramLabels(params), d.ctorParamDefaults(ent))
		return SignatureHelp{Label: d.functionSignatureFromLabels(name, labels, false, ir.Type{}), ParamLabels: labels}, true
	}
	if len(chain) > 1 {
		container, ok := d.resolveChain(chain[:len(chain)-1], line)
		if !ok || container.Kind != ir.KindEntity {
			return SignatureHelp{}, false
		}
		ent := d.entities[container.Entity]
		if ent == nil {
			// Встроенная generic-коллекция (Список<T> и т. п.) - см. hover.go/
			// genericcollection.go про то же самое ограничение.
			methods, ok := collectionMethodsOf(container.Entity)
			if !ok {
				return SignatureHelp{}, false
			}
			for _, m := range methods {
				if m.Name == chain[len(chain)-1] {
					return SignatureHelp{Label: container.Entity + "." + collectionMethodSignature(m.Name, m), ParamLabels: m.Params}, true
				}
			}
			return SignatureHelp{}, false
		}
		methodName := chain[len(chain)-1]
		for _, m := range ent.Methods {
			if m.Name != methodName {
				continue
			}
			name := d.disp(ent.Name) + "." + m.Name
			labels := appendParamDefaults(d.paramLabels(m.Params), d.methodParamDefaultsAST(ent.Name, methodName))
			return SignatureHelp{
				Label:         d.functionSignatureFromLabels(name, labels, m.HasReturn, m.ReturnType),
				ParamLabels:   labels,
				Documentation: strings.TrimSpace(d.resolvedMethodDoc(ent, methodName, m.Doc)),
			}, true
		}
		return SignatureHelp{}, false
	}
	name := chain[0]
	if fn, ok := d.funcs[name]; ok {
		labels := appendParamDefaults(d.paramLabels(fn.Params), d.funcParamDefaultsAST(fn.Name))
		return SignatureHelp{
			Label:         d.functionSignatureFromLabels(d.disp(fn.Name), labels, fn.HasReturn, fn.ReturnType),
			ParamLabels:   labels,
			Documentation: strings.TrimSpace(fn.Doc),
		}, true
	}
	if f, ok := builtins.ByName(name); ok {
		labels := make([]string, len(f.Params))
		for i, k := range f.Params {
			labels[i] = kindRechName(k)
		}
		return SignatureHelp{
			Label:         builtinFuncSignature(f),
			ParamLabels:   labels,
			Documentation: builtinFuncShortDoc[name],
		}, true
	}
	return SignatureHelp{}, false
}

// paramLabels - "имя: Тип" на каждый параметр, тем же форматом, что и
// d.functionSignature строит для полной строки сигнатуры.
func (d *doc) paramLabels(params []ir.Param) []string {
	labels := make([]string, len(params))
	for i, p := range params {
		labels[i] = p.Name + ": " + d.dispType(p.Type)
	}
	return labels
}

// appendParamDefaults добавляет " = <defaults[i]>" к каждому labels[i], для
// которого defaults[i] - непустая строка (уже готовый текст литерала - см.
// ctorParamDefaults/methodParamDefaultsAST/funcParamDefaultsAST, у каждого
// свой источник). labels мутируется на месте и возвращается для удобства
// вызывающей стороны - у неё уже нет других владельцев на этот момент
// (свежепостроенный d.paramLabels).
func appendParamDefaults(labels []string, defaults []string) []string {
	for i := range labels {
		if i >= len(defaults) || defaults[i] == "" {
			continue
		}
		labels[i] += " = " + defaults[i]
	}
	return labels
}

// ctorParamDefaults - текст default-значений параметров конструктора ent,
// ПОЗИЦИОННО совпадающий с ctorParamList(ent) - та же пара источников: явный
// конструктор пользователя несёт default ТОЛЬКО в AST
// (ast.ConstructorDecl.Params[i].Default, ir.Param, в отличие от ir.Field,
// вообще не несёт default-выражение) - оттуда, через literalTextAST;
// неявный позиционный конструктор структуры - НАПРЯМУЮ из
// ent.Fields[i].Default (ir.Expr, уже разрешённое семантикой, через
// literalText) - этот путь работает одинаково для пользовательских структур
// И для встроенных синтетических сущностей вроде "Запрос"/"Ответ"
// (internal/semantic/types.go, httpRequestDefaults) - у тех вообще нет
// ast.EntityDecl, только ir.Entity, поэтому AST-путь для них был бы всегда
// пуст.
func (d *doc) ctorParamDefaults(ent *ir.Entity) []string {
	if ent.Ctor != nil {
		ed, ok := d.astEntities[ent.Name]
		if !ok || ed.Ctor == nil {
			return nil
		}
		out := make([]string, len(ed.Ctor.Params))
		for i, p := range ed.Ctor.Params {
			if p.Default == nil {
				continue
			}
			if lit, _, ok := literalTextAST(p.Default); ok {
				out[i] = lit
			}
		}
		return out
	}
	out := make([]string, len(ent.Fields))
	for i, f := range ent.Fields {
		if f.Default == nil {
			continue
		}
		if lit, ok := literalText(f.Default); ok {
			out[i] = lit
		}
	}
	return out
}

// methodParamDefaultsAST - текст default-значений параметров метода
// methodName сущности entName - только AST (ast.MethodDecl.Params[i].Default),
// методы синтетических сущностей (Запрос/Ответ и т. п.) не существуют вовсе
// (те - структуры данных без методов), поэтому, в отличие от
// ctorParamDefaults, второй источник не нужен.
func (d *doc) methodParamDefaultsAST(entName, methodName string) []string {
	ed, ok := d.astEntities[entName]
	if !ok {
		return nil
	}
	for _, m := range ed.Methods {
		if m.Name != methodName {
			continue
		}
		out := make([]string, len(m.Params))
		for i, p := range m.Params {
			if p.Default == nil {
				continue
			}
			if lit, _, ok := literalTextAST(p.Default); ok {
				out[i] = lit
			}
		}
		return out
	}
	return nil
}

// funcParamDefaultsAST - как methodParamDefaultsAST, но для свободной
// функции name (тоже только AST - встроенные свободные функции идут через
// internal/builtins, у которых параметры вообще без имён/default, отдельная
// ветка callSignatureAt, сюда не попадают).
func (d *doc) funcParamDefaultsAST(name string) []string {
	fd, ok := d.astFuncs[name]
	if !ok {
		return nil
	}
	out := make([]string, len(fd.Params))
	for i, p := range fd.Params {
		if p.Default == nil {
			continue
		}
		if lit, _, ok := literalTextAST(p.Default); ok {
			out[i] = lit
		}
	}
	return out
}
