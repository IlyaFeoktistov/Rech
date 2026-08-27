// Наследование - "сущность Б наследует А": Б получает поля и
// методы А. Ограничено ссылочными типами (не "структура" - см.
// entitySig.IsStruct, парсер уже отказывает "структура ... наследует ...",
// здесь дополнительно проверяется, что и БАЗОВАЯ сущность не структура).
// Интерфейсы ("реализует") - в internal/semantic/interfaces.go, отдельно.
package semantic

import (
	"fmt"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// applyInheritance берёт уже собранные СОБСТВЕННЫЕ поля/константы (ownSig)
// и методы (ownMethods) сущности ent и, если у нее есть "наследует",
// сплющивает их с сигнатурой базовой сущности (entities/methods уже
// содержат её - collectEntities вызывает это до записи ent.Name в entities,
// базовая сущность обязана быть объявлена раньше в файле, та же граница,
// что и у обычных полей-сущностей). Плоский список полей (база первой,
// затем собственные) - это и есть встраивание базовой C-структуры первым
// членом на уровне представления (см. package doc
// internal/backend/cbackend/vtable.go), без отдельного уровня вложенности.
// Методы: копия карты базовой сущности с наложенными собственными - тот же
// метод в обеих (переопределение) обязан совпадать по сигнатуре, иначе
// вызов через базовый тип (виртуальная диспетчеризация, cbackend) не мог
// бы полагаться на единую сигнатуру для этого слота.
func (a *Analyzer) applyInheritance(entities map[string]entitySig, methods map[string]map[string]funcSig, ent *ast.EntityDecl, ownSig entitySig, ownMethods map[string]funcSig) (entitySig, map[string]funcSig, error) {
	flatMethods := ownMethods
	flat := ownSig
	flat.OwnInterfaces = ent.Interfaces

	if ent.BaseName != "" {
		baseSig, ok := entities[ent.BaseName]
		if !ok {
			return entitySig{}, nil, fmt.Errorf("строка %d: сущность %q наследует %q - базовая сущность не объявлена (или объявлена позже в файле)", ent.Line, ent.Name, ent.BaseName)
		}
		if baseSig.IsStruct {
			return entitySig{}, nil, fmt.Errorf("строка %d: сущность %q не может наследовать %q - структура не участвует в наследовании (ограничено ссылочными типами)", ent.Line, ent.Name, ent.BaseName)
		}
		for _, name := range ownSig.FieldNames {
			if baseSig.fieldIndex(name) != -1 {
				return entitySig{}, nil, fmt.Errorf("строка %d: сущность %q: поле %q конфликтует с унаследованным полем базовой сущности %q", ent.Line, ent.Name, name, ent.BaseName)
			}
		}
		for _, name := range ownSig.ConstNames {
			if baseSig.fieldIndex(name) != -1 || baseSig.constIndex(name) != -1 {
				return entitySig{}, nil, fmt.Errorf("строка %d: сущность %q: константа %q конфликтует с унаследованным членом базовой сущности %q", ent.Line, ent.Name, name, ent.BaseName)
			}
		}

		flat = entitySig{
			FieldNames:      append(append([]string{}, baseSig.FieldNames...), ownSig.FieldNames...),
			FieldTypes:      append(append([]ir.Type{}, baseSig.FieldTypes...), ownSig.FieldTypes...),
			FieldDocs:       append(append([]string{}, baseSig.FieldDocs...), ownSig.FieldDocs...),
			FieldVisibility: append(append([]ast.Visibility{}, baseSig.FieldVisibility...), ownSig.FieldVisibility...),
			ConstNames:      append(append([]string{}, baseSig.ConstNames...), ownSig.ConstNames...),
			ConstExprs:      append(append([]ast.Expr{}, baseSig.ConstExprs...), ownSig.ConstExprs...),
			ConstDocs:       append(append([]string{}, baseSig.ConstDocs...), ownSig.ConstDocs...),
			ConstVisibility: append(append([]ast.Visibility{}, baseSig.ConstVisibility...), ownSig.ConstVisibility...),
			States:          ownSig.States,
			IsStruct:        ownSig.IsStruct,
			BaseName:        ent.BaseName,
			OwnInterfaces:   ent.Interfaces,
			// HasCtor/CtorParams - НЕ из baseSig: собственный конструктор
			// каждой сущности независим, см. package doc entitySig.
			HasCtor:    ownSig.HasCtor,
			CtorParams: ownSig.CtorParams,
		}

		baseMethods := methods[ent.BaseName]
		flatMethods = make(map[string]funcSig, len(baseMethods)+len(ownMethods))
		for name, sig := range baseMethods {
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
				return entitySig{}, nil, fmt.Errorf("строка %d: сущность %q: переопределение метода %q не совпадает по сигнатуре с методом базовой сущности %q", ent.Line, ent.Name, name, ent.BaseName)
			}
			flatMethods[name] = sig
		}
	}

	for _, ifaceName := range ent.Interfaces {
		if err := checkImplements(a.interfaces, ent.Name, flatMethods, ifaceName, ent.Line); err != nil {
			return entitySig{}, nil, err
		}
	}
	return flat, flatMethods, nil
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
func isAssignable(entities map[string]entitySig, from, to ir.Type) bool {
	if from == to {
		return true
	}
	if to.Kind == ir.KindInterface && from.Kind == ir.KindEntity {
		return entitySatisfies(entities, from.Entity, to.Entity)
	}
	if to.Kind == ir.KindEntity && from.Kind == ir.KindEntity {
		return entityIsOrExtends(entities, from.Entity, to.Entity)
	}
	return false
}
