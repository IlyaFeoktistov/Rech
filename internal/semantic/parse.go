// Package semantic: "разобрать JSON <выражение> как <Тип>" (schema-first
// типизированный разбор - см. docs/SPEC.md). Целевой Тип проверяется на
// JSON-совместимость здесь (jsonCompatible); сам разбор текста и
// извлечение по типу - кодогенерация (internal/backend/cbackend/
// json_extract.go, internal/interp/json.go), эта проверка только
// гарантирует, что у кодогенерации не окажется поля/типа, для которого нет
// осмысленного соответствия в JSON.
package semantic

import (
	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/diagnostics"
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
	names, types, vis := systemExceptionSigFields()
	entities[parseErrorEntity] = entitySig{
		BaseName:        systemExceptionEntity,
		FieldNames:      names,
		FieldTypes:      types,
		FieldVisibility: vis,
		// HasCtor/CtorParams - тот же принцип, что и у "Исключение" самой
		// (analyzer.go, collectEntities) - "новая ОшибкаРазбора(сообщение)"
		// не должна требовать пустых аргументов, даже если пользователь
		// (не только rech_struct_throw) когда-нибудь создаст её сам.
		HasCtor:        true,
		CtorParams:     []ir.Type{ir.TypeString},
		CtorParamNames: []string{"сообщение"},
	}
}

// supportedFormats - форматы, поддержанные "разобрать"/"сериализовать".
// "PROTOBUF" - особый случай (internal/semantic/protobuf.go): Source/
// значение "Список<Байт>", не "Строка", проверяется отдельной веткой в
// analyzeParseExpr/analyzeSerializeExpr ниже, до общей проверки Строка.
var supportedFormats = map[string]bool{"JSON": true, "XML": true, "YAML": true, "PROTOBUF": true}

// analyzeParseExpr - см. package doc и ast.ParseExpr. Source обязан быть
// Строкой, целевой Тип - совместим с форматом (structCompatible).
// Регистрирует целевой тип в Program.ParseTargets/XMLParseTargets
// (registerParseTarget/registerXMLParseTarget, по e.Format) - backend
// генерирует функцию извлечения для каждого встретившегося типа (и
// рекурсивно для любого достижимого из него типа поля/элемента списка).
func analyzeParseExpr(sc *ctx, e *ast.ParseExpr) (ir.Expr, error) {
	if !supportedFormats[e.Format] {
		return nil, errAtLine(e.Line, CodeBuiltinTarget, "\"разобрать\" поддерживает только форматы JSON/XML/YAML/PROTOBUF, получено %q", e.Format)
	}
	source, err := analyzeExpr(sc, e.Source)
	if err != nil {
		return nil, err
	}
	if e.Format == protobufFormat {
		return analyzeProtobufParseExpr(sc, e, source)
	}
	if ir.TypeOf(source).Kind != ir.KindString {
		return nil, errAtLine(e.Line, CodeBuiltinTarget, "\"разобрать %s\" ожидает источник типа Строка, получено %s", e.Format, ir.TypeOf(source))
	}
	target, err := resolveTypeName(sc.entities, sc.methods, sc.aliases, sc.interfaces, sc.enums, e.TypeName)
	if err != nil {
		return nil, diagnostics.WrapAt(err, diagnostics.Position{File: currentFile, Line: e.Line}, "\"разобрать %s ... как\"", e.Format)
	}
	if err := structCompatible(sc.entities, target, nil, e.Format); err != nil {
		return nil, diagnostics.WrapAt(err, diagnostics.Position{File: currentFile, Line: e.Line}, "\"разобрать %s\" в %s", e.Format, target)
	}
	registerParseErrorEntity(sc.entities)
	switch e.Format {
	case "XML":
		sc.a.registerXMLParseTarget(target)
	case "YAML":
		sc.a.registerYAMLParseTarget(target)
	default:
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
		return nil, errAtLine(e.Line, CodeBuiltinTarget, "\"сериализовать\" поддерживает только форматы JSON/XML/YAML/PROTOBUF, получено %q", e.Format)
	}
	value, err := analyzeExpr(sc, e.Value)
	if err != nil {
		return nil, err
	}
	if e.Format == protobufFormat {
		return analyzeProtobufSerializeExpr(sc, value, e.Line)
	}
	// Доказанная непустота (established, "если а != Ничто {...}") -
	// НАЙДЕННОЕ И ИСПРАВЛЕННОЕ ограничение (Шаг 41/44):
	// "сериализовать JSON x" внутри такой ветки раньше не снимала обёртку
	// "?T" сама (structCompatible не принимает nullable-тип вовсе), хотя
	// употребление значения ЛЮБЫМ другим способом (аргумент/присваивание/
	// арифметика) уже разворачивало её - тот же принцип, что и у
	// maybeUnwrapNarrowed везде.
	value = maybeUnwrapNarrowed(sc, value)
	t := ir.TypeOf(value)
	if err := structCompatible(sc.entities, t, nil, e.Format); err != nil {
		return nil, diagnostics.WrapAt(err, diagnostics.Position{File: currentFile, Line: e.Line}, "\"сериализовать %s\" для %s", e.Format, t)
	}
	switch e.Format {
	case "XML":
		sc.a.registerXMLSerializeTarget(t)
	case "YAML":
		sc.a.registerYAMLSerializeTarget(t)
	default:
		sc.a.registerSerializeTarget(t)
	}
	return &ir.SerializeExpr{Format: e.Format, Value: value}, nil
}

