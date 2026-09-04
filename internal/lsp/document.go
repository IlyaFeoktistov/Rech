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
//
// entities/funcs/astEntities/astFuncs/astInterfaces - карты по имени, ПОД
// КОТОРЫМ вещь видна ИМЕННО В ЭТОМ файле (свои объявления - под их
// собственным именем; импортированные через "использовать" - под алиасом,
// как он написан в самом импорте) - многофайловое обогащение (multifile.go)
// добавляет туда же сущности/функции/интерфейсы ДРУГИХ файлов графа
// импортов, найденные на диске, без замены уже готового однофайлового
// результата в случае неудачи (нет "использовать", файл не читается, цикл
// импорта, ошибка семантики где-то в графе - см. tryLink). astEntities[name].
// File/astFuncs[name].File - путь исходного файла (ast.StampFile,
// см. module.Load) - "" или d.uri/d.path у своих, настоящий путь на диске у
// импортированных - см. isCurrentFile/declPosInFile.
type doc struct {
	uri  string
	text string
	// path - файловый путь, восстановленный из uri (uriToPath,
	// multifile.go) - "" для НЕ "file://" URI (в частности, тестовые
	// "test://..." по всему пакету) - многофайловое обогащение в этом
	// случае просто не пытается ничего, доступно только само дерево
	// текущего документа, как было до этой фичи.
	path string

	goodText string
	astProg  *ast.Program
	irProg   *ir.Program
	tokens   []lexer.Token
	scopes   []funcScope

	entities      map[string]*ir.Entity
	funcs         map[string]*ir.Function
	astEntities   map[string]*ast.EntityDecl
	astFuncs      map[string]*ast.FunctionDecl
	astInterfaces map[string]*ast.InterfaceDecl

	// displayName - обратное к алиасам выше: смангленное (module.Program.
	// Link) имя сущности/функции ИЗ ДРУГОГО файла -> то самое имя (алиас),
	// каким его видит и пишет ЭТОТ файл - у смангленного имени в реальном
	// исходнике Rech попросту нет буквального соответствия, показывать его
	// как есть в hover/completion (см. disp/dispType, display.go) было бы
	// неверно - пользователь никогда не писал "Логгер_ФайловыйЛоггер".
	// Заполняется только той же многофайловой линковкой, что и entities/
	// funcs/astEntities выше (applyLink) - пуст (nil), если открытый
	// документ разобран однофайлово, disp/dispType в этом случае - тождество.
	displayName map[string]string

	// localNames - имена, реально ПРЕДЪЯВИМЫЕ пользователю этого файла для
	// автодополнения (generalCompletion, completion.go) - собственные
	// объявления ЭТОГО файла плюс алиасы его собственных импортов. В
	// отличие от entities/funcs выше (которые ради resolveChain/
	// implementation.go обязаны нести ЕЩЁ и смангленные внутренние ключи
	// для сущностей из ДРУГИХ файлов), localNames НЕ содержит их вовсе -
	// показать "Логгер_ФайловыйЛоггер" в списке автодополнения было бы
	// неверно так же, как и в hover (см. display.go): пользователь никогда
	// не пишет это имя буквально, только "ФайловыйЛоггер" - под тем самым
	// алиасом, каким он импортировал его сам.
	localNames map[string]bool

	// foreignTokenCache - токены файлов ДРУГИХ документов графа импортов,
	// прочитанные с диска по требованию (declPosInFile) - копится, пока жив
	// doc, сбрасывается вместе с ним при следующем update() ниже (тот же
	// принцип, что и у остального "последнего удачного" состояния - не
	// обязан быть точным между двумя didChange, только не течь между
	// СОВСЕМ разными документами).
	foreignTokenCache map[string][]lexer.Token

	// diagnostic - ошибка ПОСЛЕДНЕГО update(), превращённая в Diagnostic
	// (diagnostics.go) - nil, если разбор+анализ прошёл успешно (см. package
	// doc update). server.go публикует её через "textDocument/
	// publishDiagnostics" после каждого didOpen/didChange - пустой список,
	// когда diagnostic стал nil, снимает раньше показанное подчёркивание.
	diagnostic *Diagnostic
}

