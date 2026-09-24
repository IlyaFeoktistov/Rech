// Наследование - "сущность Б наследует А": Б получает поля и
// методы А. Ограничено ссылочными типами (не "структура" - см.
// entitySig.IsStruct, парсер уже отказывает "структура ... наследует ...",
// здесь дополнительно проверяется, что и БАЗОВАЯ сущность не структура).
// Интерфейсы ("реализует") - в internal/semantic/interfaces.go, отдельно.
package semantic

import (
	"maps"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// applyInheritance берёт уже собранные СОБСТВЕННЫЕ поля/константы (ownSig),
// обычные методы (ownMethods) и перегруженные группы методов (ownOverloads,
// имя -> её сигнатуры, funcSig.MangledName уже заполнено - непересекающееся
// с ownMethods множество имён, см. вызывающую сторону collectEntities)
// сущности ent и, если у нее есть "наследует", сплющивает их с сигнатурой
// базовой сущности (entities/methods уже содержат её - collectEntities
// вызывает это до записи ent.Name в entities, базовая сущность обязана быть
// объявлена раньше в файле, та же граница, что и у обычных полей-сущностей).
// Плоский список полей (база первой, затем собственные) - это и есть
// встраивание базовой C-структуры первым членом на уровне представления
// (см. package doc internal/backend/cbackend/vtable.go), без отдельного
// уровня вложенности. Методы: копия карты базовой сущности с наложенными
// собственными - тот же метод в обеих (переопределение) обязан совпадать по
// сигнатуре, иначе вызов через базовый тип (виртуальная диспетчеризация,
// cbackend) не мог бы полагаться на единую сигнатуру для этого слота.
// Третий результат - методOverloads этой сущности (имя перегруженного
// метода -> её сигнатуры), ТРАНЗИТИВНО сплющенный с перегрузками базовой
// (если есть) - см. package doc Analyzer.methodOverloads.
func (a *Analyzer) applyInheritance(entities map[string]entitySig, methods map[string]map[string]funcSig, ent *ast.EntityDecl, ownSig entitySig, ownMethods map[string]funcSig, ownOverloads map[string][]funcSig) (entitySig, map[string]funcSig, map[string][]funcSig, error) {
	flatMethods := ownMethods
	flat := ownSig
	flat.OwnInterfaces = ent.Interfaces
	// methodOverloads - см. package doc выше; сущность без базовой - просто
	// ownOverloads как есть (пусто, если у неё вовсе нет перегрузок).
	methodOverloads := ownOverloads

	if ent.BaseName != "" {
		baseSig, ok := entities[ent.BaseName]
		if !ok {
			return entitySig{}, nil, nil, errAtLine(ent.Line, CodeNameResolution, "сущность %q наследует %q - базовая сущность не объявлена (или объявлена позже в файле)", ent.Name, ent.BaseName)
		}
		if baseSig.IsStruct {
			return entitySig{}, nil, nil, errAtLine(ent.Line, CodeInheritance, "сущность %q не может наследовать %q - структура не участвует в наследовании (ограничено ссылочными типами)", ent.Name, ent.BaseName)
		}
		for _, name := range ownSig.FieldNames {
			if baseSig.fieldIndex(name) != -1 {
				return entitySig{}, nil, nil, errAtLine(ent.Line, CodeDuplicateDecl, "сущность %q: поле %q конфликтует с унаследованным полем базовой сущности %q", ent.Name, name, ent.BaseName)
			}
		}
		for _, name := range ownSig.ConstNames {
			if baseSig.fieldIndex(name) != -1 || baseSig.constIndex(name) != -1 {
				return entitySig{}, nil, nil, errAtLine(ent.Line, CodeDuplicateDecl, "сущность %q: константа %q конфликтует с унаследованным членом базовой сущности %q", ent.Name, name, ent.BaseName)
			}
		}

		flat = entitySig{
			FieldNames:        append(append([]string{}, baseSig.FieldNames...), ownSig.FieldNames...),
			FieldTypes:        append(append([]ir.Type{}, baseSig.FieldTypes...), ownSig.FieldTypes...),
			FieldDocs:         append(append([]string{}, baseSig.FieldDocs...), ownSig.FieldDocs...),
			FieldTags:         append(append([]string{}, baseSig.FieldTags...), ownSig.FieldTags...),
			FieldVisibility:   append(append([]ast.Visibility{}, baseSig.FieldVisibility...), ownSig.FieldVisibility...),
			FieldDefaultExprs: append(append([]ast.Expr{}, baseSig.FieldDefaultExprs...), ownSig.FieldDefaultExprs...),
			ConstNames:        append(append([]string{}, baseSig.ConstNames...), ownSig.ConstNames...),
			ConstExprs:        append(append([]ast.Expr{}, baseSig.ConstExprs...), ownSig.ConstExprs...),
			ConstDocs:         append(append([]string{}, baseSig.ConstDocs...), ownSig.ConstDocs...),
			ConstVisibility:   append(append([]ast.Visibility{}, baseSig.ConstVisibility...), ownSig.ConstVisibility...),
			States:            ownSig.States,
			StateNames:        ownSig.StateNames,
			StateBitOffset:    baseSig.StateBitOffset + len(baseSig.StateNames),
			HasStateFlags:     baseSig.HasStateFlags || len(ownSig.StateNames) > 0,
			IsStruct:          ownSig.IsStruct,
			BaseName:          ent.BaseName,
			OwnInterfaces:     ent.Interfaces,
			// HasCtor/CtorParams - НЕ из baseSig: собственный конструктор
			// каждой сущности независим, см. package doc entitySig.
			HasCtor:        ownSig.HasCtor,
			CtorParams:     ownSig.CtorParams,
			CtorParamNames: ownSig.CtorParamNames,
			// StaticField* - НЕ из baseSig, тем же принципом, что и
			// HasCtor/CtorParams выше: статический член принадлежит
			// конкретно объявившей его сущности, "Тип::имя" валиден только
			// для точного имени этого типа, никогда для потомка (см.
			// package doc ir.Entity.StaticFields).
			StaticFieldNames:        ownSig.StaticFieldNames,
			StaticFieldTypes:        ownSig.StaticFieldTypes,
			StaticFieldVisibility:   ownSig.StaticFieldVisibility,
			StaticFieldDefaultExprs: ownSig.StaticFieldDefaultExprs,
		}
		baseMethods := methods[ent.BaseName]
		baseOverloads := a.methodOverloads[ent.BaseName]
		flatMethods = make(map[string]funcSig, len(baseMethods)+len(ownMethods))
		for name, sig := range baseMethods {
			if sig.IsStatic {
				// Статический метод не наследуется - тем же принципом, что
				// и конструктор (см. package doc funcSig.IsStatic):
				// "Потомок::статМетод" не должен резолвиться в реализацию
				// базовой сущности.
				continue
			}
			if sig.SelfReturn {
				// Унаследованный без переопределения метод с "этот" как
				// типом результата - перерезолвливается в ЭТУ (ent.Name)
				// сущность заново, а не тащит ReturnType базовой сущности
				// дальше как есть - см. package doc funcSig.SelfReturn.
				sig.ReturnType = ir.EntityType(ent.Name)
			}
			flatMethods[name] = sig
		}
		for name, sig := range ownMethods {
			if baseSigM, isOverride := baseMethods[name]; isOverride && !sigsMatch(baseSigM, sig) {
				return entitySig{}, nil, nil, errAtLine(ent.Line, CodeInheritance, "сущность %q: переопределение метода %q не совпадает по сигнатуре с методом базовой сущности %q", ent.Name, name, ent.BaseName)
			}
			// Обычное (не перегруженное) переопределение метода, который в
			// базовой сущности - ГРУППА перегрузок - обратный случай
			// проверки ниже (перегрузка поверх обычного унаследованного
			// метода): непонятно, какую из перегрузок это "переопределяет".
			if _, baseOverloaded := baseOverloads[name]; baseOverloaded {
				return entitySig{}, nil, nil, errAtLine(ent.Line, CodeInheritance, "сущность %q: нельзя переопределить перегруженный метод %q - базовая сущность %q объявляет несколько перегрузок с этим именем", ent.Name, name, ent.BaseName)
			}
			flatMethods[name] = sig
		}
		for name := range ownOverloads {
			// Перегрузка ПОВЕРХ обычного унаследованного метода ИЛИ поверх
			// унаследованной группы перегрузок (с любого уровня цепочки,
			// транзитивно - baseOverloads уже несёт объединение своей и
			// унаследованной ею перегрузки) - запрещена в этой версии: было
			// бы неясно, участвует ли унаследованный вариант(ы) в общем
			// выборе перегрузки вместе с собственными.
			if _, isOverride := baseMethods[name]; isOverride {
				return entitySig{}, nil, nil, errAtLine(ent.Line, CodeInheritance, "сущность %q: нельзя перегружать метод %q - базовая сущность %q уже объявляет метод с этим именем", ent.Name, name, ent.BaseName)
			}
			if _, baseOverloaded := baseOverloads[name]; baseOverloaded {
				return entitySig{}, nil, nil, errAtLine(ent.Line, CodeInheritance, "сущность %q: нельзя перегружать метод %q - базовая сущность %q уже объявляет перегрузки с этим именем", ent.Name, name, ent.BaseName)
			}
		}
		// methodOverloads этой сущности - копия унаследованной с наложенными
		// собственными группами - имена не пересекаются с уже отклонёнными
		// выше коллизиями, наложение всегда безопасно.
		merged := make(map[string][]funcSig, len(baseOverloads)+len(ownOverloads))
		maps.Copy(merged, baseOverloads)
		maps.Copy(merged, ownOverloads)
		methodOverloads = merged
	}
	// Мангл-имена собственных перегрузок ЭТОЙ сущности - в flatMethods (уже
	// ownMethods без базовой, либо свежая карта с базовой) - см. package doc
	// funcSig.MangledName. Унаследованные перегрузки БАЗОВОЙ сущности УЖЕ в
	// flatMethods под своими мангл-именами (скопированы вместе с остальными
	// её обычными методами через baseMethods выше - методы[БазоваяСущность]
	// хранит их так же, под мангл-именем, не под голым).
	for _, group := range ownOverloads {
		for _, sig := range group {
			flatMethods[sig.MangledName] = sig
		}
	}

	for _, ifaceName := range ent.Interfaces {
		if err := checkImplements(a.interfaces, ent.Name, flatMethods, ifaceName, ent.Line); err != nil {
			return entitySig{}, nil, nil, err
		}
	}
	if total := flat.StateBitOffset + len(flat.StateNames); total > 64 {
		return entitySig{}, nil, nil, errAtLine(ent.Line, CodeInheritance, "сущность %q: суммарно %d состояний по цепочке наследования - рантайм-хранилище состояний вмещает не больше 64", ent.Name, total)
	}
	return flat, flatMethods, methodOverloads, nil
}

// isAssignable - можно ли значение типа from использовать там, где
// объявлен тип to (аргумент функции/метода, присваивание переменной, полю,
// "вернуть") - точное совпадение, как везде в языке, ИЛИ одно из двух
// отношений подтипов: from - сущность, наследующая to (напрямую или
// транзитивно), или to - интерфейс, которому from соответствует
// ("реализует", напрямую или унаследованно через BaseName). Единственные
// случаи, где Rech вообще допускает "не точное совпадение типов" без
// явного каста - оба они про ссылочные сущности/интерфейсы, не про
// примитивы (числовая промоция - отдельный механизм,
// generateIRArithmetic/analyzeArithmetic, isAssignable её не подменяет).
func isAssignable(entities map[string]entitySig, interfaces map[string]interfaceSig, from, to ir.Type) bool {
	if from == to {
		return true
	}
	if from.Kind == ir.KindTask && to.Kind == ir.KindTask {
		return from.Entity == "" || to.Entity == ""
	}
	if to.Kind == ir.KindInterface && from.Kind == ir.KindEntity {
		return entitySatisfies(entities, interfaces, from.Entity, to.Entity)
	}
	if to.Kind == ir.KindEntity && from.Kind == ir.KindEntity {
		return entityIsOrExtends(entities, from.Entity, to.Entity)
	}
	// Интерфейсное значение более узкого интерфейса (from) - там, где
	// ожидается более общий (to), которому from соответствует через
	// "наследует" - тот же принцип, что и у сущности выше, но между двумя
	// уже интерфейсными статическими типами (не через конкретную сущность).
	if to.Kind == ir.KindInterface && from.Kind == ir.KindInterface {
		return interfaceIsOrExtends(interfaces, from.Entity, to.Entity)
	}
	return false
}
