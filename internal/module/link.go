package module

import (
	"fmt"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
)

// mangledName - имя, под которым объявление name из модуля m в итоге
// попадает в слинкованную программу: без изменений у entry (единственный
// модуль, чьи имена видны программе снаружи - как единица компиляции,
// раньше это тоже был всегда единственный файл), с префиксом Qualifier у
// всех остальных - модули загружаются независимо друг от друга и могут
// использовать одно и то же имя для разных вещей (сама идея пространств
// имён), поэтому при слиянии в одну плоскую internal/ast.Program (её всё
// ещё разбирает единственный проход cbackend/semantic, см. package doc)
// имена нужно развести, иначе они столкнутся так же, как если бы это был
// один файл.
func mangledName(m, entry *Module, name string) string {
	if m == entry {
		return name
	}
	return m.Qualifier + "_" + name
}

// buildNames строит карту "имя, как написано в файле m" -> "итоговое имя
// после линковки" - для собственных объявлений m (включая производные типы
// состояний - localTypeNames, а не только то, что можно импортировать) и
// для того, что m импортировал (Import.Alias - как оно написано в теле m -
// -> mangledName настоящего Import.Name в модуле-источнике; переименование
// при импорте, "как", - именно поэтому ключ здесь Alias, а не Name).
//
// НАЙДЕННЫЙ БАГ (исправлен здесь): имена интерфейсов m.AST.Interfaces
// входят в localTypeNames(m.AST) наравне с функциями/сущностями, поэтому
// раньше тоже проходили через mangledName - получали префикс Qualifier у
// любого НЕ-entry модуля. Но Link() (см. merge ниже) сливает
// merged.Interfaces БЕЗ переименования (тем же принципом, что и Enums/
// Consts) - несовпадение этих двух мест давало ссылку на смангленное имя
// ("Файл_Интерфейс") там, где сам интерфейс на самом деле остался под
// своим бare-именем, и cbackend падал "неизвестный тип" при первом же
// использовании интерфейса как типа параметра/поля/результата внутри
// НЕ-entry файла многофайловой программы (что скомпилировалось бы без
// единого "использовать" вообще). Интерфейсы поэтому исключены из
// mangledName здесь - разрешаются в СВОЁ СОБСТВЕННОЕ bare-имя, ровно как
// они и оказываются в merged.Interfaces. Тот же принцип применён ниже к
// ИМПОРТИРОВАННЫМ интерфейсам, а также к перечислениям и константам уровня
// модуля (паритет с уже существующей приватностью
// функций/сущностей) - у всех трёх категорий merge (см. Link ниже) не
// манглит собственное имя, значит и buildNames обязана разрешать
// ссылку/alias в то же bare-имя, а не в mangledName(...) - иначе тот же
// класс несовпадения.
func buildNames(m, entry *Module) map[string]string {
	names := map[string]string{}
	for name := range localTypeNames(m.AST) {
		if unmangledCategoryName(m.AST, name) {
			names[name] = name
			continue
		}
		names[name] = mangledName(m, entry, name)
	}
	for _, imp := range m.Imports {
		if unmangledCategoryName(imp.Source.AST, imp.Name) {
			names[imp.Alias] = imp.Name
			continue
		}
		names[imp.Alias] = mangledName(imp.Source, entry, imp.Name)
	}
	return names
}

// unmangledCategoryName сообщает, объявлен ли name в prog как интерфейс,
// перечисление или константа уровня модуля (а не как функция/сущность) -
// используется buildNames выше, чтобы решить, нужно ли имени mangledName
// (этим трём категориям - нет, см. её комментарий: Link() сливает их без
// переименования, единая плоская область видимости на всю программу).
func unmangledCategoryName(prog *ast.Program, name string) bool {
	for _, iface := range prog.Interfaces {
		if iface.Name == name {
			return true
		}
	}
	for _, e := range prog.Enums {
		if e.Name == name {
			return true
		}
	}
	for _, c := range prog.Consts {
		if c.Name == name {
			return true
		}
	}
	for _, v := range prog.Vars {
		if v.Name == name {
			return true
		}
	}
	for _, s := range prog.Synonyms {
		if s.Name == name {
			return true
		}
	}
	return false
}

