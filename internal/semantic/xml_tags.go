package semantic

import (
	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/structtag"
)

// xmlFieldName - внешнее имя поля i сущности sig в XML и признак атрибута
// (тег "xml:имя"/"xml:@имя"/"xml:@") - тот же structtag.XMLName, что и у
// ir.Field.XMLName: здесь entitySig собрана ДО построения ir.Entity.
func xmlFieldName(sig entitySig, i int) (string, bool) {
	tag := ""
	if i < len(sig.FieldTags) {
		tag = sig.FieldTags[i]
	}
	return structtag.XMLName(tag, sig.FieldNames[i])
}

// isXMLAttrType - атрибутом XML может быть только примитив: у значения
// атрибута нет вложенных элементов.
func isXMLAttrType(t ir.Type) bool {
	switch t.Kind {
	case ir.KindInt, ir.KindFloat32, ir.KindFloat, ir.KindString, ir.KindBool, ir.KindDecimal:
		return true
	}
	return false
}

// checkXMLFieldNames проверяет XML-отображение полей сущности sig:
// поле-атрибут - только примитив; имена дочерних элементов попарно
// различны, имена атрибутов - тоже (элемент и атрибут с одним именем не
// конфликтуют - это разные пространства в XML).
func checkXMLFieldNames(sig entitySig) error {
	elems := make(map[string]string, len(sig.FieldNames))
	attrs := make(map[string]string)
	for i, fieldName := range sig.FieldNames {
		name, attr := xmlFieldName(sig, i)
		seen := elems
		kind := "имя XML-элемента"
		if attr {
			if !isXMLAttrType(sig.FieldTypes[i]) {
				return errNoPos(CodeBuiltinTarget, "поле %q типа %s не может быть XML-атрибутом (`xml:@...`) - атрибутом может быть только Целое/Дробное/Действительное/Строка/Логическое", fieldName, sig.FieldTypes[i])
			}
			seen = attrs
			kind = "имя XML-атрибута"
		}
		if other, dup := seen[name]; dup {
			return errNoPos(CodeBuiltinTarget, "поля %q и %q конфликтуют - у обоих одинаковое %s %q (проверьте теги `xml:...`)", other, fieldName, kind, name)
		}
		seen[name] = fieldName
	}
	return nil
}
