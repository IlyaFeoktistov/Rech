package lsp

import (
	"sort"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/lexer"
	"github.com/IlyaFeoktistov/Rech/internal/morph"
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
	// throws/throwsProg - ленивый индекс "что бросает" (throws.go) и IR,
	// по которому он построен: новый irProg - индекс строится заново.
	throws     *throwsIndex
	throwsProg *ir.Program
	// forCompletion - документ-копия для автодополнения (completionDoc):
	// анализ без проверок потока управления.
	forCompletion bool
	tokens        []lexer.Token
	scopes        []funcScope

	entities      map[string]*ir.Entity
	funcs         map[string]*ir.Function
	astEntities   map[string]*ast.EntityDecl
	astFuncs      map[string]*ast.FunctionDecl
	astInterfaces map[string]*ast.InterfaceDecl
	// astEnums - собственные перечисления документа (плюс импортированные
	// через multifile.go - никогда не манглятся при линковке, см. её
	// package doc, поэтому не нуждаются в алиасах, в отличие от astEntities/
	// astFuncs). Встроенные (HTTPКод/ДеньНедели/Месяц) сюда НЕ входят - они
	// не объявлены ни в одном файле, см. semantic.StandardEnums,
	// enumhover.go.
	astEnums map[string]*ast.EnumDecl

	// astConsts - константы МОДУЛЬНОГО уровня документа (плюс
	// импортированные через multifile.go - тем же принципом, что и
	// astEnums: константа никогда не манглится при линковке, см.
	// importedMergedName). Только AST, не IR - в отличие от константы
	// СУЩНОСТИ (ir.Entity.Constants, уже резолвится семантикой в типизированный
	// ir.Expr), модульная константа в IR не сохраняется вовсе (её значение
	// подставляется в места использования на этапе семантического анализа,
	// сама декларация после этого никому не нужна) - hover поэтому читает
	// AST-значение напрямую (constanthover.go, literalTextAST).
	astConsts map[string]*ast.ConstDecl

	// astSynonyms - синонимы типов документа (плюс импортированные через
	// multifile.go - не манглятся при линковке, как и astEnums). Цель в
	// SynonymDecl.Target уже раскрыта семантикой, поэтому hover берёт текст
	// объявления из токенов исходника (synonymhover.go).
	astSynonyms map[string]*ast.SynonymDecl

	// dict - морфологический словарь, накопленный ЭТИМ проходом разбора
	// (parser.Parser.Dict, тот же самый экземпляр, что разбирал astProg
	// ниже) - нужен references.go/highlight.go, чтобы опознавать
	// склонённую словоформу переменной ("паролем") как ссылку на её
	// переменную ("пароль") - тем же движком, что и падежное сопоставление
	// аргументов естественного вызова (internal/parser/call.go). Не
	// восстанавливается заново при многофайловой линковке (multifile.go) -
	// словарь нужен только для ИМЁН ЭТОГО файла, они видны в нём
	// независимо от того, слинковался ли граф импортов целиком.
	dict *morph.Dictionary

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

	// diagnostics - ошибки ПОСЛЕДНЕГО update(), превращённые в Diagnostic
	// (diagnostics.go) - пусто, если разбор+анализ прошёл полностью успешно
	// (см. package doc update). Может нести НЕСКОЛЬКО записей одновременно -
	// разбор в lenient-режиме (internal/parser, ParseProgramLenient)
	// копит ошибку на КАЖДОЕ сломанное верхнеуровневое объявление файла,
	// а не только первую, плюс, отдельным элементом в конце, ошибку
	// семантики/линковки, если она есть. server.go публикует их все через
	// "textDocument/publishDiagnostics" после каждого didOpen/didChange -
	// пустой список снимает раньше показанное подчёркивание.
	diagnostics []Diagnostic
}

