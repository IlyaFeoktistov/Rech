package semantic

import "github.com/IlyaFeoktistov/Rech/internal/ir"

// datagramFields - фиксированные поля встроенной сущности "Дейтаграмма" -
// результат "получить_от" (recvfrom на udp-узле, полученном "сетевой_узел"
// БЕЗ хоста, см. registerBuiltinFuncs/internal/builtins.All) - одно
// входящее UDP-сообщение вместе с адресом ЕГО ОТПРАВИТЕЛЯ (в отличие от
// "получить" на подключённом сокете, где собеседник уже зафиксирован
// самим соединением) - "адрес"/"порт" здесь и есть тот отправитель,
// нужный, чтобы ответить именно ему через "отправить_в".
var datagramFields = []ir.Field{
	{Name: "данные", Type: ir.TypeString},
	{Name: "адрес", Type: ir.TypeString},
	{Name: "порт", Type: ir.TypeInt},
}

// registerNetworkFuncs регистрирует сущность "Дейтаграмма" и функции
// "получить_от"/"отправить_в"/"принять" - не входят в internal/
// builtins.All (там только Kind примитивов и не-nullable возврат, а
// "получить_от" возвращает сущность, "принять" - nullable "?Сокет"), тот
// же принцип, что и у "запрос"/registerDateTimeFuncs/
// "получить_переменную_окружения" (registerEnvFuncs).
func registerNetworkFuncs(entities map[string]entitySig, funcs map[string]funcSig) {
	registerHTTPEntity(entities, "Дейтаграмма", datagramFields)
	funcs["получить_от"] = funcSig{Params: []ir.Type{ir.TypeSocket, ir.TypeInt}, ReturnType: ir.EntityType("Дейтаграмма"), HasReturn: true}
	funcs["отправить_в"] = funcSig{Params: []ir.Type{ir.TypeSocket, ir.TypeString, ir.TypeString, ir.TypeInt}, HasReturn: false}

	// "принять" возвращает Ничто, если на слушающем Сокете установлен
	// таймаут ("установить_таймаут") и он истёк раньше, чем подключился
	// клиент - иначе (нет таймаута, или клиент подключился раньше) ведёт
	// себя как раньше, просто обёрнуто в "?Сокет".
	if _, exists := entities["?Сокет"]; !exists {
		entities["?Сокет"] = entitySig{IsNullable: true, ElemType: ir.TypeSocket}
	}
	funcs["принять"] = funcSig{
		Params:     []ir.Type{ir.TypeSocket},
		ReturnType: ir.NullableType(ir.TypeSocket), HasReturn: true,
	}
}
