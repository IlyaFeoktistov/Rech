package semantic

import (
	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

// Встроенные исключения рантайма (rtmsg.ErrorKinds): ОшибкаФайла,
// ОшибкаСети, ОшибкаБД, ОшибкаРазбора, ОшибкаПроцесса, ОшибкаКриптографии -
// наследники "Исключение" с его полями ("сообщение", "код") и
// конструктором "(сообщение)". Есть в любой программе: рантайм бросает их
// сам, а программа ловит по имени в любом месте.

// parseErrorEntity - исключение разбора (JSON/XML/YAML/protobuf, каст
// строки, ввод числа).
const parseErrorEntity = rtmsg.ErrorParse

// registerErrorEntities заводит сигнатуры всех встроенных исключений.
func registerErrorEntities(entities map[string]entitySig, methods map[string]map[string]funcSig) {
	for _, name := range rtmsg.ErrorKinds {
		names, types, vis := systemExceptionSigFields()
		entities[name] = entitySig{
			BaseName:        systemExceptionEntity,
			FieldNames:      names,
			FieldTypes:      types,
			FieldVisibility: vis,
			HasCtor:         true,
			CtorParams:      []ir.Type{ir.TypeString},
			CtorParamNames:  []string{"сообщение"},
		}
		methods[name] = map[string]funcSig{}
	}
}

// errorIREntities - IR всех встроенных исключений. Ctor принимает ровно
// сообщение - рантайм (rech_error_as) создаёт объект напрямую, минуя
// "новый".
func errorIREntities() []*ir.Entity {
	out := make([]*ir.Entity, 0, len(rtmsg.ErrorKinds))
	for _, name := range rtmsg.ErrorKinds {
		out = append(out, &ir.Entity{
			Name:     name,
			BaseName: systemExceptionEntity,
			Fields:   systemExceptionFields(),
			Ctor:     systemExceptionCtor(),
		})
	}
	return out
}
