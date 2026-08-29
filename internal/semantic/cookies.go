// Cookies поверх уже существующего HTTP (Шаг 9): "заголовки" у "Запрос"/
// "Ответ" - одна `Строка` (docs/SPEC.md, "заголовки - одна Строка... не
// Словарь<К, В>") - cookies читаются/пишутся через пару чистых функций над
// этой строкой, а не через отдельный тип с собственными методами - решение
// пользователя явно (обсуждались варианты с методами `Ответ.установить_куку`,
// отклонён - "Ответ" сейчас структура без единого метода, потребовал бы
// большого разворота дизайна ради одной фичи HTTP).
package semantic

import (
	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// cookieOptionsFields - поля встроенной сущности "ОпцииКуки" (структура,
// как "Запрос"/"Ответ"/"ДатаВремя" - копируется по значению, без общего
// состояния). Пустая строка/false - "атрибут не выставлять" для каждого
// поля, КРОМЕ "максимальный_возраст" - `?Целое`, не голое `Целое`: 0 -
// содержательное значение (Max-Age=0 - немедленное удаление cookie), не
// то же самое, что "атрибут не выставлять" (`Ничто`). Каждое поле несёт
// значение по умолчанию (см. registerCookieOptionsEntity) - "новый
// ОпцииКуки(путь: "/", ...)" называет только нужные атрибуты, остальные
// молча принимают "не выставлен".
var cookieOptionsFields = []ir.Field{
	{Name: "путь", Type: ir.TypeString},
	{Name: "домен", Type: ir.TypeString},
	{Name: "максимальный_возраст", Type: ir.NullableType(ir.TypeInt)},
	{Name: "только_http", Type: ir.TypeBool},
	{Name: "безопасный", Type: ir.TypeBool},
	{Name: "тот_же_сайт", Type: ir.TypeString},
}

// registerCookieOptionsEntity - лениво синтезирует entitySig "ОпцииКуки" -
// не через registerHTTPEntity (та не заполняет FieldDefaultExprs - ни
// одной из существующих встроенных сущностей (Запрос/Ответ/ДатаВремя) это
// не требовалось) - каждое поле здесь получает собственное значение по
// умолчанию (тем же путём, что и "имеет: поле: Тип = значение" у
// пользовательской сущности - resolveFieldDefault, internal/semantic/
// visibility.go), поэтому именованная форма конструктора может назвать
// только часть полей.
func registerCookieOptionsEntity(entities map[string]entitySig) {
	if _, exists := entities["ОпцииКуки"]; exists {
		return
	}
	sig := entitySig{}
	for _, f := range cookieOptionsFields {
		sig.FieldNames = append(sig.FieldNames, f.Name)
		sig.FieldTypes = append(sig.FieldTypes, f.Type)
		sig.FieldVisibility = append(sig.FieldVisibility, ast.Public)
	}
	sig.FieldDefaultExprs = []ast.Expr{
		&ast.StringLiteral{Value: ""},  // путь
		&ast.StringLiteral{Value: ""},  // домен
		&ast.NullLiteral{},             // максимальный_возраст
		&ast.BoolLiteral{Value: false}, // только_http
		&ast.BoolLiteral{Value: false}, // безопасный
		&ast.StringLiteral{Value: ""},  // тот_же_сайт
	}
	entities["ОпцииКуки"] = sig
}

// registerCookieFuncs регистрирует "ОпцииКуки" и функции "значение_куки"/
// "строка_куки" - тем же принципом, что и registerDateTimeFuncs (принимают/
// возвращают сущность, не примитив, поэтому не входят в internal/
// builtins.All). Безусловно, как Запрос/Ответ/ДатаВремя/Исключение (не
// лениво по первому использованию) - обе nullable-инстанциации (`?Целое`/
// `?Строка`) и сама "ОпцииКуки" дешёвы (несколько десятков строк C на
// каждую), раз в программе и так уже есть HTTP.
//
// "Expires" (второй, устаревающий способ задать срок жизни cookie) - вне
// скоупа осознанно: современная практика предпочитает "Max-Age" (имеет
// приоритет при одновременном присутствии обоих), а поддержка "Expires"
// потребовала бы форматирования HTTP-даты (RFC 1123) - отдельная, не
// обязательная здесь работа.
func registerCookieFuncs(entities map[string]entitySig, funcs map[string]funcSig) {
	registerHTTPEntity(entities, "Запрос", httpRequestFields)
	registerCookieOptionsEntity(entities)
	if _, exists := entities["?Целое"]; !exists {
		entities["?Целое"] = entitySig{IsNullable: true, ElemType: ir.TypeInt}
	}
	if _, exists := entities["?Строка"]; !exists {
		entities["?Строка"] = entitySig{IsNullable: true, ElemType: ir.TypeString}
	}
	funcs["значение_куки"] = funcSig{
		Params:     []ir.Type{ir.EntityType("Запрос"), ir.TypeString},
		ReturnType: ir.NullableType(ir.TypeString), HasReturn: true,
	}
	funcs["строка_куки"] = funcSig{
		Params:     []ir.Type{ir.TypeString, ir.TypeString, ir.EntityType("ОпцииКуки")},
		ReturnType: ir.TypeString, HasReturn: true,
	}
}