// update пересобирает документ по новому тексту - при ошибке разбора старое
// "последнее удачное" состояние остаётся как было (см. package doc), text
// (текущий, возможно кривой буфер) обновляется всегда. Семантический анализ
// ОДНОГО этого файла (irProg/semErr ниже) может не пройти именно потому,
// что файл ссылается на другой ("использовать") - тогда решает
// многофайловая линковка (tryLink, см. multifile.go): она одна может
// провалиться сама по себе (нет "использовать" вообще, файл не читается,
// цикл импорта, ошибка семантики где-то в графе), тогда сохраняется
// однофайловый результат, как и раньше - НИ ОДИН путь не сработал только
// если оба провалились, и тогда update целиком ничего не меняет (та же
// гарантия "не хуже, чем было", что и раньше у одного semErr).
func (d *doc) update(text string) {
	d.text = text
	toks, err := lexer.New(text).Tokenize()
	if err != nil {
		d.setDiagnostic(err)
		return
	}
	astProg, err := parser.New(toks).ParseProgram()
	if err != nil {
		d.setDiagnostic(err)
		return
	}
	ast.StampFile(astProg, d.uri)
	// astProg/tokens - валидны сами по себе (синтаксис уже прошёл), даже
	// если семантика/линковка ниже провалятся - codeActions (автоимпорт,
	// codeaction.go - нужен d.astProg.Uses, чтобы найти место вставки) и
	// прочий позиционный код (tokenIndexAt и т. п.) должны видеть их и в
	// этом случае, не только при полностью успешном разборе.
	d.astProg = astProg
	d.tokens = toks
	d.foreignTokenCache = map[string][]lexer.Token{}

	irProg, semErr := semantic.Analyze(astProg)
	linked, linkOK, linkErr := tryLink(d.path)
	if semErr != nil && !linkOK {
		// linkErr, если линковка вообще пыталась (см. package doc tryLink) -
		// обычно информативнее одиночного semErr одного файла.
		if linkErr != nil {
			d.setDiagnostic(linkErr)
		} else {
			d.setDiagnostic(semErr)
		}
		return
	}
	d.diagnostic = nil
	d.goodText = text

	if linkOK {
		d.applyLink(linked)
	} else {
		d.displayName = nil
		d.irProg = irProg
		d.entities = map[string]*ir.Entity{}
		for _, e := range irProg.Entities {
			d.entities[e.Name] = e
		}
		d.funcs = map[string]*ir.Function{}
		for _, f := range irProg.Functions {
			d.funcs[f.Name] = f
		}
		d.astEntities = map[string]*ast.EntityDecl{}
		for _, e := range astProg.Entities {
			d.astEntities[e.Name] = e
		}
		d.astFuncs = map[string]*ast.FunctionDecl{}
		for _, f := range astProg.Functions {
			d.astFuncs[f.Name] = f
		}
		d.astInterfaces = map[string]*ast.InterfaceDecl{}
		for _, iface := range astProg.Interfaces {
			d.astInterfaces[iface.Name] = iface
		}
		d.localNames = map[string]bool{}
		for _, e := range astProg.Entities {
			d.localNames[e.Name] = true
		}
		for _, f := range astProg.Functions {
			d.localNames[f.Name] = true
		}
	}
	d.scopes = buildScopes(astProg, d.irProg)
}

// newDoc строит документ сразу (didOpen получает готовый текст, не пустой,
// который потом "обновляется" - но update делает ровно то же самое, что
// нужно и в момент открытия).
func newDoc(uri, text string) *doc {
	d := &doc{uri: uri, path: uriToPath(uri)}
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
