// Package dap - протокол Debug Adapter Protocol (DAP), которым VS Code
// говорит с внешним отладчиком: JSON-сообщения (запрос/ответ/событие),
// разделённые заголовком "Content-Length" (тот же транспорт, что и у
// Language Server Protocol - оба построены на одной и той же идее заголовка
// перед телом, не JSON-RPC как таковой - у DAP/LSP свой конверт полей).
// Пакет не знает ничего про Rech - конкретные команды (setBreakpoints,
// stackTrace и т. п.) и их аргументы/тела разбирает internal/debugadapter,
// сюда попадает только транспорт и общий конверт сообщения.
package dap

import "encoding/json"

// ProtocolMessage - общие поля любого сообщения DAP (запрос/ответ/событие).
type ProtocolMessage struct {
	Seq  int    `json:"seq"`
	Type string `json:"type"` // "request" | "response" | "event"
}

// Request - запрос от клиента (VS Code) - Arguments не разобраны заранее
// (json.RawMessage), т. к. схема зависит от Command - см.
// internal/debugadapter, где Arguments декодируется в конкретный тип по
// имени команды.
type Request struct {
	ProtocolMessage
	Command   string          `json:"command"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

// Response - ответ сервера на конкретный Request (RequestSeq - Seq того
// запроса). Success == false - Message несёт текст ошибки, показываемый
// пользователю VS Code как есть.
type Response struct {
	ProtocolMessage
	RequestSeq int             `json:"request_seq"`
	Success    bool            `json:"success"`
	Command    string          `json:"command"`
	Message    string          `json:"message,omitempty"`
	Body       json.RawMessage `json:"body,omitempty"`
}

// Event - сообщение, инициированное сервером (не в ответ на запрос) -
// "stopped"/"thread"/"output"/"terminated"/"exited" и т. д. Может прийти в
// любой момент, независимо от того, ждёт ли клиент ответа на что-то ещё -
// см. Server.SendEvent (package doc transport.go про потокобезопасность
// записи).
type Event struct {
	ProtocolMessage
	Event string          `json:"event"`
	Body  json.RawMessage `json:"body,omitempty"`
}