// Link проверяет видимость (каждая ссылка на функцию/тип в теле функции,
// метода или в сигнатуре сущности должна быть либо объявлена в том же
// файле, либо явно импортирована - иначе ошибка компиляции, а не то, что
// уже сейчас происходит в режиме каталога, где виден вообще весь каталог)
// и возвращает одну объединённую internal/ast.Program - тем же интерфейсом,
// что и раньше принимают cbackend.Generate/semantic.Analyze, поэтому им не
// нужно знать про модули: к моменту, когда они получают Program, все имена
// уже глобально уникальны (переименование по mangledName) и синтаксически
// корректны в изначальном, невозможном для коллизии виде.
func (p *Program) Link() (*ast.Program, error) {
	// allEnums/allConsts - имена ВСЕХ перечислений/констант, объявленных
	// хоть где-то в целой программе (across p.Modules) - см. package doc
	// rewriter.allEnums. Строятся один раз, общие для rewriter'а каждого
	// модуля ниже - только так rewriteExpr одного файла может отличить
	// "ссылается на перечисление/константу другого файла, но не
	// импортировал(а)" (ошибка видимости) от "имя не найдено вообще нигде"
	// (не наша забота, см. её же комментарий).
	allEnums := map[string]bool{}
	allConsts := map[string]bool{}
	allVars := map[string]bool{}
	for _, m := range p.Modules {
		for _, e := range m.AST.Enums {
			allEnums[e.Name] = true
		}
		for _, c := range m.AST.Consts {
			allConsts[c.Name] = true
		}
		for _, v := range m.AST.Vars {
			allVars[v.Name] = true
		}
	}
	for _, m := range p.Modules {
		names := buildNames(m, p.Entry)
		r := &rewriter{names: names, allEnums: allEnums, allConsts: allConsts, allVars: allVars}
		// Переменные модуля - значение может вызывать функции/создавать
		// сущности этого файла и импортированные - те же имена, что и в
		// теле функции.
		for _, v := range m.AST.Vars {
			r.locals = nil
			if err := r.rewriteStmt(v); err != nil {
				return nil, fmt.Errorf("%s: переменная %q: %w", m.Path, v.Name, err)
			}
		}
		for _, fn := range m.AST.Functions {
			r.locals = nil
			if err := r.rewriteTypeParams(fn.TypeParams); err != nil {
				return nil, fmt.Errorf("%s: функция %q: %w", m.Path, fn.Name, err)
			}
			r.typeParams = typeParamSet(fn.TypeParams)
			if err := r.rewriteParams(fn.Params); err != nil {
				return nil, fmt.Errorf("%s: функция %q: %w", m.Path, fn.Name, err)
			}
			resolved, err := r.rewriteReturnType(fn.ReturnType)
			if err != nil {
				return nil, fmt.Errorf("%s: функция %q: тип результата: %w", m.Path, fn.Name, err)
			}
			fn.ReturnType = resolved
			if err := r.rewriteStmts(fn.Body); err != nil {
				return nil, fmt.Errorf("%s: функция %q: %w", m.Path, fn.Name, err)
			}
			r.typeParams = nil
		}
		for _, ent := range m.AST.Entities {
			if err := r.rewriteTypeParams(ent.TypeParams); err != nil {
				return nil, fmt.Errorf("%s: сущность %q: %w", m.Path, ent.Name, err)
			}
			r.typeParams = typeParamSet(ent.TypeParams)
			if err := r.rewriteFields(ent.Fields); err != nil {
				return nil, fmt.Errorf("%s: сущность %q: %w", m.Path, ent.Name, err)
			}
			// НАЙДЕННЫЙ БАГ (исправлен здесь): явный конструктор
			// (EntityDecl.Ctor, internal/ast/constructor.go) не входил в
			// этот цикл вообще - в отличие от обычных методов (см. ниже),
			// его параметры и тело никогда не проходили rewriteParams/
			// rewriteStmts. Тип параметра конструктора, ссылающийся на
			// сущность из ДРУГОГО файла (например, "конструктор(бд: БД,
			// логгер: ЛоггерИнтерфейс)" ... "конструктор(зависимость:
			// Зависимость)", где "Зависимость" объявлена в другом файле),
			// оставался несмангленным ("неизвестный тип" у cbackend) - тот
			// же класс бага, что и у интерфейсов в НЕ-entry модуле (см.
			// TestLinkResolvesInterfaceTypeInNonEntryModule), только для
			// параметров конструктора, а не типа целиком.
			if ent.Ctor != nil {
				r.locals = nil
				if err := r.rewriteParams(ent.Ctor.Params); err != nil {
					return nil, fmt.Errorf("%s: сущность %q: конструктор: %w", m.Path, ent.Name, err)
				}
				if err := r.rewriteStmts(ent.Ctor.Body); err != nil {
					return nil, fmt.Errorf("%s: сущность %q: конструктор: %w", m.Path, ent.Name, err)
				}
			}
			for _, meth := range ent.Methods {
				r.locals = nil
				if err := r.rewriteParams(meth.Params); err != nil {
					return nil, fmt.Errorf("%s: сущность %q: метод %q: %w", m.Path, ent.Name, meth.Name, err)
				}
				resolved, err := r.rewriteReturnType(meth.ReturnType)
				if err != nil {
					return nil, fmt.Errorf("%s: сущность %q: метод %q: тип результата: %w", m.Path, ent.Name, meth.Name, err)
				}
				meth.ReturnType = resolved
				if err := r.rewriteStmts(meth.Body); err != nil {
					return nil, fmt.Errorf("%s: сущность %q: метод %q: %w", m.Path, ent.Name, meth.Name, err)
				}
			}
			r.typeParams = nil
		}
		// НАЙДЕННЫЙ БАГ (исправлен здесь): методы m.AST.Interfaces не
		// проходили через rewriteParams/rewriteReturnType вообще - тот же
		// класс бага, что и у конструктора выше (см. её комментарий), только
		// для сигнатур методов интерфейса. Пока параметр/тип результата
		// метода интерфейса мог быть только примитивом или именем другого
		// интерфейса (оба не манглятся, см. package doc unmangledCategoryName)
		// это было незаметно; как только collectInterfaces
		// (internal/semantic/interfaces.go) стал разрешать ссылки на
		// сущность/структуру программы в сигнатуре метода интерфейса, ссылка
		// на сущность из НЕ-entry модуля (мангленную) осталась бы написанной
		// её исходным, ещё не смангленным именем - "неизвестный тип" у
		// semantic, даже если сущность объявлена и видна через "использовать".
		for _, iface := range m.AST.Interfaces {
			for i := range iface.Methods {
				meth := &iface.Methods[i]
				r.locals = nil
				if err := r.rewriteParams(meth.Params); err != nil {
					return nil, fmt.Errorf("%s: интерфейс %q: метод %q: %w", m.Path, iface.Name, meth.Name, err)
				}
				resolved, err := r.rewriteReturnType(meth.ReturnType)
				if err != nil {
					return nil, fmt.Errorf("%s: интерфейс %q: метод %q: тип результата: %w", m.Path, iface.Name, meth.Name, err)
				}
				meth.ReturnType = resolved
			}
		}
		for _, ext := range m.AST.ExternFuncs {
			if err := r.rewriteParams(ext.Params); err != nil {
				return nil, fmt.Errorf("%s: внешняя функция %q: %w", m.Path, ext.Alias, err)
			}
			resolved, err := r.rewriteReturnType(ext.ReturnType)
			if err != nil {
				return nil, fmt.Errorf("%s: внешняя функция %q: тип результата: %w", m.Path, ext.Alias, err)
			}
			ext.ReturnType = resolved
		}
		// Цель синонима - обычная ссылка на тип в этом файле; параметры
		// generic-синонима - локальные заглушки, как у generic-функции.
		for _, syn := range m.AST.Synonyms {
			r.typeParams = stringSet(syn.TypeParams)
			resolved, err := r.resolveType(syn.Target)
			r.typeParams = nil
			if err != nil {
				return nil, fmt.Errorf("%s: синоним %q: %w", m.Path, syn.Name, err)
			}
			syn.Target = resolved
		}
		// ExternEntityDecl ("внешняя сущность") не несёт ссылок на другие
		// типы (нет полей/параметров) - только собственное имя, поэтому
		// здесь нет rewrite-вызова, только переименование Alias ниже.

		// Переименование собственных объявлений m - после переписывания тел
		// выше (rewriter резолвит по ИСХОДНЫМ, ещё не переименованным именам
		// из names), не до: порядок между этим блоком и предыдущим на самом
		// деле не важен (разные поля разных узлов), но так нагляднее -
		// сначала решить, что куда ссылается, потом переименовать то, на что
		// ссылаются.
		if m != p.Entry {
			for _, fn := range m.AST.Functions {
				fn.Name = mangledName(m, p.Entry, fn.Name)
			}
			for _, ent := range m.AST.Entities {
				ent.Name = mangledName(m, p.Entry, ent.Name)
				for i := range ent.States {
					ent.States[i].TypeName = mangledName(m, p.Entry, ent.States[i].TypeName)
				}
			}
			// ExternFuncDecl.Alias переименовывается тем же принципом - CName
			// (настоящий C-символ) не трогается, переименовывается только
			// Rech-facing имя, под которым функция видна в остальном коде.
			for _, ext := range m.AST.ExternFuncs {
				ext.Alias = mangledName(m, p.Entry, ext.Alias)
			}
			// ExternEntityDecl.Alias - тот же принцип: CName не трогается.
			for _, extEnt := range m.AST.ExternEntities {
				extEnt.Alias = mangledName(m, p.Entry, extEnt.Alias)
			}
		}
	}

	merged := &ast.Program{}
	for _, m := range p.Modules {
		merged.Functions = append(merged.Functions, m.AST.Functions...)
		merged.Entities = append(merged.Entities, m.AST.Entities...)
		merged.ExternFuncs = append(merged.ExternFuncs, m.AST.ExternFuncs...)
		merged.ExternEntities = append(merged.ExternEntities, m.AST.ExternEntities...)
		// Enums/Interfaces/Consts - без мангла при слиянии (в отличие от
		// функций/сущностей/внешних функций выше) - ЕДИНАЯ плоская область
		// видимости на всю программу: internal/semantic.collectEnums/
		// collectModuleConsts видят все перечисления/константы всех слитых
		// файлов сразу, по голому bare-имени, независимо от того, из какого
		// файла они пришли. "использовать" умеет импортировать все три
		// категории по имени, с приватностью (паритет с
		// уже существующей приватностью функций/сущностей) - buildNames
		// выше отражает это разрешением собственного/импортированного
		// alias в то же bare-имя, rewriteExpr (walk.go, case
		// *ast.Identifier/*ast.FieldAccess) отказывает в резолюции
		// приватному/неимпортированному имени, найденному в r.allEnums/
		// r.allConsts. Само имя при этом по-прежнему не различается по
		// файлу-источнику В МЕРЖЕ - два НЕСВЯЗАННЫХ файла с одноимённым
		// перечислением/интерфейсом/константой столкнутся здесь (collectEnums/
		// collectModuleConsts поймают коллизию, менее подробное сообщение,
		// чем у мангла выше) - осознанный, а не забытый пробел этого среза.
		merged.Enums = append(merged.Enums, m.AST.Enums...)
		merged.Interfaces = append(merged.Interfaces, m.AST.Interfaces...)
		merged.Consts = append(merged.Consts, m.AST.Consts...)
		// Переменные модуля - тем же принципом, что и константы: без мангла,
		// единая область имён на программу (одноимённые переменные двух
		// файлов - ошибка семантики). Порядок модулей - порядок
		// инициализации.
		merged.Vars = append(merged.Vars, m.AST.Vars...)
		merged.Synonyms = append(merged.Synonyms, m.AST.Synonyms...)
	}
	return merged, nil
}