// structCompatible проверяет, что t можно заполнить из значения формата
// format (JSON/XML) или сериализовать в него: примитив из ограниченного
// набора (Целое/Дробное/Действительное/Строка/Логическое - ни JSON, ни XML
// не различают целое/дробное число отдельным синтаксисом, проверка "целое
// ли число на самом деле" происходит во время исполнения), Список<T> с
// совместимым T (рекурсия ниже не различает T-примитив и T-сущность -
// Список<Сущность> проходит эту проверку так же, как Список<Целое>;
// единственное ограничение у сущности-элемента - она не может быть ПОЛЕМ
// другой сущности, раздел 8 SPEC.md, к цели разбора/сериализации не
// относится), либо сущность/структура, ВСЕ поля которой
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
// ): и JSON, и XML - деревья конечной глубины, для
// самоссылающегося типа без nullable-поля нет корректного способа
// сериализовать его конечным текстом.
func structCompatible(entities map[string]entitySig, t ir.Type, seen map[string]bool, format string) error {
	switch t.Kind {
	case ir.KindInt, ir.KindFloat32, ir.KindFloat, ir.KindString, ir.KindBool:
		return nil
	case ir.KindEntity:
		sig, ok := entities[t.Entity]
		if !ok {
			return errNoPos(CodeNameResolution, "тип %s не найден", t)
		}
		if sig.IsList {
			return structCompatible(entities, sig.ElemType, seen, format)
		}
		if sig.IsDict || sig.IsExternOpaque || sig.IsExternStruct {
			return errNoPos(CodeBuiltinTarget, "%s не поддержан как цель разбора %s (только примитивы, сущности/структуры и Список<T>)", t, format)
		}
		if sig.HasCtor {
			return errNoPos(CodeBuiltinTarget, "%s не поддержан как цель разбора %s: у сущности есть собственный \"конструктор(...)\" - разбор заполняет поля напрямую, а не вызывает пользовательский конструктор с неизвестным ему набором аргументов", t, format)
		}
		if seen[t.Entity] {
			return errNoPos(CodeBuiltinTarget, "%s ссылается сам на себя - самоссылающиеся типы не поддержаны как цель разбора %s", t, format)
		}
		if format == "JSON" || format == "YAML" {
			// YAML - тот же принцип "ключ: значение", что и JSON (и то же
			// поле JSONKey/тег "json:..." переименовывает ключ, отдельного
			// "yaml:..." тега не заводится - одно и то же имя поля видно
			// под одним и тем же ключом обоим форматам) - проверка
			// попарной различности ключей нужна ей ровно так же.
			if err := checkJSONKeysDistinct(sig); err != nil {
				return diagnostics.Wrap(err, "%s", t)
			}
		}
		if format == "XML" {
			if err := checkXMLFieldNames(sig); err != nil {
				return diagnostics.Wrap(err, "%s", t)
			}
		}
		next := make(map[string]bool, len(seen)+1)
		for k := range seen {
			next[k] = true
		}
		next[t.Entity] = true
		for i, ft := range sig.FieldTypes {
			if err := structCompatible(entities, ft, next, format); err != nil {
				return diagnostics.Wrap(err, "поле %q", sig.FieldNames[i])
			}
		}
		return nil
	default:
		return errNoPos(CodeBuiltinTarget, "%s не поддержан как цель разбора %s (только примитивы, сущности/структуры и Список<T>)", t, format)
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
			return errNoPos(CodeBuiltinTarget, "поля %q и %q конфликтуют - у обоих одинаковый JSON-ключ %q (проверьте теги `json:...`)", other, name, key)
		}
		seen[key] = name
	}
	return nil
}
