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
	"github.com/IlyaFeoktistov/Rech/internal/structtag"
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

// supportedFormats - форматы, поддержанные "разобрать"/"сериализовать".
var supportedFormats = map[string]bool{"JSON": true, "XML": true}

// analyzeParseExpr - см. package doc и ast.ParseExpr. Source обязан быть
// Строкой, целевой Тип - совместим с форматом (structCompatible).
// Регистрирует целевой тип в Program.ParseTargets/XMLParseTargets
// (registerParseTarget/registerXMLParseTarget, по e.Format) - backend
// генерирует функцию извлечения для каждого встретившегося типа (и
// рекурсивно для любого достижимого из него типа поля/элемента списка).
func analyzeParseExpr(sc *ctx, e *ast.ParseExpr) (ir.Expr, error) {
	if !supportedFormats[e.Format] {
		return nil, fmt.Errorf("строка %d: \"разобрать\" поддерживает только форматы JSON/XML, получено %q", e.Line, e.Format)
	}
	source, err := analyzeExpr(sc, e.Source)
	if err != nil {
		return nil, err
	}
	if ir.TypeOf(source).Kind != ir.KindString {
		return nil, fmt.Errorf("строка %d: \"разобрать %s\" ожидает источник типа Строка, получено %s", e.Line, e.Format, ir.TypeOf(source))
	}
	target, err := resolveTypeName(sc.entities, sc.methods, sc.aliases, sc.interfaces, e.TypeName)
	if err != nil {
		return nil, fmt.Errorf("строка %d: \"разобрать %s ... как\": %w", e.Line, e.Format, err)
	}
	if err := structCompatible(sc.entities, target, nil, e.Format); err != nil {
		return nil, fmt.Errorf("строка %d: \"разобрать %s\" в %s: %w", e.Line, e.Format, target, err)
	}
	registerParseErrorEntity(sc.entities)
	if e.Format == "XML" {
		sc.a.registerXMLParseTarget(target)
	} else {
		sc.a.registerParseTarget(target)
	}
	return &ir.ParseExpr{Format: e.Format, Source: source, To: target}, nil
}

// analyzeSerializeExpr - "сериализовать JSON/XML <выражение>" (см.
// ast.SerializeExpr) - обратное к analyzeParseExpr: Value любого
// совместимого с форматом типа (structCompatible - та же проверка, что и у
// цели разбора) сериализуется в Строка. Регистрирует тип Value в
// Program.SerializeTargets/XMLSerializeTargets (по e.Format) - backend
// генерирует функцию сериализации для каждого встретившегося типа (и
// рекурсивно для любого достижимого из него).
func analyzeSerializeExpr(sc *ctx, e *ast.SerializeExpr) (ir.Expr, error) {
	if !supportedFormats[e.Format] {
		return nil, fmt.Errorf("строка %d: \"сериализовать\" поддерживает только форматы JSON/XML, получено %q", e.Line, e.Format)
	}
	value, err := analyzeExpr(sc, e.Value)
	if err != nil {
		return nil, err
	}
	t := ir.TypeOf(value)
	if err := structCompatible(sc.entities, t, nil, e.Format); err != nil {
		return nil, fmt.Errorf("строка %d: \"сериализовать %s\" для %s: %w", e.Line, e.Format, t, err)
	}
	if e.Format == "XML" {
		sc.a.registerXMLSerializeTarget(t)
	} else {
		sc.a.registerSerializeTarget(t)
	}
	return &ir.SerializeExpr{Format: e.Format, Value: value}, nil
}

