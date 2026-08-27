// Package lsp - минимальный сервер Language Server Protocol для Rech
// ("rech lsp") - hover и автодополнение (doc-комментарии, шаг 19 -
// продолжение отладчика). Общий транспорт с DAP - internal/rpcframe
// (заголовок Content-Length), сама форма сообщения - обычный JSON-RPC 2.0
// ("jsonrpc"/"method"/"params"/"id"), не конверт DAP (internal/dap) - две
// разные, хоть и похожие, спецификации, поэтому свой пакет, не переиспользование
// internal/dap целиком.
//
// Не полная реализация LSP - только то, что нужно hover/completion (см.
// server.go): initialize/initialized, textDocument/didOpen-didChange-
// didClose, textDocument/hover, textDocument/completion, shutdown/exit.
package lsp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"sync"

	"github.com/IlyaFeoktistov/Rech/internal/rpcframe"
)

// Message - конверт JSON-RPC 2.0: запрос/уведомление от клиента к серверу
// несёт Method (+ Params), запрос дополнительно ID (уведомление - без ID,
// ответа не ждёт). Тот же тип читает и запросы, и уведомления - различаются
// только наличием ID.
type Message struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// IsNotification - true, если у сообщения нет ID (уведомление, не запрос -
// ответ на него не нужен и не ожидается).
func (m *Message) IsNotification() bool {
	return len(m.ID) == 0
}

// rpcError - "error" JSON-RPC 2.0.
type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type notification struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

// Server - транспорт JSON-RPC 2.0 поверх in/out (в реальном запуске -
// stdin/stdout процесса "rech lsp") - как internal/dap.Server, но со своей
// формой сообщения.
type Server struct {
	in    *bufio.Reader
	out   io.Writer
	outMu sync.Mutex
}

func NewServer(in io.Reader, out io.Writer) *Server {
	return &Server{in: bufio.NewReader(in), out: out}
}

// ReadMessage блокируется до следующего запроса/уведомления - io.EOF, когда
// клиент закрыл соединение.
func (s *Server) ReadMessage() (*Message, error) {
	raw, err := rpcframe.ReadFrame(s.in)
	if err != nil {
		return nil, err
	}
	var msg Message
	if err := json.Unmarshal(raw, &msg); err != nil {
		return nil, fmt.Errorf("lsp: не удалось разобрать сообщение: %w", err)
	}
	return &msg, nil
}

// Reply отвечает на запрос с ID id - result (успех, errObj == nil) или
// errObj (неуспех, result игнорируется).
func (s *Server) Reply(id json.RawMessage, result any, errObj error) error {
	resp := response{JSONRPC: "2.0", ID: id}
	if errObj != nil {
		resp.Error = &rpcError{Code: -32603, Message: errObj.Error()}
	} else {
		resp.Result = result
	}
	s.outMu.Lock()
	defer s.outMu.Unlock()
	return rpcframe.WriteFrame(s.out, &resp)
}

// Notify отправляет уведомление серверу->клиенту (не используется этим
// минимальным сервером сейчас, кроме как в будущем для диагностики -
// оставлено для полноты транспорта).
func (s *Server) Notify(method string, params any) error {
	s.outMu.Lock()
	defer s.outMu.Unlock()
	return rpcframe.WriteFrame(s.out, &notification{JSONRPC: "2.0", Method: method, Params: params})
}
