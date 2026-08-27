package debugadapter

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// fakeClient - минимальный клиент DAP (та же роль, что и VS Code) поверх
// io.Pipe, соединённого с debugadapter.Run - шлёт запросы, читает ответы/
// события тем же форматом Content-Length, что и настоящий VS Code.
type fakeClient struct {
	t    *testing.T
	w    io.Writer
	r    *bufio.Reader
	seq  int
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
		msg, err := readFrame(c.r)
		if err != nil {
			close(c.msgs)
			return
		}
		c.msgs <- msg
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
	if length < 0 {
		return nil, fmt.Errorf("нет Content-Length")
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

func (c *fakeClient) send(command string, args any) int {
	c.t.Helper()
	c.seq++
	msg := map[string]any{"seq": c.seq, "type": "request", "command": command}
	if args != nil {
		msg["arguments"] = args
	}
	b, err := json.Marshal(msg)
	if err != nil {
		c.t.Fatalf("маршалинг запроса %s: %v", command, err)
	}
	if _, err := fmt.Fprintf(c.w, "Content-Length: %d\r\n\r\n", len(b)); err != nil {
		c.t.Fatalf("запись заголовка: %v", err)
	}
	if _, err := c.w.Write(b); err != nil {
		c.t.Fatalf("запись тела: %v", err)
	}
	return c.seq
}

// next ждёт следующее сообщение (ответ или событие) от сервера.
func (c *fakeClient) next(timeout time.Duration) map[string]any {
	c.t.Helper()
	select {
	case m, ok := <-c.msgs:
		if !ok {
			c.t.Fatalf("сервер закрыл соединение")
		}
		return m
	case <-time.After(timeout):
		c.t.Fatalf("не дождались сообщения от сервера за %s", timeout)
		return nil
	}
}

// nextEvent ждёт событие с именем name, пропуская мимо остальные сообщения
// (ответы на запросы, другие события) - реальный DAP-обмен не гарантирует
// порядок между ответом на запрос и асинхронными событиями.
func (c *fakeClient) nextEvent(name string, timeout time.Duration) map[string]any {
	c.t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		m := c.next(timeout)
		if m["type"] == "event" && m["event"] == name {
			return m
		}
	}
	c.t.Fatalf("не дождались события %q", name)
	return nil
}

// nextResponse ждёт ответ (type == "response") - пропускает события,
// пришедшие раньше него (например, "initialized" может обогнать ответ на
// сам "initialize", если сервер шлёт его сразу же).
func (c *fakeClient) nextResponse(timeout time.Duration) map[string]any {
	c.t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		m := c.next(timeout)
		if m["type"] == "response" {
			return m
		}
	}
	c.t.Fatalf("не дождались ответа")
	return nil
}

func writeTempProgram(t *testing.T, src string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "программа.rech")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatalf("не удалось записать временную программу: %v", err)
	}
	return path
}

// TestDebugAdapterFullSession - полная последовательность DAP от initialize
// до terminated, поверх настоящего разбора+анализа файла на диске (не
// готового *ir.Program, как в internal/interp - здесь важно проверить
// именно перевод протокола, включая launch по пути к файлу).
func TestDebugAdapterFullSession(t *testing.T) {
	program := writeTempProgram(t, `
функция удвоить(число: Целое): Целое
{
    вернуть число * 2
}

функция главная
{
    переменная х = удвоить(21)
    вывести х
}
`)

	client, serverR, serverW := newFakeClient(t)
	done := make(chan error, 1)
	go func() { done <- Run(serverR, serverW, strings.NewReader("")) }()

	client.send("initialize", map[string]any{"clientID": "test"})
	initResp := client.nextResponse(2 * time.Second)
	if initResp["success"] != true {
		t.Fatalf("initialize неуспешен: %+v", initResp)
	}
	client.nextEvent("initialized", 2*time.Second)

	client.send("launch", map[string]any{"program": program})
	launchResp := client.nextResponse(2 * time.Second)
	if launchResp["success"] != true {
		t.Fatalf("launch неуспешен: %+v", launchResp)
	}

	client.send("setBreakpoints", map[string]any{
		"source":      map[string]any{"path": program},
		"breakpoints": []map[string]any{{"line": 4}},
	})
	bpResp := client.nextResponse(2 * time.Second)
	if bpResp["success"] != true {
		t.Fatalf("setBreakpoints неуспешен: %+v", bpResp)
	}

	client.send("configurationDone", nil)
	client.nextResponse(2 * time.Second)

	stopped := client.nextEvent("stopped", 2*time.Second)
	body := stopped["body"].(map[string]any)
	if body["reason"] != "breakpoint" {
		t.Fatalf("ожидалась причина breakpoint, получено %+v", body)
	}
	threadID := int(body["threadId"].(float64))

	stReq := client.send("stackTrace", map[string]any{"threadId": threadID})
	_ = stReq
	stResp := client.nextResponse(2 * time.Second)
	frames := stResp["body"].(map[string]any)["stackFrames"].([]any)
	if len(frames) != 2 {
		t.Fatalf("ожидалось 2 кадра, получено %d: %+v", len(frames), frames)
	}
	top := frames[0].(map[string]any)
	if top["name"] != "удвоить" || top["line"] != float64(4) {
		t.Fatalf("неожиданный верхний кадр: %+v", top)
	}
	frameID := int(top["id"].(float64))

	client.send("scopes", map[string]any{"frameId": frameID})
	scopesResp := client.nextResponse(2 * time.Second)
	scopes := scopesResp["body"].(map[string]any)["scopes"].([]any)
	if len(scopes) != 1 {
		t.Fatalf("ожидалась одна область видимости, получено %+v", scopes)
	}
	varsRef := int(scopes[0].(map[string]any)["variablesReference"].(float64))

	client.send("variables", map[string]any{"variablesReference": varsRef})
	varsResp := client.nextResponse(2 * time.Second)
	vars := varsResp["body"].(map[string]any)["variables"].([]any)
	if len(vars) != 1 {
		t.Fatalf("ожидалась одна переменная, получено %+v", vars)
	}
	v := vars[0].(map[string]any)
	if v["name"] != "число" || v["value"] != "21" {
		t.Fatalf("неожиданная переменная: %+v", v)
	}

	client.send("evaluate", map[string]any{"expression": "число * 10", "frameId": frameID})
	evalResp := client.nextResponse(2 * time.Second)
	if evalResp["body"].(map[string]any)["result"] != "210 (Целое)" {
		t.Fatalf("неожиданный результат evaluate: %+v", evalResp)
	}

	client.send("continue", map[string]any{"threadId": threadID})
	client.nextResponse(2 * time.Second)

	client.nextEvent("exited", 2*time.Second)
	client.nextEvent("terminated", 2*time.Second)

	client.send("disconnect", nil)
	client.nextResponse(2 * time.Second)

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run завершился с ошибкой: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("Run не завершился после disconnect")
	}
}
