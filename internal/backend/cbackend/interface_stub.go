package cbackend

import (
	"fmt"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// descriptorAlias - псевдоним непрозрачного дескриптора экспортированной
// сущности внутри generateInterfaceStub ("ТочкаДескриптор") - освобождает
// настоящее имя сущности ("Точка") для автоген обёрточной сущности ниже,
// тем же приёмом, что и ручной пример examples/проекты/
// экспорт_библиотеки_потребитель/главный.rech.
func descriptorAlias(entity string) string {
	return entity + "Дескриптор"
}

// rechParamList - список параметров в синтаксисе Rech ("имя: Тип, ..."),
// общий формат и для "внешняя функция" (methodCSymbol(...)), и для
// автоген-обёртки (обычный метод/конструктор) - оба принимают ОДИНАКОВЫЕ
// имена параметров, разница только в теле.
func rechParamList(params []ir.Param) string {
	parts := make([]string, len(params))
	for i, p := range params {
		parts[i] = fmt.Sprintf("%s: %s", p.Name, p.Type.String())
	}
	return strings.Join(parts, ", ")
}

// rechArgList - те же параметры, вызовом по именам ("имя1, имя2") - список
// аргументов автоген-обёртки при вызове соответствующей "внешняя функция".
func rechArgList(params []ir.Param) string {
	parts := make([]string, len(params))
	for i, p := range params {
		parts[i] = p.Name
	}
	return strings.Join(parts, ", ")
}

// rechReturnSuffix - ": Тип" после списка параметров, "" - без результата.
func rechReturnSuffix(hasReturn bool, t ir.Type) string {
	if !hasReturn {
		return ""
	}
	return ": " + t.String()
}

// receiverParamName - имя параметра-получателя во "внешняя функция" метода
// (дескриптор сущности первым параметром - см. package doc
// generateInterfaceStub) - "получатель", а не "этот" (зарезервировано,
// ThisExpr валиден только внутри тела метода, не в списке параметров
// внешней функции).
const receiverParamName = "получатель"

// generateInterfaceStub строит "<имя>.rechi" - публичный интерфейс
// собранной библиотеки (`rech build --lib`) В СИНТАКСИСЕ САМОГО Rech,
// рядом с C-заголовком (generateHeader) - без единой ручной строки FFI на
// стороне потребителя (docs/SPEC.md, "Закрытые (бинарные) пакеты"):
// непрозрачный дескриптор ("внешняя сущность ... как ...Дескриптор") +
// "внешняя функция" на настоящие (не транслитерированные) C-символы
// экспортированных функций/методов/конструктора + автоген обёрточная
// сущность с ТЕМ ЖЕ именем и ТЕМИ ЖЕ методами, что у оригинала - каждый
// метод/конструктор обёртки просто зовёт соответствующую "внешняя
// функция", ровно тем кодом, который сегодня пишет вручную потребитель
// (examples/проекты/экспорт_библиотеки_потребитель/главный.rech). Строится
// из тех же данных ir.Program, что уже собирает generateHeader - разница
// только в целевом синтаксисе (Rech, не C) - оба обязаны видеть один и тот
// же набор экспортированных объявлений, поэтому берут его из ОДНОГО
// прохода по prog.Functions/prog.Entities, без отдельной сверки.
func generateInterfaceStub(prog *ir.Program) (string, error) {
	var sb strings.Builder
	sb.WriteString("// Автоматически сгенерировано компилятором rech (rech build --lib). Не редактировать руками.\n")
	sb.WriteString("// Публичный интерфейс библиотеки в синтаксисе Rech - \"использовать\" подключает его вместо ручного FFI.\n\n")

	for _, fn := range prog.Functions {
		if !fn.Exported {
			continue
		}
		fmt.Fprintf(&sb, "внешняя функция %s(%s)%s\n", fn.Name, rechParamList(fn.Params), rechReturnSuffix(fn.HasReturn, fn.ReturnType))
	}

	for _, ent := range prog.Entities {
		if !ent.Exported {
			continue
		}
		hasCtor := ent.Ctor != nil && ent.Ctor.Exported
		exportedMethods := make([]*ir.Method, 0, len(ent.Methods))
		for _, m := range ent.Methods {
			if m.Exported {
				exportedMethods = append(exportedMethods, m)
			}
		}

		alias := descriptorAlias(ent.Name)
		fmt.Fprintf(&sb, "внешняя сущность %s как %s\n", ent.Name, alias)
		if hasCtor {
			fmt.Fprintf(&sb, "внешняя функция %s(%s): %s\n", exportedCtorCSymbol(ent.Name), rechParamList(ent.Ctor.Params), alias)
		}
		receiver := ir.Param{Name: receiverParamName, Type: ir.Type{Kind: ir.KindEntity, Entity: alias}}
		for _, m := range exportedMethods {
			params := append([]ir.Param{receiver}, m.Params...)
			fmt.Fprintf(&sb, "внешняя функция %s(%s)%s\n", exportedMethodCSymbol(ent.Name, m.Name), rechParamList(params), rechReturnSuffix(m.HasReturn, m.ReturnType))
		}
		sb.WriteString("\n")

		if !hasCtor && len(exportedMethods) == 0 {
			// Дескриптор без единого доступного снаружи члена - обёртка
			// была бы пустой сущностью без способа её создать/использовать,
			// потребителю ничего не даёт (docs/SPEC.md, "Закрытые
			// (бинарные) пакеты", границы среза) - только "внешняя сущность"
			// выше, без автоген обёртки.
			continue
		}
		fmt.Fprintf(&sb, "сущность %s\n{\n\tимеет:\n\t\tприватный дескриптор: %s\n\n", ent.Name, alias)
		if hasCtor {
			fmt.Fprintf(&sb, "\tконструктор(%s)\n\t{\n\t\tэтот.дескриптор = %s(%s)\n\t}\n\n", rechParamList(ent.Ctor.Params), exportedCtorCSymbol(ent.Name), rechArgList(ent.Ctor.Params))
		}
		for _, m := range exportedMethods {
			args := "этот.дескриптор"
			if len(m.Params) > 0 {
				args += ", " + rechArgList(m.Params)
			}
			call := fmt.Sprintf("%s(%s)", exportedMethodCSymbol(ent.Name, m.Name), args)
			if m.HasReturn {
				fmt.Fprintf(&sb, "\tпубличный %s(%s): %s\n\t{\n\t\tвернуть %s\n\t}\n\n", m.Name, rechParamList(m.Params), m.ReturnType.String(), call)
			} else {
				fmt.Fprintf(&sb, "\tпубличный %s(%s)\n\t{\n\t\t%s\n\t}\n\n", m.Name, rechParamList(m.Params), call)
			}
		}
		sb.WriteString("}\n\n")
	}
	return sb.String(), nil
}
