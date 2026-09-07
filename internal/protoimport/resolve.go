package protoimport

import (
	"fmt"
	"strings"
)

// registry - плоский указатель по всем message/enum, встретившимся во
// всём графе импортов (см. protoimport.go, collectFiles) - ключ:
// квалифицированное точками имя БЕЗ пакета, от корня файла (например,
// "Person" - верхнеуровневое сообщение, "Person.Address" - вложенное в
// него). Пакет ".proto" (`package ...;`) сознательно не участвует в
// разрешении типов - Rech не различает конфликтующие имена по пакету
// (плоское пространство имён модуля), тот же принцип упрощения, что и у
// самого приплюснутого имени (flattenName).
type registry struct {
	messages    map[string]*message
	enums       map[string]*enumType
	rechName    map[string]string   // qualified -> имя Rech-типа (flattenName)
	simpleIndex map[string][]string // простое имя (последний сегмент) -> все qualified с ним
}

func newRegistry() *registry {
	return &registry{
		messages:    map[string]*message{},
		enums:       map[string]*enumType{},
		rechName:    map[string]string{},
		simpleIndex: map[string][]string{},
	}
}

// flattenName - "Person.Address" -> "Person_Address": у Rech-сущностей нет
// вложенной области видимости (в отличие от message ".proto") - вложенные
// типы прибиваются к верхнему уровню модуля, с именем родителя как
// префиксом, чтобы избежать столкновений двух одноимённых вложенных типов
// в разных сообщениях (C-style flattening, тот же принцип, что генераторы
// protobuf для языков без вложенных типов).
func flattenName(qualified string) string {
	return strings.ReplaceAll(qualified, ".", "_")
}

func (r *registry) addSimple(qualified string) {
	simple := qualified
	if idx := strings.LastIndex(qualified, "."); idx >= 0 {
		simple = qualified[idx+1:]
	}
	r.simpleIndex[simple] = append(r.simpleIndex[simple], qualified)
}

// register обходит messages/enums файла f (рекурсивно, вложенные тоже) и
// заносит их в реестр под квалифицированным именем - вызывается для
// КАЖДОГО файла графа импортов (порядок не важен - разрешение типов ниже
// делается отдельным проходом ПОСЛЕ того, как реестр построен целиком по
// всем файлам, чтобы порядок импортов/объявлений не влиял на то, что
// видно).
func (r *registry) register(f *file) error {
	for _, m := range f.messages {
		if err := r.registerMessage(m, ""); err != nil {
			return err
		}
	}
	for _, e := range f.enums {
		if err := r.registerEnum(e, ""); err != nil {
			return err
		}
	}
	return nil
}

func (r *registry) registerMessage(m *message, parentQualified string) error {
	qualified := m.name
	if parentQualified != "" {
		qualified = parentQualified + "." + m.name
	}
	if _, dup := r.messages[qualified]; dup {
		return fmt.Errorf("строка %d: сообщение %q объявлено более одного раза", m.line, qualified)
	}
	if _, dup := r.enums[qualified]; dup {
		return fmt.Errorf("строка %d: %q уже объявлено как enum", m.line, qualified)
	}
	r.messages[qualified] = m
	r.rechName[qualified] = flattenName(qualified)
	r.addSimple(qualified)
	for _, nested := range m.nested {
		if err := r.registerMessage(nested, qualified); err != nil {
			return err
		}
	}
	for _, e := range m.nestedEnums {
		if err := r.registerEnum(e, qualified); err != nil {
			return err
		}
	}
	return nil
}

func (r *registry) registerEnum(e *enumType, parentQualified string) error {
	qualified := e.name
	if parentQualified != "" {
		qualified = parentQualified + "." + e.name
	}
	if _, dup := r.enums[qualified]; dup {
		return fmt.Errorf("строка %d: enum %q объявлен более одного раза", e.line, qualified)
	}
	if _, dup := r.messages[qualified]; dup {
		return fmt.Errorf("строка %d: %q уже объявлено как message", e.line, qualified)
	}
	r.enums[qualified] = e
	r.rechName[qualified] = flattenName(qualified)
	r.addSimple(qualified)
	return nil
}

// refKind - что нашло разрешение имени типа поля.
type refKind int

const (
	refScalar refKind = iota
	refMessage
	refEnum
)

// resolveFieldType разрешает rawType (как написано в ".proto" - голое
// скалярное имя, простое имя типа, "Родитель.Вложенный" или
// ".абсолютный.путь") в конкретный Rech-тип поля - enclosingChain - цепочка
// квалифицированных имён от корня до НЕПОСРЕДСТВЕННОГО родителя поля
// (пусто - поле верхнеуровневого сообщения), используется для поиска "от
// ближайшей объемлющей области видимости наружу", тем же принципом, что и
// настоящий protobuf (protobuf.dev/programming-guides/proto3, "Nested
// Types").
func (r *registry) resolveFieldType(rawType string, enclosingChain []string) (rechType string, kind refKind, resolvedQualified string, err error) {
	if rechPrim, ok := scalarTypes[rawType]; ok {
		return rechPrim, refScalar, "", nil
	}
	if explicitlyUnsupportedScalarTypes[rawType] {
		return "", 0, "", fmt.Errorf("тип %q не поддержан этим импортёром - его wire-представление (zigzag/fixed-целое) не совпадает с тем, как кодек Rech уже интерпретирует varint/fixed32/fixed64 для существующих типов (см. docs/SPEC.md, раздел про PROTOBUF) - переведите поле в схеме на int32/int64/double/float, если формат допускает, либо это поле пока не переносимо", rawType)
	}

	t := rawType
	absolute := strings.HasPrefix(t, ".")
	if absolute {
		t = strings.TrimPrefix(t, ".")
	}

	var candidates []string
	if absolute {
		candidates = []string{t}
	} else {
		for i := len(enclosingChain); i >= 0; i-- {
			if i == 0 {
				candidates = append(candidates, t)
				continue
			}
			candidates = append(candidates, strings.Join(enclosingChain[:i], ".")+"."+t)
		}
	}
	for _, c := range candidates {
		if _, ok := r.messages[c]; ok {
			return r.rechName[c], refMessage, c, nil
		}
		if _, ok := r.enums[c]; ok {
			return r.rechName[c], refEnum, c, nil
		}
	}
	// Резервный вариант - совпадение по ПРОСТОМУ (последнему) сегменту
	// имени, если оно однозначно во всём графе импортов - реальные
	// контракты редко используют одинаковое короткое имя в разных
	// сообщениях, но это не гарантировано, поэтому только при полной
	// однозначности.
	simple := t
	if idx := strings.LastIndex(t, "."); idx >= 0 {
		simple = t[idx+1:]
	}
	if names, ok := r.simpleIndex[simple]; ok && len(names) == 1 {
		c := names[0]
		if _, ok := r.messages[c]; ok {
			return r.rechName[c], refMessage, c, nil
		}
		return r.rechName[c], refEnum, c, nil
	}
	return "", 0, "", fmt.Errorf("неизвестный тип поля %q (ни встроенный скаляр, ни объявленное в этом файле/импортах сообщение/enum)", rawType)
}
