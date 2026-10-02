package semantic

import (
	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// ProcessFields - поля встроенной сущности "Процесс" (результат "запустить_
// процесс") - только "pid": этого достаточно, чтобы использовать уже
// существующий "отправить_сигнал(pid, сигнал)" для завершения дочернего
// процесса - отдельного метода вроде "завершить" здесь нарочно нет,
// отправка сигнала уже есть как общая функция, дублировать её под другим
// именем только потому, что pid теперь приходит из сущности, а не
// напрямую от пользователя, незачем.
var ProcessFields = []ir.Field{
	{Name: "pid", Type: ir.TypeInt},
}

// CommandResultFields - поля встроенной сущности "РезультатКоманды"
// (результат "выполнить_команду").
var CommandResultFields = []ir.Field{
	{Name: "вывод", Type: ir.TypeString},
	{Name: "ошибка", Type: ir.TypeString},
	{Name: "код_возврата", Type: ir.TypeInt},
}

// subprocessDirDefault - значение по умолчанию второго аргумента
// "выполнить_команду" ("" - унаследовать текущий рабочий каталог самой
// программы, ничего не менять).
var subprocessDirDefault = &ast.StringLiteral{Value: ""}

// registerSubprocessFuncs регистрирует сущности "Процесс"/"РезультатКоманды"
// и три встроенные функции запуска внешних процессов (docs/SPEC.md):
//   - "запустить_процесс(команда: Строка, аргументы: Список<Строка>):
//     Процесс" - неблокирующий spawn БЕЗ системной оболочки (execvp/
//     CreateProcess напрямую по имени программы + готовому списку
//     аргументов, без разбора пайпов/"&&"/подстановок) - возвращает pid
//     сразу, не дожидаясь завершения.
//   - "дождаться_процесса(процесс: Процесс): Целое" - блокируется до
//     завершения РАНЕЕ запущенного "запустить_процесс", возвращает код
//     возврата. Обязателен для процесса, с которым не покончили иначе
//     (без него завершившийся потомок остаётся зомби вплоть до выхода
//     самой программы - POSIX).
//   - "выполнить_команду(команда: Строка, каталог: Строка = ""):
//     РезультатКоманды" - блокирующий запуск ЧЕРЕЗ системную оболочку
//     (sh -c/cmd /C) - в отличие от "запустить_процесс", понимает пайпы/
//     "&&"/подстановку, как в терминале - сразу отдаёт и вывод (отдельно
//     stdout/stderr), и код возврата, ждать отдельно не нужно. "каталог" -
//     рабочий каталог ЭТОЙ команды (пустая строка - унаследовать текущий
//     рабочий каталог программы, не менять его).
func registerSubprocessFuncs(entities map[string]entitySig, funcs map[string]funcSig) {
	registerHTTPEntity(entities, "Процесс", ProcessFields)
	registerHTTPEntity(entities, "РезультатКоманды", CommandResultFields)
	funcs["запустить_процесс"] = funcSig{
		Params:     []ir.Type{ir.TypeString, ir.EntityType(splitResultType)},
		ReturnType: ir.EntityType("Процесс"),
		HasReturn:  true,
	}
	funcs["дождаться_процесса"] = funcSig{
		Params:     []ir.Type{ir.EntityType("Процесс")},
		ReturnType: ir.TypeInt,
		HasReturn:  true,
	}
	funcs["выполнить_команду"] = funcSig{
		Params:        []ir.Type{ir.TypeString, ir.TypeString},
		ParamDefaults: []ast.Expr{nil, subprocessDirDefault},
		ReturnType:    ir.EntityType("РезультатКоманды"),
		HasReturn:     true,
	}
}

// subprocessFuncNames - функции, вместе с которыми cbackend эмитит весь
// рантайм процессов (runtime/subprocess) - а он объявляет
// rech_spawn_process(..., Список<Строка> *), значит тип нужен любой из них,
// не только "запустить_процесс" (иначе программа с одним
// "выполнить_команду" не компилировалась).
var subprocessFuncNames = map[string]bool{
	"запустить_процесс":  true,
	"выполнить_команду":  true,
	"дождаться_процесса": true,
}

// registerSubprocessTypesOnCall - вызывается из analyzeCallExpr при каждом
// вызове функции из subprocessFuncNames - Список<Строка> (generic-коллекция,
// свой typedef/методы в cbackend) - тем же
// принципом, что и registerSplitTypeOnCall/registerDBTypesOnCall.
// resolveTypeName сам идемпотентен, поэтому безопасно звать на каждый
// вызов, не только на первый.
func registerSubprocessTypesOnCall(entities map[string]entitySig, methods map[string]map[string]funcSig, aliases map[string]string, interfaces map[string]interfaceSig, enums map[string]map[string]int64) error {
	_, err := resolveTypeName(entities, methods, aliases, interfaces, enums, splitResultType)
	return err
}
