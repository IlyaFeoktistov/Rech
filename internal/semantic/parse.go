// Package semantic: "разобрать JSON <выражение> как <Тип>" (schema-first
// типизированный разбор - см. docs/SPEC.md). Целевой Тип проверяется на
// JSON-совместимость здесь (jsonCompatible); сам разбор текста и
// извлечение по типу - кодогенерация (internal/backend/cbackend/
// json_extract.go, internal/interp/json.go), эта проверка только
// гарантирует, что у кодогенерации не окажется поля/типа, для которого нет
// осмысленного соответствия в JSON.
package semantic

import (
	"fmt"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// parseErrorEntity - встроенное исключение, выбрасываемое "разобрать JSON"
// при ошибке (некорректный текст, отсутствующее поле, несовпадение типа) -
// наследник systemExceptionEntity. В отличие от Запрос/Ответ/ДатаВремя,
// регистрируется НЕ безусловно в каждой программе (registerParseErrorEntity
// вызывается только из analyzeParseExpr, при первом "разобрать") - иначе
// любая программа, ни разу не использующая эту конструкцию, платила бы за
// участие "Исключение" в диспетчеризации (vtable/isa-таблица - см.
// internal/backend/cbackend/vtable.go, computeDispatch) без всякой причины.
const parseErrorEntity = "ОшибкаРазбора"

func registerParseErrorEntity(entities map[string]entitySig) {
	if _, exists := entities[parseErrorEntity]; exists {
		return
	}
	entities[parseErrorEntity] = entitySig{
		BaseName:        systemExceptionEntity,
		FieldNames:      []string{"сообщение"},
		FieldTypes:      []ir.Type{ir.TypeString},
		FieldVisibility: []ast.Visibility{ast.Public},
	}
}

// analyzeParseExpr - см. package doc и ast.ParseExpr. Source обязан быть
// Строкой, целевой Тип - JSON-совместим (jsonCompatible). Регистрирует
// целевой тип в Program.ParseTargets (registerParseTarget) - cbackend
// генерирует функцию извлечения для каждого встретившегося типа (и
// рекурсивно для любого достижимого из него типа поля/элемента списка).
func analyzeParseExpr(sc *ctx, e *ast.ParseExpr) (ir.Expr, error) {
	if e.Format != "JSON" {
		return nil, fmt.Errorf("строка %d: \"разобрать\" поддерживает только формат JSON, получено %q", e.Line, e.Format)
	}
	source, err := analyzeExpr(sc, e.Source)
	if err != nil {
		return nil, err
	}
	if ir.TypeOf(source).Kind != ir.KindString {
		return nil, fmt.Errorf("строка %d: \"разобрать JSON\" ожидает источник типа Строка, получено %s", e.Line, ir.TypeOf(source))
	}
	target, err := resolveTypeName(sc.entities, sc.methods, sc.aliases, sc.interfaces, e.TypeName)
	if err != nil {
		return nil, fmt.Errorf("строка %d: \"разобрать JSON ... как\": %w", e.Line, err)
	}
	if err := jsonCompatible(sc.entities, target, nil); err != nil {
		return nil, fmt.Errorf("строка %d: \"разобрать JSON\" в %s: %w", e.Line, target, err)
	}
	registerParseErrorEntity(sc.entities)
	sc.a.registerParseTarget(target)
	return &ir.ParseExpr{Format: e.Format, Source: source, To: target}, nil
}

// jsonCompatible проверяет, что t можно заполнить из JSON-значения:
// примитив из ограниченного набора (Целое/Дробное/Действительное/Строка/
// Логическое - JSON не различает целое и дробное число отдельным типом
// синтаксически, проверка "целое ли число на самом деле" происходит во
// время исполнения, см. package doc internal/interp/json.go), Список<T> с
// JSON-совместимым T (T сам по себе ограничен уже существующим Шагом 4.2 -
// Список<T> поддерживает только примитивные элементы, сущности - нет, не
// специфика JSON), либо сущность/структура, ВСЕ поля которой (включая
// унаследованные - entitySig.FieldTypes уже сплющены, см. applyInheritance)
// JSON-совместимы (по имени поля - один в один с ключом JSON-объекта, без
// отдельного механизма переименования в этом срезе). seen - защита от
// бесконечной рекурсии у самоссылающейся сущности (поле своего же типа,
// прямо или через цепочку) - на практике недостижимо сейчас (сущность не
// может сослаться на саму себя по имени поля, пока не объявлена целиком -
// collectEntities), но оставлено на случай, если это ограничение снимется
// (например, указатели, ROADMAP.md): JSON - дерево конечной глубины, для
// самоссылающегося типа без nullable-поля нет корректного способа
// сериализовать его конечным текстом.
func jsonCompatible(entities map[string]entitySig, t ir.Type, seen map[string]bool) error {
	switch t.Kind {
	case ir.KindInt, ir.KindFloat32, ir.KindFloat, ir.KindString, ir.KindBool:
		return nil
	case ir.KindEntity:
		sig, ok := entities[t.Entity]
		if !ok {
			return fmt.Errorf("тип %s не найден", t)
		}
		if sig.IsList {
			return jsonCompatible(entities, sig.ElemType, seen)
		}
		if sig.IsDict || sig.IsExternOpaque || sig.IsExternStruct {
			return fmt.Errorf("%s не поддержан как цель разбора JSON (только примитивы, сущности/структуры и Список<T>)", t)
		}
		if sig.HasCtor {
			return fmt.Errorf("%s не поддержан как цель разбора JSON: у сущности есть собственный \"конструктор(...)\" - разбор заполняет поля напрямую из JSON-объекта, а не вызывает пользовательский конструктор с неизвестным ему набором аргументов", t)
		}
		if seen[t.Entity] {
			return fmt.Errorf("%s ссылается сам на себя - самоссылающиеся типы не поддержаны как цель разбора JSON", t)
		}
		next := make(map[string]bool, len(seen)+1)
		for k := range seen {
			next[k] = true
		}
		next[t.Entity] = true
		for i, ft := range sig.FieldTypes {
			if err := jsonCompatible(entities, ft, next); err != nil {
				return fmt.Errorf("поле %q: %w", sig.FieldNames[i], err)
			}
		}
		return nil
	default:
		return fmt.Errorf("%s не поддержан как цель разбора JSON (только примитивы, сущности/структуры и Список<T>)", t)
	}
}