// update пересобирает документ по новому тексту - text (текущий, возможно
// кривой буфер) обновляется всегда. astProg/tokens - тоже, ВСЕГДА, даже
// если lenient-разбор (internal/parser, ParseProgramLenient) нашёл ошибки
// в части объявлений файла - остальные, синтаксически исправные, разобраны
// как обычно и не должны становиться недоступны hover/definition/completion
// только потому, что где-то ещё в файле опечатка - "LSP не должен ГЛОХНУТЬ
// целиком из-за одной ошибки в файле". Единственный
// путь, где старое "последнее удачное" состояние (goodText/irProg/entities/
// funcs/scopes) остаётся как было - провал СЕМАНТИКИ/линковки: она либо
// проходит целиком, либо нет, частичного результата у неё, в отличие от
// парсера, не бывает. Семантический анализ ОДНОГО этого файла (irProg/semErr
// ниже) может не пройти именно потому, что файл ссылается на другой
// ("использовать") - тогда решает многофайловая линковка (tryLink, см.
// multifile.go): она одна может провалиться сама по себе (нет
// "использовать" вообще, файл не читается, цикл импорта, ошибка семантики
// где-то в графе), тогда сохраняется однофайловый результат, как и раньше -
// НИ ОДИН путь не сработал только если оба провалились.
func (d *doc) update(text string) {
	d.text = text
	lines := strings.Split(text, "\n")
	toks, lexErr := lexer.New(text).Tokenize()
	if lexErr != nil {
		// Ошибка лексера - диагностика по ней, а разбор дальше идёт по
		// тексту, где место ошибки забито пробелами (partial_analysis.go).
		// Диагностика - только сама ошибка лексера: ошибки разбора/семантики
		// забитого текста - её следствие, не отдельные проблемы.
		defer d.setDiagnostic(lexErr)
		salvaged, ok := tokenizeSalvaging(text)
		if !ok {
			return
		}
		toks = salvaged
	}
	prs := parser.New(toks)
	astProg, parseErrs := prs.ParseProgramLenient()
	d.dict = prs.Dict()
	ast.StampFile(astProg, d.uri)
	// astProg/tokens - весь синтаксически исправный ОСТАТОК файла, даже
	// если часть объявлений сломана (parseErrs выше) или семантика/линковка
	// ниже провалятся - codeActions (автоимпорт, codeaction.go - нужен
	// d.astProg.Uses, чтобы найти место вставки) и прочий позиционный код
	// (tokenIndexAt и т. п.) должны видеть их и в этом случае.
	d.astProg = astProg
	d.tokens = toks
	d.foreignTokenCache = map[string][]lexer.Token{}

	irProg, semErr := d.analyze(astProg)
	linked, linkOK, linkErr := tryLink(d.path)
	if semErr != nil && !linkOK {
		// linkErr, если линковка вообще пыталась (см. package doc tryLink) -
		// обычно информативнее одиночного semErr одного файла.
		if linkErr != nil {
			d.setDiagnostics(append(parseDiagnostics(parseErrs, lines), semanticDiagnosticsFor(linkErr, lines)...))
		} else {
			d.setDiagnostics(append(parseDiagnostics(parseErrs, lines), semanticDiagnosticsFor(semErr, lines)...))
		}
		// Частичный IR без объявлений с ошибками (partial_analysis.go) -
		// точнее старого "последнего удачного" состояния: строки совпадают
		// с текущим текстом, и он есть даже у файла, открытого сразу с
		// ошибкой.
		salvaged, ok := analyzeSalvaging(astProg, d.analyze)
		if !ok {
			return
		}
		irProg = salvaged
	} else {
		d.setDiagnostics(parseDiagnostics(parseErrs, lines))
	}
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
		d.astEnums = map[string]*ast.EnumDecl{}
		for _, e := range astProg.Enums {
			d.astEnums[e.Name] = e
		}
		d.astConsts = map[string]*ast.ConstDecl{}
		for _, c := range astProg.Consts {
			d.astConsts[c.Name] = c
		}
		d.astSynonyms = map[string]*ast.SynonymDecl{}
		for _, s := range astProg.Synonyms {
			d.astSynonyms[s.Name] = s
		}
		d.localNames = map[string]bool{}
		for _, e := range astProg.Entities {
			d.localNames[e.Name] = true
		}
		for _, f := range astProg.Functions {
			d.localNames[f.Name] = true
		}
		for _, iface := range astProg.Interfaces {
			d.localNames[iface.Name] = true
		}
		for _, s := range astProg.Synonyms {
			d.localNames[s.Name] = true
		}
	}
	d.scopes = buildScopes(astProg, d.irProg)
}