// structCompatible проверяет, что t можно заполнить из значения формата
// format (JSON/XML) или сериализовать в него: примитив из ограниченного
// набора (Целое/Дробное/Действительное/Строка/Логическое - ни JSON, ни XML
// не различают целое/дробное число отдельным синтаксисом, проверка "целое
// ли число на самом деле" происходит во время исполнения), Список<T> с
// совместимым T (T сам по себе ограничен уже существующим Шагом 4.2 -
// Список<T> поддерживает только примитивные элементы, сущности - нет, не
// специфика этой проверки), либо сущность/структура, ВСЕ поля которой
// (включая унаследованные - entitySig.FieldTypes уже сплющены, см.
// applyInheritance) совместимы (по имени поля - один в один с ключом
// JSON-объекта/именем XML-элемента, если у поля нет тега "json:..." - см.
// ir.Field.JSONKey, тогда используется его ключ вместо имени поля). Ключи,
// под которыми поля одной сущности видны формату, обязаны быть попарно
// различны (jsonFieldKey, дублирующая проверка ниже) - иначе разбор/
// сериализация не знали бы, какому полю принадлежит совпавший ключ. seen -
// защита от бесконечной рекурсии у
// самоссылающейся сущности (поле своего же типа, прямо или через цепочку)
// - на практике недостижимо сейчас (сущность не может сослаться на саму
// себя по имени поля, пока не объявлена целиком - collectEntities), но
// оставлено на случай, если это ограничение снимется (например, указатели,
// ROADMAP.md): и JSON, и XML - деревья конечной глубины, для
// самоссылающегося типа без nullable-поля нет корректного способа
// сериализовать его конечным текстом.
func structCompatible(entities map[string]entitySig, t ir.Type, seen map[string]bool, format string) error {
	switch t.Kind {
	case ir.KindInt, ir.KindFloat32, ir.KindFloat, ir.KindString, ir.KindBool:
		return nil
	case ir.KindEntity:
		sig, ok := entities[t.Entity]
		if !ok {
			return fmt.Errorf("тип %s не найден", t)
		}
		if sig.IsList {
			return structCompatible(entities, sig.ElemType, seen, format)
		}
		if sig.IsDict || sig.IsExternOpaque || sig.IsExternStruct {
			return fmt.Errorf("%s не поддержан как цель разбора %s (только примитивы, сущности/структуры и Список<T>)", t, format)
		}
		if sig.HasCtor {
			return fmt.Errorf("%s не поддержан как цель разбора %s: у сущности есть собственный \"конструктор(...)\" - разбор заполняет поля напрямую, а не вызывает пользовательский конструктор с неизвестным ему набором аргументов", t, format)
		}
		if seen[t.Entity] {
			return fmt.Errorf("%s ссылается сам на себя - самоссылающиеся типы не поддержаны как цель разбора %s", t, format)
		}
		if format == "JSON" {
			if err := checkJSONKeysDistinct(sig); err != nil {
				return fmt.Errorf("%s: %w", t, err)
			}
		}
		next := make(map[string]bool, len(seen)+1)
		for k := range seen {
			next[k] = true
		}
		next[t.Entity] = true
		for i, ft := range sig.FieldTypes {
			if err := structCompatible(entities, ft, next, format); err != nil {
				return fmt.Errorf("поле %q: %w", sig.FieldNames[i], err)
			}
		}
		return nil
	default:
		return fmt.Errorf("%s не поддержан как цель разбора %s (только примитивы, сущности/структуры и Список<T>)", t, format)
	}
}

// jsonFieldKey - внешний JSON-ключ поля i сущности sig: значение тега
// "json:..." (internal/structtag), если оно задано, иначе - само имя поля
// (тот же принцип, что и у ir.Field.JSONKey - здесь работает с entitySig,
// собранной ДО построения ir.Entity, поэтому не может переиспользовать сам
// метод напрямую).
func jsonFieldKey(sig entitySig, i int) string {
	if i < len(sig.FieldTags) {
		if key := structtag.Parse(sig.FieldTags[i]).Get("json"); key != "" {
			return key
		}
	}
	return sig.FieldNames[i]
}

// checkJSONKeysDistinct проверяет, что теги "json:..." (и имена полей без
// тега) сущности sig не сталкивают два разных поля на один и тот же внешний
// ключ - иначе разбор/сериализация не могли бы однозначно сопоставить ключ
// JSON-объекта конкретному полю.
func checkJSONKeysDistinct(sig entitySig) error {
	seen := make(map[string]string, len(sig.FieldNames))
	for i, name := range sig.FieldNames {
		key := jsonFieldKey(sig, i)
		if other, dup := seen[key]; dup {
			return fmt.Errorf("поля %q и %q конфликтуют - у обоих одинаковый JSON-ключ %q (проверьте теги `json:...`)", other, name, key)
		}
		seen[key] = name
	}
	return nil
}
