package lsp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"testing"
	"time"
)

// fakeClient - минимальный клиент JSON-RPC 2.0 (та же роль, что и у
// internal/debugadapter/debugadapter_test.go для DAP) поверх io.Pipe.
type fakeClient struct {
	t    *testing.T
	w    io.Writer
	r    *bufio.Reader
	id   int
	msgs chan map[string]any
}

func newFakeClient(t *testing.T) (*fakeClient, io.Reader, io.Writer) {
	clientR, serverW := io.Pipe()
	serverR, clientW := io.Pipe()
	c := &fakeClient{t: t, w: clientW, r: bufio.NewReader(clientR), msgs: make(chan map[string]any, 64)}
	go c.readLoop()
	return c, serverR, serverW
}

func (c *fakeClient) readLoop() {
	for {
		m, err := readFrame(c.r)
		if err != nil {
			close(c.msgs)
			return
		}
		c.msgs <- m
	}
}

func readFrame(r *bufio.Reader) (map[string]any, error) {
	length := -1
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		if name, value, ok := strings.Cut(line, ":"); ok && strings.EqualFold(strings.TrimSpace(name), "Content-Length") {
			n, err := strconv.Atoi(strings.TrimSpace(value))
			if err != nil {
				return nil, err
			}
			length = n
		}
	}
	body := make([]byte, length)
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, err
	}
	return m, nil
}

func (c *fakeClient) request(method string, params any) int {
	c.t.Helper()
	c.id++
	msg := map[string]any{"jsonrpc": "2.0", "id": c.id, "method": method}
	if params != nil {
		msg["params"] = params
	}
	c.send(msg)
	return c.id
}

func (c *fakeClient) notify(method string, params any) {
	c.t.Helper()
	msg := map[string]any{"jsonrpc": "2.0", "method": method}
	if params != nil {
		msg["params"] = params
	}
	c.send(msg)
}

func (c *fakeClient) send(msg map[string]any) {
	c.t.Helper()
	b, err := json.Marshal(msg)
	if err != nil {
		c.t.Fatalf("маршалинг: %v", err)
	}
	if _, err := fmt.Fprintf(c.w, "Content-Length: %d\r\n\r\n", len(b)); err != nil {
		c.t.Fatalf("запись заголовка: %v", err)
	}
	if _, err := c.w.Write(b); err != nil {
		c.t.Fatalf("запись тела: %v", err)
	}
}

func (c *fakeClient) response(timeout time.Duration) map[string]any {
	c.t.Helper()
	select {
	case m, ok := <-c.msgs:
		if !ok {
			c.t.Fatalf("сервер закрыл соединение")
		}
		return m
	case <-time.After(timeout):
		c.t.Fatalf("не дождались ответа за %s", timeout)
		return nil
	}
}

func TestLSPServerFullSession(t *testing.T) {
	client, serverR, serverW := newFakeClient(t)
	done := make(chan error, 1)
	go func() { done <- Run(serverR, serverW) }()

	client.request("initialize", map[string]any{})
	initResp := client.response(2 * time.Second)
	if initResp["result"] == nil {
		t.Fatalf("initialize неуспешен: %+v", initResp)
	}

	client.notify("textDocument/didOpen", map[string]any{
		"textDocument": map[string]any{"uri": "test://a.rech", "text": hoverTestSrc},
	})

	line := lineOf(t, hoverTestSrc, "вывести т.x")
	col := runeCol(t, "    вывести т.x", "x")
	client.request("textDocument/hover", map[string]any{
		"textDocument": map[string]any{"uri": "test://a.rech"},
		"position":     map[string]any{"line": line - 1, "character": col - 1},
	})
	hoverResp := client.response(2 * time.Second)
	result, ok := hoverResp["result"].(map[string]any)
	if !ok {
		t.Fatalf("ожидался непустой результат hover: %+v", hoverResp)
	}
	contents, ok := result["contents"].(map[string]any)
	if !ok || !strings.Contains(contents["value"].(string), "Точка.x: Целое") {
		t.Fatalf("неожиданное содержимое hover: %+v", result)
	}

	dotCol := runeCol(t, "    вывести т.x", ".")
	client.request("textDocument/completion", map[string]any{
		"textDocument": map[string]any{"uri": "test://a.rech"},
		"position":     map[string]any{"line": line - 1, "character": dotCol},
	})
	compResp := client.response(2 * time.Second)
	items, ok := compResp["result"].([]any)
	if !ok || len(items) == 0 {
		t.Fatalf("ожидался непустой список автодополнения: %+v", compResp)
	}
	found := false
	for _, it := range items {
		if it.(map[string]any)["label"] == "x" {
			found = true
		}
	}
	if !found {
		t.Fatalf("ожидалось поле x в автодополнении: %+v", items)
	}

	client.notify("textDocument/didClose", map[string]any{
		"textDocument": map[string]any{"uri": "test://a.rech"},
	})
	client.notify("exit", nil)

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run завершился с ошибкой: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("Run не завершился после exit")
	}
}

// TestLSPServerImplementationOverWire - "textDocument/implementation" по
// настоящему JSON-RPC 2.0 (не напрямую d.implementations(), см.
// implementation_test.go) - initialize должен объявить
// "implementationProvider", а сам ответ - настоящий JSON-массив локаций
// (не один Location, как у "definition").
func TestLSPServerImplementationOverWire(t *testing.T) {
	client, serverR, serverW := newFakeClient(t)
	done := make(chan error, 1)
	go func() { done <- Run(serverR, serverW) }()

	client.request("initialize", map[string]any{})
	initResp := client.response(2 * time.Second)
	caps, ok := initResp["result"].(map[string]any)["capabilities"].(map[string]any)
	if !ok || caps["implementationProvider"] != true {
		t.Fatalf("ожидалось \"implementationProvider\": true, получено: %+v", initResp)
	}

	client.notify("textDocument/didOpen", map[string]any{
		"textDocument": map[string]any{"uri": "test://impl.rech", "text": implTestSrc},
	})

	line := lineOf(t, implTestSrc, "интерфейс Логгер")
	col := runeCol(t, "интерфейс Логгер", "Логгер")
	client.request("textDocument/implementation", map[string]any{
		"textDocument": map[string]any{"uri": "test://impl.rech"},
		"position":     map[string]any{"line": line - 1, "character": col - 1},
	})
	implResp := client.response(2 * time.Second)
	locations, ok := implResp["result"].([]any)
	if !ok || len(locations) != 2 {
		t.Fatalf("ожидались 2 локации-реализации, получено: %+v", implResp)
	}

	client.notify("exit", nil)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run завершился с ошибкой: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("Run не завершился после exit")
	}
}