// newDoc строит документ сразу (didOpen получает готовый текст, не пустой,
// который потом "обновляется" - но update делает ровно то же самое, что
// нужно и в момент открытия).
// analyze - семантический анализ документа (см. forCompletion).
func (d *doc) analyze(prog *ast.Program) (*ir.Program, error) {
	if d.forCompletion {
		return semantic.AnalyzeForCompletion(prog)
	}
	return semantic.Analyze(prog)
}

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
		// Явный конструктор (ed.Ctor/e.Ctor) - собственная область
		// видимости, тем же принципом, что и у обычного метода ниже, но
		// раньше не заводилась вовсе (только astProg.Functions/ed.Methods) -
		// hover/definition/completion внутри тела конструктора и на его
		// собственных именах параметров молча ничего не находили (нет
		// funcScope - scopeAt(line) возвращала nil на всех его строках).
		if ed.Ctor != nil && e.Ctor != nil {
			out = append(out, funcScope{
				declLine: ed.Ctor.Line, name: ed.Name + ".конструктор", entityName: ed.Name,
				params: e.Ctor.Params, body: e.Ctor.Body, hasReturn: e.Ctor.HasReturn, returnType: e.Ctor.ReturnType, doc: e.Ctor.Doc,
			})
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
// разбора настоящей границы блока - см. package doc walkLocals).
func localsAt(scope *funcScope, line int) map[string]ir.Type {
	out := map[string]ir.Type{}
	if scope == nil {
		return out
	}
	for _, p := range scope.params {
		out[p.Name] = p.Type
	}
	walkLocals(scope.body, line, func(name string, t ir.Type, _ *ir.VarDecl, _ int) { out[name] = t })
	return out
}

// localDeclsAt - объявления локальных переменных/констант, видимые на
// строке line (для doc-комментария и "переменная"/"константа" в hover).
// Переменные циклов и "поймать" объявлений не имеют - их здесь нет.
func localDeclsAt(scope *funcScope, line int) map[string]*ir.VarDecl {
	out := map[string]*ir.VarDecl{}
	if scope == nil {
		return out
	}
	walkLocals(scope.body, line, func(name string, _ ir.Type, decl *ir.VarDecl, _ int) {
		if decl != nil {
			out[name] = decl
		} else {
			delete(out, name)
		}
	})
	return out
}

// walkLocals - обход локальных имён тела до строки maxLine (см. localsAt):
// visit получает имя, тип, объявление (nil - переменная цикла/"поймать") и
// строку объявления (0 - неизвестна, у переменной "поймать"). Затенение
// (переменная объявлена дважды в разных ветках "если") не моделируется
// отдельно - последняя увиденная побеждает, для подсказки при hover/
// completion/definition этого достаточно.
func walkLocals(stmts []ir.Stmt, maxLine int, visit func(name string, t ir.Type, decl *ir.VarDecl, line int)) {
	for _, s := range stmts {
		switch s := s.(type) {
		case *ir.VarDecl:
			if s.Line <= maxLine {
				visit(s.Name, ir.TypeOf(s.Value), s, s.Line)
			}
		case *ir.IfStmt:
			walkLocals(s.Then, maxLine, visit)
			walkLocals(s.Else, maxLine, visit)
		case *ir.WhileStmt:
			walkLocals(s.Body, maxLine, visit)
		case *ir.ForStmt:
			if s.Line <= maxLine {
				visit(s.Var, ir.TypeInt, nil, s.Line)
			}
			walkLocals(s.Body, maxLine, visit)
		case *ir.ForEachStmt:
			if s.Line <= maxLine {
				if s.KeyVar != "" {
					visit(s.KeyVar, s.KeyType, nil, s.Line)
				}
				visit(s.Var, s.ElemType, nil, s.Line)
			}
			walkLocals(s.Body, maxLine, visit)
		case *ir.CStyleForStmt:
			if s.Init != nil && s.Line <= maxLine {
				visit(s.Init.Name, ir.TypeOf(s.Init.Value), s.Init, s.Line)
			}
			walkLocals(s.Body, maxLine, visit)
		case *ir.TryStmt:
			walkLocals(s.Body, maxLine, visit)
			visit(s.CatchVar, s.CatchVarType, nil, 0)
			walkLocals(s.CatchBody, maxLine, visit)
		case *ir.GerundStmt:
			walkLocals(s.Body, maxLine, visit)
		case *ir.BlockStmt:
			walkLocals(s.Body, maxLine, visit)
		}
	}
}
