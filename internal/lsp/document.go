package lsp

import (
	"sort"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/lexer"
	"github.com/IlyaFeoktistov/Rech/internal/parser"
	"github.com/IlyaFeoktistov/Rech/internal/semantic"
)

// funcScope - функция/метод, найденные в документе (см. buildScopes) -
// объединяет то, что для них известно из AST (declLine - позиция самого
// объявления, для определения "курсор внутри этой функции") и из IR (тело/
// параметры уже с разрешёнными типами - вместо повторного разрешения имён
// типов вручную).
type funcScope struct {
	declLine   int
	name       string // отображаемое имя - "функция" или "Тип.метод"
	entityName string // "" у свободной функции
	params     []ir.Param
	body       []ir.Stmt
	hasReturn  bool
	returnType ir.Type
	doc        string
}

// doc - состояние одного открытого документа. "Последний удачный" разбор
// (goodText/prog/ir/tokens/scopes) хранится ОТДЕЛЬНО от text (текущего
// содержимого буфера VS Code) - пока пользователь набирает код, текст часто
// синтаксически неполон; hover/completion работают по последней версии,
// которая успешно прошла разбор+анализ целиком, а не падают/показывают
// пусто на каждое такое промежуточное состояние.
type doc struct {
	uri  string
	text string

	goodText string
	astProg  *ast.Program
	irProg   *ir.Program
	tokens   []lexer.Token
	scopes   []funcScope

	entities map[string]*ir.Entity
	funcs    map[string]*ir.Function
}

// update пересобирает документ по новому тексту - при ошибке разбора/
// анализа старое "последнее удачное" состояние остаётся как было (см.
// package doc), text (текущий, возможно кривой буфер) обновляется всегда.
func (d *doc) update(text string) {
	d.text = text
	toks, err := lexer.New(text).Tokenize()
	if err != nil {
		return
	}
	astProg, err := parser.New(toks).ParseProgram()
	if err != nil {
		return
	}
	ast.StampFile(astProg, d.uri)
	irProg, err := semantic.Analyze(astProg)
	if err != nil {
		return
	}
	d.goodText = text
	d.astProg = astProg
	d.irProg = irProg
	d.tokens = toks
	d.entities = map[string]*ir.Entity{}
	for _, e := range irProg.Entities {
		d.entities[e.Name] = e
	}
	d.funcs = map[string]*ir.Function{}
	for _, f := range irProg.Functions {
		d.funcs[f.Name] = f
	}
	d.scopes = buildScopes(astProg, irProg)
}

// newDoc строит документ сразу (didOpen получает готовый текст, не пустой,
// который потом "обновляется" - но update делает ровно то же самое, что
// нужно и в момент открытия).
func newDoc(uri, text string) *doc {
	d := &doc{uri: uri}
	d.update(text)
	return d
}

// buildScopes сопоставляет объявления функций/методов AST (declLine) с их
// же представлением в IR (тело/параметры уже разрешены) по имени -
// генерик-шаблоны (у них нет собственного ir.Function/ir.Method, только у
// инстанциаций, см. internal/semantic/generics.go) сюда не попадают -
// hover/completion внутри тела generic-шаблона в этом срезе не работает,
// осознанное ограничение.
func buildScopes(astProg *ast.Program, irProg *ir.Program) []funcScope {
	irFuncs := map[string]*ir.Function{}
	for _, f := range irProg.Functions {
		irFuncs[f.Name] = f
	}
	irEntities := map[string]*ir.Entity{}
	for _, e := range irProg.Entities {
		irEntities[e.Name] = e
	}

	var out []funcScope
	for _, fd := range astProg.Functions {
		f, ok := irFuncs[fd.Name]
		if !ok {
			continue
		}
		out = append(out, funcScope{
			declLine: fd.Line, name: fd.Name,
			params: f.Params, body: f.Body, hasReturn: f.HasReturn, returnType: f.ReturnType, doc: f.Doc,
		})
	}
	for _, ed := range astProg.Entities {
		e, ok := irEntities[ed.Name]
		if !ok {
			continue
		}
		for _, md := range ed.Methods {
			var m *ir.Method
			for _, mm := range e.Methods {
				if mm.Name == md.Name {
					m = mm
					break
				}
			}
			if m == nil {
				continue
			}
			out = append(out, funcScope{
				declLine: md.Line, name: ed.Name + "." + md.Name, entityName: ed.Name,
				params: m.Params, body: m.Body, hasReturn: m.HasReturn, returnType: m.ReturnType, doc: m.Doc,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].declLine < out[j].declLine })
	return out
}

// scopeAt находит funcScope, содержащий line - ближайшее ПРЕДШЕСТВУЮЩЕЕ
// объявление функции/метода (нет точных границ конца тела - см. package
// doc buildScopes; для однофайловых непересекающихся тел этого достаточно).
func (d *doc) scopeAt(line int) *funcScope {
	var best *funcScope
	for i := range d.scopes {
		if d.scopes[i].declLine <= line && (best == nil || d.scopes[i].declLine > best.declLine) {
			best = &d.scopes[i]
		}
	}
	return best
}

// localsAt - параметры функции/метода scope плюс переменные, объявленные в
// её теле НЕ ПОЗЖЕ line (обход рекурсивный, по всем вложенным блокам без
// разбора настоящей границы блока - см. package doc collectVarDecls).
func localsAt(scope *funcScope, line int) map[string]ir.Type {
	out := map[string]ir.Type{}
	if scope == nil {
		return out
	}
	for _, p := range scope.params {
		out[p.Name] = p.Type
	}
	collectVarDecls(scope.body, line, out)
	return out
}

// collectVarDecls - см. localsAt. Затенение (переменная objявлена дважды
// в разных ветках "если") не моделируется отдельно - последняя увиденная
// побеждает, для подсказки типа при hover/completion этого достаточно.
func collectVarDecls(stmts []ir.Stmt, maxLine int, out map[string]ir.Type) {
	for _, s := range stmts {
		switch s := s.(type) {
		case *ir.VarDecl:
			if s.Line <= maxLine {
				out[s.Name] = ir.TypeOf(s.Value)
			}
		case *ir.IfStmt:
			collectVarDecls(s.Then, maxLine, out)
			collectVarDecls(s.Else, maxLine, out)
		case *ir.WhileStmt:
			collectVarDecls(s.Body, maxLine, out)
		case *ir.ForStmt:
			out[s.Var] = ir.TypeInt
			collectVarDecls(s.Body, maxLine, out)
		case *ir.ForEachStmt:
			out[s.Var] = s.ElemType
			collectVarDecls(s.Body, maxLine, out)
		case *ir.TryStmt:
			collectVarDecls(s.Body, maxLine, out)
			out[s.CatchVar] = s.CatchVarType
			collectVarDecls(s.CatchBody, maxLine, out)
		case *ir.GerundStmt:
			collectVarDecls(s.Body, maxLine, out)
		case *ir.BlockStmt:
			collectVarDecls(s.Body, maxLine, out)
		}
	}
}
