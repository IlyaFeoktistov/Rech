package dap

import (
	"bytes"
	"encoding/json"
	"strconv"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/rpcframe"
)

// TestServerRequestResponseRoundTrip - клиент шлёт запрос в формате
// "Content-Length: N\r\n\r\n<JSON>", сервер должен его разобрать и суметь
// ответить в том же формате.
func TestServerRequestResponseRoundTrip(t *testing.T) {
	reqJSON := `{"seq":1,"type":"request","command":"initialize","arguments":{"clientID":"vscode"}}`
	in := bytes.NewBufferString("Content-Length: " + strconv.Itoa(len(reqJSON)) + "\r\n\r\n" + reqJSON)
	var out bytes.Buffer
	s := NewServer(in, &out)

	req, err := s.ReadRequest()
	if err != nil {
		t.Fatalf("ReadRequest: %v", err)
	}
	if req.Command != "initialize" {
		t.Fatalf("ожидалась команда initialize, получено %q", req.Command)
	}
	var args struct {
		ClientID string `json:"clientID"`
	}
	if err := json.Unmarshal(req.Arguments, &args); err != nil {
		t.Fatalf("аргументы: %v", err)
	}
	if args.ClientID != "vscode" {
		t.Fatalf("ожидался clientID=vscode, получено %q", args.ClientID)
	}

	if err := s.SendResponse(req, true, map[string]bool{"supportsConfigurationDoneRequest": true}, ""); err != nil {
		t.Fatalf("SendResponse: %v", err)
	}

	// out теперь содержит фрейм ответа - разбираем его тем же читателем,
	// что и клиент бы разобрал (проверка формата, не только содержимого).
	respServer := NewServer(&out, &bytes.Buffer{})
	raw, err := rpcframe.ReadFrame(respServer.in)
	if err != nil {
		t.Fatalf("ReadFrame ответа: %v", err)
	}
	var resp Response
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatalf("разбор ответа: %v", err)
	}
	if resp.Type != "response" || resp.Command != "initialize" || !resp.Success || resp.RequestSeq != 1 {
		t.Fatalf("неожиданный ответ: %+v", resp)
	}
}

// TestServerReadRequestReturnsEOF - клиент закрыл соединение (нет данных
// вообще) - ReadRequest должен вернуть io.EOF, не зависать и не паниковать.
func TestServerReadRequestReturnsEOF(t *testing.T) {
	s := NewServer(&bytes.Buffer{}, &bytes.Buffer{})
	if _, err := s.ReadRequest(); err == nil {
		t.Fatalf("ожидалась ошибка (EOF) на пустом вводе")
	}
}
