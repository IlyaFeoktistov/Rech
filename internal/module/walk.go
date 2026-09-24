package module

import (
	"fmt"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/builtins"
)

// callableTypeName - зарезервированное имя типа "функция как значение"
// (internal/parser/callable.go, callableTypeName) - продублировано здесь
// локальной константой, не импортом (internal/parser уже импортирует
// internal/module, обратный импорт дал бы цикл), см. package doc
// resolveTypeRef.
const callableTypeName = "Вызываемый"

// findMatchingAngleClose возвращает индекс ">", закрывающего "<" на позиции
// openIdx в s, с учётом вложенных "<...>" (глубина, не первая попавшаяся
// ">") - -1, если не найдена. Используется resolveTypeRef для канонической
// строки типа "Вызываемый<...>Результат", где ">" после параметров - НЕ
// обязательно последний символ строки (в отличие от "Шаблон<...>" ниже).
func findMatchingAngleClose(s string, openIdx int) int {
	depth := 0
	for i := openIdx; i < len(s); i++ {
		switch s[i] {
		case '<':
			depth++
		case '>':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// builtinTypeBases - имена типов, встроенных в язык (не требуют ни
// объявления, ни импорта): примитивы, "Файл"/"Сокет"/"БД" (непрозрачные
// дескрипторы), "Запрос"/"Ответ" (HTTP-клиент,
// internal/builtins.ReservedEntityNames), "Исключение" (база для
// "наследует" в любом файле без явного объявления, internal/semantic),
// "Блокировка"/"Семафор" (синхронизация между потоками, internal/semantic/
// mutex.go, internal/semantic/semaphore.go) и имена четырёх generic-
// коллекций/"Указатель" - сами эти имена-шаблоны
// (то, что ДО "<") никогда не резолвятся как обычная ссылка на тип
// (resolveTypeRef ниже разбирает "Шаблон<Аргумент>" отдельно, резолвит
// только Аргумент - сам Шаблон встроен всегда, вне зависимости от того,
// что внутри "<...>" - примитив, другая коллекция или пользовательская
// сущность/структура, internal/semantic.resolveGenericTypeName).
var builtinTypeBases = map[string]bool{
	"Целое":          true,
	"Действительное": true,
	"Дробное":        true,
	"Символ":         true,
	"Байт":           true,
	"Строка":         true,
	"Логическое":     true,
	"Файл":           true,
	"Сокет":          true,
	"БД":             true,
	"HTTP2Сессия":    true,
	"HTTP2Поток":     true,
	"Запрос":         true,
	"Ответ":          true,
	"Задача":         true,
	"Исключение":     true,
	"Список":         true,
	"Стопка":         true,
	"Очередь":        true,
	"Словарь":        true,
	"ДатаВремя":      true,
	"RGBЦвет":        true,
	"ОшибкаРазбора":  true,
	"Блокировка":     true,
	"Семафор":        true,
	"Транзакция":     true,
	"ОпцииКуки":      true,
	"Совпадение":     true,
	"ЗапросБайты":    true,
	"ОтветБайты":     true,
	"ЧастьФормы":     true,
	"ДекодированноеИзображение": true,
	// Дейтаграмма/Процесс/РезультатКоманды/Регистры/СостояниеБД -
	// НАЙДЕННЫЙ БАГ (исправлен здесь): встроенные сущности из
	// internal/builtins.ReservedEntityNames, пропущенные при добавлении
	// самих сущностей - без записи здесь имя явно как тип (в отличие от
	// употребления в позиции конструктора/вызова, resolveBareName для
	// функций/значений - отдельная, уже верная ветка) считалось
	// "необъявленным и неимпортированным" везде, где resolveTypeRef реально
	// доходит до него - обнаружилось только когда rewriteStmt(*ast.VarDecl)
	// начал резолвить s.Type (см. её комментарий).
	"Дейтаграмма":      true,
	"Процесс":          true,
	"РезультатКоманды": true,
	"Регистры":         true,
	"СостояниеБД":      true,
	"Целое8":           true,
	"Целое16":          true,
	"Целое32":          true,
	"Целое64":          true,
	"Натуральное":      true,
	"Натуральное8":     true,
	"Натуральное16":    true,
	"Натуральное32":    true,
	"Натуральное64":    true,
	"Тип":              true,
	"Значение":         true,
}

// resolveBareName ищет name (имя функции или простого - не generic - типа)
// в names (карта "как написано в файле" -> "итоговое, возможно
// переименованное имя", см. link.go) - встроенные имена (типы из
// builtinTypeBases, функции шага 8 из internal/builtins - ввод консоли,
// математика) возвращаются как есть: они не объявлены ни в одном файле
// Rech-программы и потому не участвуют в переименовании при линковке.
// Не найденное имя - ошибка ("не объявлено и не импортировано").
//
// names проверяется ПЕРВЫМ, до builtinTypeBases/IsReservedName - затенение
// (internal/builtins.ShadowableEntityNames, "давай группу А" - ДатаВремя и
// т. п.): если файл сам объявил (или явно импортировал) сущность с именем,
// совпадающим со встроенным, names уже содержит запись на неё (см.
// buildNames), и эта своя сущность должна победить. Для любого имени, для
// которого names НЕ содержит записи (обычный случай - имя ничем не
// затенено, а также ЛЮБОЕ реально зарезервированное имя вроде "Блокировка"/
// "Файл" - собственную сущность с таким именем в принципе нельзя объявить,
// internal/semantic.ReservedEntityNames без ShadowableEntityNames), порядок
// проверки не меняет результат - до names долетать нечему.
func resolveBareName(name string, names map[string]string) (string, error) {
	if resolved, ok := names[name]; ok {
		return resolved, nil
	}
	if builtinTypeBases[name] {
		return name, nil
	}
	if builtins.IsReservedName(name) {
		return name, nil
	}
	return "", fmt.Errorf("%q не объявлено в этом файле и не импортировано", name)
}

// resolveTypeRef - то же самое для имени типа, включая generic-инстанциации
// ("Список<Целое>"/"Список<Пользователь>"). "?Тип" (nullable,
// ir.KindNullable) - отдельный случай ДО проверки "<": сама
// nullable-обёртка не объявляется и не импортируется (как и "Список"), но
// её внутренний Тип - обычное имя, которое обязано пройти резолюцию/
// переименование при импорте как любое другое (например, "?Пользователь"
// из другого файла с "как" должно стать "?Псевдоним", не остаться
// нерезолвленным).
//
// "Вызываемый<Тип1, Тип2>Результат" (каноническая строка типа "функция как
// значение", internal/parser/callable.go, callableTypeName - НЕ
// импортируется отсюда: internal/parser уже импортирует internal/module,
// импорт в обратную сторону дал бы цикл, поэтому имя продублировано
// локальной строкой) - результат идёт СРАЗУ за закрывающей ">" БЕЗ
// разделителя, значит форма НЕ подходит под общий случай "Шаблон<...>"
// ниже (тот требует, чтобы typeName оканчивался на ">" ЦЕЛИКОМ - для
// непустого Результата это не так). Раньше (см. историю этой функции)
// из-за этого несовпадения строка возвращалась КАК ЕСТЬ безусловно - тип
// пользовательской сущности внутри "Вызываемый(...)" не переименовывался
// при связывании нескольких файлов, "неизвестный тип" у cbackend. Разбирается
// отдельно, ДО общего случая "Шаблон<...>" ниже: находится ЗАКРЫВАЮЩАЯ
// "<...>", соответствующая первой "<" (findMatchingAngleClose - учитывает
// вложенные "<...>" у самих параметров/результата, как и
// splitTopLevelTypeArgs ниже), всё после неё - Результат (резолвится тем же
// resolveTypeRef, пусто - результата нет). Собственные параметры типа
// "Вызываемый<T>(T): T" в каноническую строку не попадают вовсе
// (parseCallableTypeName, package doc) - здесь их и не увидеть.
//
// "Шаблон<Аргумент[, Аргумент2]>" (Список/Стопка/Очередь/Словарь/
// Указатель) - САМ Шаблон не резолвится (встроенное ключевое слово, не
// объявляется/не импортируется), а каждый Аргумент - обычная ссылка на
// тип, резолвится рекурсивно тем же resolveTypeRef (Аргумент - примитив,
// ДРУГАЯ встроенная коллекция или пользовательская сущность/структура,
// см. internal/semantic.resolveGenericTypeName - сущность-элемент из
// НЕ-entry модуля обязана переименоваться точно так же, как и любая другая
// ссылка на неё). Требует ровно распознанной формы "Шаблон<...>" ЦЕЛИКОМ
// (typeName оканчивается на ">").
// typeParams - имена параметров типа generic-сущности/функции, ЧЬЁ
// объявление сейчас резолвится (rewriter.typeParams, typeParamSet) -
// локальные заглушки ("T"), не ссылки на объявленный где-то тип, поэтому
// возвращаются как есть, БЕЗ похода в names - вплоть до этого исправления
// resolveTypeRef ЭТОГО НЕ ЗНАЛ вообще: только верхнеуровневая обёртка
// rewriter.resolveType сверяла typeParams, и только для ЦЕЛОЙ строки типа
// параметра/поля/результата ЦЕЛИКОМ - "T" сам по себе (тип результата)
// проходил проверку, но "Список<T>" (тип параметра generic-функции вида
// "первый<T>(список: Список<T>): T") не совпадал с "T" как целая строка,
// проваливался в резолюцию Аргумента ниже уже БЕЗ typeParams (рекурсивные
// вызовы были к этой же функции напрямую, в обход rewriter.resolveType) -
// "T" внутри "Список<T>" считалось ссылкой на необъявленный тип. НАЙДЕННЫЙ
// РЕАЛЬНЫЙ БАГ, не связанный с "Вызываемый(...)" выше: ЛЮБАЯ
// generic-функция/сущность с контейнером собственного параметра типа
// параметром/полем ломалась при прохождении через настоящий
// module.Link() (даже в ОДНОМ файле без единого "использовать" -
// compiler.Compile всегда вызывает Link()) - не поймано тестами раньше,
// потому что internal/interp.TestRunAllExamplesMatchCompiledOutput
// (`examples/обобщённые/коробка.rech`, ровно этот случай) гоняет
// parser+semantic напрямую, минуя internal/module целиком.
func resolveTypeRef(typeName string, names map[string]string, typeParams map[string]bool) (string, error) {
	if typeName == "" {
		return typeName, nil
	}
	if typeParams[typeName] || typeName == callableTypeName {
		// Голое "Вызываемый" - только "Вызываемый фн = значение", сигнатура
		// из значения (internal/semantic/bare_callable_decl.go).
		return typeName, nil
	}
	if strings.HasPrefix(typeName, "?") {
		inner, err := resolveTypeRef(typeName[len("?"):], names, typeParams)
		if err != nil {
			return "", err
		}
		return "?" + inner, nil
	}
	if strings.HasPrefix(typeName, callableTypeName+"<") {
		open := len(callableTypeName)
		closeIdx := findMatchingAngleClose(typeName, open)
		if closeIdx == -1 {
			return typeName, nil
		}
		paramsText := typeName[open+1 : closeIdx]
		var resolvedParams []string
		if paramsText != "" {
			for _, a := range splitTopLevelTypeArgs(paramsText) {
				resolved, err := resolveTypeRef(strings.TrimSpace(a), names, typeParams)
				if err != nil {
					return "", err
				}
				resolvedParams = append(resolvedParams, resolved)
			}
		}
		resultText := typeName[closeIdx+1:]
		resolvedResult := ""
		if resultText != "" {
			resolved, err := resolveTypeRef(resultText, names, typeParams)
			if err != nil {
				return "", err
			}
			resolvedResult = resolved
		}
		return callableTypeName + "<" + strings.Join(resolvedParams, ", ") + ">" + resolvedResult, nil
	}
	if open := strings.Index(typeName, "<"); open != -1 && strings.HasSuffix(typeName, ">") {
		template := typeName[:open]
		args := splitTopLevelTypeArgs(typeName[open+1 : len(typeName)-1])
		resolvedArgs := make([]string, len(args))
		for i, a := range args {
			resolved, err := resolveTypeRef(strings.TrimSpace(a), names, typeParams)
			if err != nil {
				return "", err
			}
			resolvedArgs[i] = resolved
		}
		// Шаблон - имя из names (импортированный под другим именем
		// generic-синоним), иначе как есть (встроенные коллекции).
		if resolved, ok := names[template]; ok {
			template = resolved
		}
		return template + "<" + strings.Join(resolvedArgs, ", ") + ">", nil
	}
	if strings.Contains(typeName, "<") {
		return typeName, nil
	}
	return resolveBareName(typeName, names)
}

// splitTopLevelTypeArgs разбивает argsText (содержимое между "<" и ">") по
// запятым верхнего уровня - не по запятым внутри ВЛОЖЕННОГО "<...>"
// (многомерные списки/Словарь со сложным аргументом-значением, "Словарь<
// Строка, Список<Целое>>" не должна разбиться на "Строка"/"Список<Целое"/
// "Целое>>").
func splitTopLevelTypeArgs(argsText string) []string {
	var args []string
	depth := 0
	start := 0
	for i, r := range argsText {
		switch r {
		case '<':
			depth++
		case '>':
			depth--
		case ',':
			if depth == 0 {
				args = append(args, argsText[start:i])
				start = i + 1
			}
		}
	}
	args = append(args, argsText[start:])
	return args
}

// rewriter переписывает каждую ссылку на глобальное имя (вызов функции,
// имя типа) в дереве по names - см. link.go, buildNames. typeParams - имена
// параметров типа generic-сущности/функции, чьи поля/параметры/тип
// результата сейчас переписываются - локальные заглушки ("T"), не
// ссылки на объявленный где-то тип, поэтому исключены из резолюции;
// выставляется/сбрасывается вызывающим кодом (link.go) перед/после каждого
// generic-объявления, nil - вне такого объявления (обычная резолюция).
type rewriter struct {
	names      map[string]string
	typeParams map[string]bool
	// locals - имена, объявленные внутри тела функции/метода, которое
	// сейчас переписывается (параметры, "переменная"/"константа",
	// переменная цикла "для"/"для...в", "поймать(...)") - у локальной
	// переменной типа "Вызываемый", вызванной как "ф(...)" ("ф" не найдено
	// в names - это не файловая декларация), отсутствие записи в names не
	// ошибка видимости, в отличие от истинно необъявленного/
	// неимпортированного имени - см. CallExpr case в rewriteExpr.
	// Сбрасывается вызывающим кодом (link.go) перед каждым новым телом
	// функции/метода верхнего уровня - не настоящая вложенная область
	// видимости (переменная одной ветки "если" считается локальной для
	// всего тела), но ложноположительный исход (пропущенная ошибка
	// видимости при случайном совпадении имени) безопаснее
	// ложноотрицательного (сломанный вызов значения).
	locals map[string]bool
	// allEnums/allConsts - имена ВСЕХ перечислений/констант уровня модуля,
	// объявленных хоть где-то в ЦЕЛОЙ программе (across p.Modules, не
	// только в m/её импортах) - строятся один раз в Link(), общие для всех
	// rewriter'ов прохода. Нужны ТОЛЬКО чтобы отличить "имя ссылается на
	// перечисление/константу, объявленные в другом файле, но не
	// импортированные (или приватные) здесь - ошибка видимости" от "имя не
	// найдено вообще нигде - не наша забота, пусть разбирается
	// internal/semantic (голая необъявленная переменная, ссылка на
	// встроенное перечисление вроде HTTPКод/ДеньНедели/Месяц - см.
	// standardEnumNames, или тип сущности/интерфейса в паттерне "выбрать/
	// когда")" - см. rewriteExpr, case *ast.Identifier/*ast.FieldAccess.
	allEnums  map[string]bool
	allConsts map[string]bool
}

func (r *rewriter) addLocal(name string) {
	if r.locals == nil {
		r.locals = map[string]bool{}
	}
	r.locals[name] = true
}

// typeParamSet - имена параметров типа params в виде множества, для
// rewriter.typeParams - nil, если params пуст (не generic-объявление).
func typeParamSet(params []ast.GenericParam) map[string]bool {
	if len(params) == 0 {
		return nil
	}
	set := make(map[string]bool, len(params))
	for _, p := range params {
		set[p.Name] = true
	}
	return set
}

// resolveType - resolveTypeRef с уже известными r.names/r.typeParams (см.
// package doc resolveTypeRef про typeParams).
func (r *rewriter) resolveType(typeName string) (string, error) {
	return resolveTypeRef(typeName, r.names, r.typeParams)
}

func (r *rewriter) rewriteParams(params []ast.Param) error {
	for i, p := range params {
		resolved, err := r.resolveType(p.Type)
		if err != nil {
			return fmt.Errorf("параметр %q: %w", p.Name, err)
		}
		params[i].Type = resolved
		r.addLocal(p.Name)
	}
	return nil
}

func (r *rewriter) rewriteFields(fields []ast.Field) error {
	for i, f := range fields {
		resolved, err := r.resolveType(f.Type)
		if err != nil {
			return fmt.Errorf("поле %q: %w", f.Name, err)
		}
		fields[i].Type = resolved
	}
	return nil
}

func (r *rewriter) rewriteReturnType(returnType string) (string, error) {
	return r.resolveType(returnType)
}

// rewriteTypeParams переписывает имена типов внутри ограничений параметров
// типа generic-сущности/функции - сами имена параметров (T, U)
// НЕ переписываются (это не ссылки на объявленные где-то типы, а
// локальные заглушки, вводимые прямо здесь).
func (r *rewriter) rewriteTypeParams(params []ast.GenericParam) error {
	for i, p := range params {
		for j, c := range p.Constraints {
			// nil, не r.typeParams - вызывается ДО link.go выставляет
			// r.typeParams для этого же generic-объявления (см. package doc
			// rewriter), да и ограничение не может осмысленно ссылаться на
			// сам объявляемый параметр типа.
			resolved, err := resolveTypeRef(c, r.names, nil)
			if err != nil {
				return fmt.Errorf("параметр типа %q: %w", p.Name, err)
			}
			params[i].Constraints[j] = resolved
		}
	}
	return nil
}

func (r *rewriter) rewriteStmts(stmts []ast.Stmt) error {
	for _, s := range stmts {
		if err := r.rewriteStmt(s); err != nil {
			return err
		}
	}
	return nil
}

func (r *rewriter) rewriteStmt(s ast.Stmt) error {
	switch s := s.(type) {
	case *ast.PrintStmt:
		return r.rewriteExpr(s.Value)
	case *ast.ShowStmt:
		return r.rewriteExpr(s.Value)
	case *ast.VarDecl:
		r.addLocal(s.Name)
		// s.Type - явная аннотация ("переменная а: Тип = ...") ИЛИ сахарная
		// форма "Тип а = ..." (оба хранятся в одном и том же поле, см. package
		// doc ast.VarDecl) - НАЙДЕННЫЙ БАГ (исправлен здесь): в отличие от
		// ast.NewExpr.Type чуть ниже, это поле не резолвилось вовсе - сущность
		// из НЕ-entry модуля (импортированная или своя, всё равно
		// переименовывается при линковке, см. package doc mangledName) в этой
		// позиции оставалась НЕПЕРЕИМЕНОВАННЫМ bare-именем, cbackend падал
		// "неизвестный тип" - при этом та же сущность справа, в "новое
		// Тип(...)", резолвилась верно (её резолвит отдельный case ниже).
		resolvedType, err := r.resolveType(s.Type)
		if err != nil {
			return fmt.Errorf("строка %d: %w", s.Line, err)
		}
		s.Type = resolvedType
		return r.rewriteExpr(s.Value)
	case *ast.ConstDecl:
		r.addLocal(s.Name)
		return r.rewriteExpr(s.Value)
	case *ast.ParentCallStmt:
		// НАЙДЕННЫЙ БАГ (исправлен здесь, попутно с паритетом Enum/Const,
		// не связан с ним): "родитель(<арг>,...)" (явный конструктор с
		// "наследует") вообще не входил ни в один
		// case этого switch - падал в default ("внутренняя ошибка:
		// неизвестная инструкция") при ЛЮБОМ прогоне через настоящий
		// "rech build"/"rech run" (compiler.Compile всегда вызывает
		// module.Link() - см. её package doc), даже в ОДНОМ файле без
		// единого "использовать" - тот же класс пробела, что уже был у
		// EntityDecl.Ctor (см. её комментарий выше по этому файлу), не
		// пойманный existing тестами интерпретатора (парсер+semantic
		// напрямую, минуя internal/module целиком - package doc
		// internal/interp.TestRunAllExamplesMatchCompiledOutput). Аргументы
		// вызова родительского конструктора нуждаются в той же резолюции
		// имён, что и любой другой вызов.
		return r.rewriteExprs(s.Args)
	case *ast.Assign:
		return r.rewriteExpr(s.Value)
	case *ast.NullCoalesceAssign:
		return r.rewriteExpr(s.Value)
	case *ast.FieldAssign:
		if err := r.rewriteExpr(s.Receiver); err != nil {
			return err
		}
		return r.rewriteExpr(s.Value)
	case *ast.BecomeStmt:
		return r.rewriteExpr(s.Receiver)
	case *ast.DerefAssignStmt:
		if err := r.rewriteExpr(s.Pointer); err != nil {
			return err
		}
		return r.rewriteExpr(s.Value)
	case *ast.AssertStmt:
		if err := r.rewriteExpr(s.Cond); err != nil {
			return err
		}
		if s.Message == nil {
			return nil
		}
		return r.rewriteExpr(s.Message)
	case *ast.IfStmt:
		if err := r.rewriteExpr(s.Cond); err != nil {
			return err
		}
		if err := r.rewriteStmts(s.Then); err != nil {
			return err
		}
		return r.rewriteStmts(s.Else)
	case *ast.WhileStmt:
		if err := r.rewriteExpr(s.Cond); err != nil {
			return err
		}
		return r.rewriteStmts(s.Body)
	case *ast.ForStmt:
		r.addLocal(s.Var)
		if err := r.rewriteExpr(s.From); err != nil {
			return err
		}
		if err := r.rewriteExpr(s.To); err != nil {
			return err
		}
		return r.rewriteStmts(s.Body)
	case *ast.ForEachStmt:
		if s.KeyVar != "" {
			r.addLocal(s.KeyVar)
		}
		r.addLocal(s.Var)
		if err := r.rewriteExpr(s.Collection); err != nil {
			return err
		}
		return r.rewriteStmts(s.Body)
	case *ast.CStyleForStmt:
		// Init/Cond/Post независимо могут быть nil ("для ;; { ... }",
		// package doc ast.CStyleForStmt) - rewriteStmt/rewriteExpr здесь
		// НЕ nil-безопасны сами по себе (их default - настоящая ошибка
		// "неизвестная инструкция/выражение", единственная защита от
		// пропущенного case, см. *ast.ParentCallStmt выше) - каждая секция
		// пропускается явно, если её нет. Init, когда есть, уже
		// *ast.VarDecl - тот же case выше (addLocal + rewriteExpr(Value)) -
		// не дублируется вручную.
		if s.Init != nil {
			if err := r.rewriteStmt(s.Init); err != nil {
				return err
			}
		}
		if s.Cond != nil {
			if err := r.rewriteExpr(s.Cond); err != nil {
				return err
			}
		}
		if s.Post != nil {
			if err := r.rewriteStmt(s.Post); err != nil {
				return err
			}
		}
		return r.rewriteStmts(s.Body)
	case *ast.BreakStmt, *ast.ContinueStmt:
		return nil
	case *ast.ReturnStmt:
		if s.Value == nil {
			return nil
		}
		return r.rewriteExpr(s.Value)
	case *ast.ExprStmt:
		if s.Call != nil {
			return r.rewriteExpr(s.Call)
		}
		if s.MethodCall != nil {
			return r.rewriteExpr(s.MethodCall)
		}
		if s.Await != nil {
			return r.rewriteExpr(s.Await)
		}
		return nil
	case *ast.ThrowStmt:
		return r.rewriteExpr(s.Value)
	case *ast.TryStmt:
		if s.CatchVar != "" {
			r.addLocal(s.CatchVar)
		}
		if err := r.rewriteStmts(s.Body); err != nil {
			return err
		}
		for i, t := range s.CatchTypes {
			resolved, err := resolveTypeRef(t, r.names, r.typeParams)
			if err != nil {
				return fmt.Errorf("\"поймать(...)\": %w", err)
			}
			s.CatchTypes[i] = resolved
		}
		return r.rewriteStmts(s.CatchBody)
	case *ast.GerundStmt:
		resolved, err := resolveBareName(s.Callee, r.names)
		if err != nil {
			if !r.locals[s.Callee] {
				return fmt.Errorf("строка %d: %w", s.Line, err)
			}
		} else {
			s.Callee = resolved
		}
		if err := r.rewriteExprs(s.Args); err != nil {
			return err
		}
		return r.rewriteStmts(s.Body)
	case *ast.SwitchStmt:
		if err := r.rewriteExpr(s.Subject); err != nil {
			return err
		}
		for _, c := range s.Cases {
			// Значения ветки "когда" - как и везде в этом rewriter'е, голое
			// имя (*ast.Identifier) не переписывается (см. case *ast.Identifier
			// в rewriteExpr ниже) - для типа, объявленного тут же в файле,
			// этого достаточно (resolveTypeName в internal/semantic ищет по
			// тому же голому имени); импортированный тип, сопоставляемый по
			// типу в "когда" (а не по значению), в этом срезе не
			// квалифицируется - как и голые Identifier-значения нигде в этом
			// rewriter'е не квалифицируются.
			if err := r.rewriteExprs(c.Values); err != nil {
				return err
			}
			if err := r.rewriteStmts(c.Body); err != nil {
				return err
			}
		}
		return r.rewriteStmts(s.Default)
	default:
		return fmt.Errorf("внутренняя ошибка: неизвестная инструкция %T", s)
	}
}

func (r *rewriter) rewriteExprs(exprs []ast.Expr) error {
	for _, e := range exprs {
		if err := r.rewriteExpr(e); err != nil {
			return err
		}
	}
	return nil
}

func (r *rewriter) rewriteExpr(e ast.Expr) error {
	switch e := e.(type) {
	case *ast.StringLiteral, *ast.IntLiteral, *ast.FloatLiteral, *ast.BoolLiteral, *ast.CharLiteral, *ast.NullLiteral, *ast.ThisExpr, *ast.IncDecExpr:
		return nil
	case *ast.Identifier:
		// Голое имя как ЗНАЧЕНИЕ - подавляющее большинство случаев (локальная
		// переменная/параметр) НЕ файловая декларация вовсе, r.locals
		// проверяется первым, тем же приоритетом, что у затенения в
		// internal/semantic (sc.lookup смотрит переменные раньше константы
		// уровня модуля). Иначе имя может называть КОНСТАНТУ уровня модуля -
		// единственную категорию объявлений, у которой голое имя (без "."/
		// "::") само по себе - файловая декларация (функции/сущности идут
		// через CallExpr/NewExpr/типовые позиции, не бывают голым значением
		// напрямую) - r.names уже включает свои/импортированные константы
		// (module.go, declaredNames), резолюция - тот же resolveBareName,
		// что и у CallExpr.Callee ниже. r.allConsts - имя КОНСТАНТЫ,
		// объявленной ГДЕ-ТО в целой программе (паритет с
		// уже существующей приватностью функций/сущностей/интерфейсов), но
		// не найденной в r.names ИМЕННО ЭТОГО файла - настоящая ошибка
		// видимости (приватная в другом файле или просто не импортирована
		// здесь), в отличие от ЛЮБОГО другого нерезолвленного имени
		// (необъявленная переменная, встроенное перечисление вроде
		// HTTPКод.ОК - его ИМЯ здесь не встречается голым, "тип.член" идёт
		// через FieldAccess ниже, голое имя типа сущности/интерфейса в
		// паттерне "выбрать/когда" - см. package doc SwitchCase.Values) - те
		// оставляются как есть, тем же принципом, что и раньше (либо дальше
		// разберётся internal/semantic с более точным сообщением, либо это
		// осознанно некатегоризированный здесь случай).
		if r.locals[e.Name] {
			return nil
		}
		if resolved, err := resolveBareName(e.Name, r.names); err == nil {
			e.Name = resolved
			return nil
		}
		if r.allConsts[e.Name] {
			return fmt.Errorf("строка %d: константа %q не объявлена в этом файле и не импортирована", e.Line, e.Name)
		}
		return nil
	case *ast.CallExpr:
		// Callee здесь может называть либо объявленную функцию (обычный
		// случай), либо локальную переменную типа "Вызываемый"
		// ("ф(<арг>, ...)") - у локальных имён нет записи в r.names (это
		// не файловая/импортированная декларация), и в отличие от истинно
		// необъявленного/неимпортированного имени, отсутствие записи
		// здесь не ошибка, если имя есть среди r.locals (см. package doc
		// rewriter.locals).
		resolved, err := resolveBareName(e.Callee, r.names)
		if err != nil {
			if !r.locals[e.Callee] {
				return fmt.Errorf("строка %d: %w", e.Line, err)
			}
		} else {
			e.Callee = resolved
		}
		for i, t := range e.TypeArgs {
			resolvedType, err := resolveTypeRef(t, r.names, r.typeParams)
			if err != nil {
				return fmt.Errorf("строка %d: аргумент типа: %w", e.Line, err)
			}
			e.TypeArgs[i] = resolvedType
		}
		return r.rewriteExprs(e.Args)
	case *ast.NewExpr:
		resolved, err := r.resolveType(e.Type)
		if err != nil {
			return fmt.Errorf("строка %d: %w", e.Line, err)
		}
		e.Type = resolved
		return r.rewriteExprs(e.Args)
	case *ast.FieldAccess:
		// Receiver - Identifier покрывает ТРИ разных случая одной и той же
		// формой AST, различить их можно только по имени (без типов на этой
		// стадии): (1) обычный доступ к полю/методу через "." у ЗНАЧЕНИЯ
		// (локальная переменная/параметр слева) - самый частый случай,
		// r.locals проверяется первым, тем же приоритетом, что и у
		// *ast.Identifier выше; (2) "Перечисление.Член" - r.names уже
		// включает свои/импортированные перечисления (module.go); (3)
		// "Тип::статическийЧлен" (Static==true) - Receiver обязан быть
		// именем сущности/структуры (internal/semantic.staticReceiverEntity
		// уже отдельно отвергает переменную слева от "::"), тоже резолвится
		// через r.names тем же путём - побочный эффект этого изменения:
		// "Тип::член" для сущности из ДРУГОГО (импортированного) файла тоже
		// начинает резолвиться корректно (раньше не резолвилось никак,
		// отдельный, не пересматриваемый здесь пробел). r.allEnums - имя
		// ПЕРЕЧИСЛЕНИЯ, объявленного ГДЕ-ТО в целой программе, но не
		// найденного в r.names этого файла - настоящая ошибка видимости
		// (паритет с уже существующей приватностью
		// функций/сущностей/интерфейсов). Встроенные перечисления
		// (HTTPКод/ДеньНедели/Месяц, internal/semantic.StandardEnums) не
		// объявлены ни в одном .rech-файле программы - не входят ни в
		// r.names, ни в r.allEnums, поэтому проходят молча, как и раньше.
		if ident, ok := e.Receiver.(*ast.Identifier); ok {
			if r.locals[ident.Name] {
				return nil
			}
			if resolved, err := resolveBareName(ident.Name, r.names); err == nil {
				ident.Name = resolved
				return nil
			}
			if r.allEnums[ident.Name] {
				return fmt.Errorf("строка %d: перечисление %q не объявлено в этом файле и не импортировано", e.Line, ident.Name)
			}
			return nil
		}
		return r.rewriteExpr(e.Receiver)
	case *ast.IndexExpr:
		if err := r.rewriteExpr(e.Receiver); err != nil {
			return err
		}
		return r.rewriteExpr(e.Index)
	case *ast.ListLiteral:
		return r.rewriteExprs(e.Elems)
	case *ast.SpreadExpr:
		return r.rewriteExpr(e.Value)
	case *ast.MethodCallExpr:
		if err := r.rewriteExpr(e.Receiver); err != nil {
			return err
		}
		return r.rewriteExprs(e.Args)
	case *ast.BinaryExpr:
		if err := r.rewriteExpr(e.Left); err != nil {
			return err
		}
		return r.rewriteExpr(e.Right)
	case *ast.TernaryExpr:
		if err := r.rewriteExpr(e.Cond); err != nil {
			return err
		}
		if err := r.rewriteExpr(e.Then); err != nil {
			return err
		}
		return r.rewriteExpr(e.Else)
	case *ast.UnaryExpr:
		return r.rewriteExpr(e.Operand)
	case *ast.CastExpr:
		return r.rewriteExpr(e.Value)
	case *ast.ParseExpr:
		resolved, err := r.resolveType(e.TypeName)
		if err != nil {
			return fmt.Errorf("строка %d: %w", e.Line, err)
		}
		e.TypeName = resolved
		return r.rewriteExpr(e.Source)
	case *ast.SerializeExpr:
		return r.rewriteExpr(e.Value)
	case *ast.IsExpr:
		resolved, err := r.resolveType(e.Target)
		if err != nil {
			return fmt.Errorf("строка %d: %w", e.Line, err)
		}
		e.Target = resolved
		return r.rewriteExpr(e.Value)
	case *ast.LaunchExpr:
		return r.rewriteExpr(e.Operand())
	case *ast.CoroLaunchExpr:
		return r.rewriteExpr(e.Call)
	case *ast.AwaitExpr:
		return r.rewriteExpr(e.Task)
	case *ast.FuncRefExpr:
		// "&имя" называет либо объявленную функцию (обычный случай -
		// ссылка на неё как на "Вызываемый"), либо локальную переменную
		// (адрес её значения, "Указатель<T>", docs/SPEC.md, "Указатели") -
		// тот же принцип исключения через r.locals, что и у CallExpr.Callee
		// выше (у локальных имён нет записи в r.names).
		resolved, err := resolveBareName(e.Name, r.names)
		if err != nil {
			if !r.locals[e.Name] {
				return fmt.Errorf("строка %d: %w", e.Line, err)
			}
			return nil
		}
		e.Name = resolved
		return nil
	case *ast.AddressOfExpr:
		// Receiver - всегда простое имя локальной переменной/параметра
		// (docs/SPEC.md, "Указатели") - как и у FuncRefExpr.Name в случае
		// локальной переменной выше, переименованию не подлежит. Index -
		// произвольное выражение (может ссылаться на импортированное имя).
		if e.Index != nil {
			return r.rewriteExpr(e.Index)
		}
		return nil
	case *ast.DerefExpr:
		return r.rewriteExpr(e.Value)
	case *ast.TypeOfExpr:
		resolved, err := r.resolveType(e.TypeName)
		if err != nil {
			return fmt.Errorf("строка %d: %w", e.Line, err)
		}
		e.TypeName = resolved
		return nil
	case *ast.AsExpr:
		resolved, err := r.resolveType(e.TypeName)
		if err != nil {
			return fmt.Errorf("строка %d: %w", e.Line, err)
		}
		e.TypeName = resolved
		return r.rewriteExpr(e.Value)
	case *ast.LambdaExpr:
		if err := r.rewriteParams(e.Params); err != nil {
			return fmt.Errorf("строка %d: лямбда: %w", e.Line, err)
		}
		resolved, err := r.rewriteReturnType(e.ReturnType)
		if err != nil {
			return fmt.Errorf("строка %d: лямбда: %w", e.Line, err)
		}
		e.ReturnType = resolved
		return r.rewriteStmts(e.Body)
	default:
		return fmt.Errorf("внутренняя ошибка: неизвестное выражение %T", e)
	}
}

// stringSet - множество из среза имён (nil для пустого).
func stringSet(names []string) map[string]bool {
	if len(names) == 0 {
		return nil
	}
	set := make(map[string]bool, len(names))
	for _, n := range names {
		set[n] = true
	}
	return set
}
