package dap

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
)

// Server - транспорт DAP поверх in/out (в реальном использовании - stdin/
// stdout процесса "rech debug", VS Code запускает его как внешний процесс
// и говорит с ним ровно этим протоколом). Владеет только чтением запросов и
// записью ответов/событий - разбор конкретных команд снаружи
// (internal/debugadapter), этот пакет ничего не знает про их смысл.
type Server struct {
	in  *bufio.Reader
	out io.Writer
	// outMu - запись должна быть потокобезопасна: событие ("stopped" при
	// остановке на точке останова) может понадобиться отправить с
	// произвольной горутины исполнения программы АСИНХРОННО, одновременно с
	// тем, как основной цикл сервера отвечает на очередной запрос.
	outMu sync.Mutex
	seq   int
}

// NewServer оборачивает in/out готовым к работе транспортом DAP.
func NewServer(in io.Reader, out io.Writer) *Server {
	return &Server{in: bufio.NewReader(in), out: out}
}

// ReadRequest блокируется до следующего запроса от клиента - io.EOF, когда
// клиент закрыл соединение (VS Code завершил сессию отладки).
func (s *Server) ReadRequest() (*Request, error) {
	for {
		raw, err := readMessage(s.in)
		if err != nil {
			return nil, err
		}
		var msg ProtocolMessage
		if err := json.Unmarshal(raw, &msg); err != nil {
			return nil, fmt.Errorf("dap: не удалось разобрать конверт сообщения: %w", err)
		}
		if msg.Type != "request" {
			// DAP - строго клиент шлёт только request; что-то иное здесь -
			// либо баг клиента, либо будущее расширение протокола, которое
			// этот сервер пока не обязан понимать. Пропускаем, не падаем.
			continue
		}
		var req Request
		if err := json.Unmarshal(raw, &req); err != nil {
			return nil, fmt.Errorf("dap: не удалось разобрать запрос: %w", err)
		}
		return &req, nil
	}
}

// SendResponse отправляет ответ на req. body - произвольная Go-структура
// (маршалится в JSON), nil - без тела. errMsg - непусто только при
// success == false.
func (s *Server) SendResponse(req *Request, success bool, body any, errMsg string) error {
	var bodyRaw json.RawMessage
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("dap: тело ответа %q: %w", req.Command, err)
		}
		bodyRaw = b
	}
	resp := Response{
		ProtocolMessage: ProtocolMessage{Type: "response"},
		RequestSeq:      req.Seq,
		Success:         success,
		Command:         req.Command,
		Message:         errMsg,
		Body:            bodyRaw,
	}
	return s.send(&resp.ProtocolMessage, &resp)
}

// SendEvent отправляет событие event (произвольная строка команды DAP -
// "stopped"/"thread"/"output"/"terminated"/"exited" и т. п.) с телом body
// (nil - без тела). Безопасно вызывать с любой горутины.
func (s *Server) SendEvent(event string, body any) error {
	var bodyRaw json.RawMessage
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("dap: тело события %q: %w", event, err)
		}
		bodyRaw = b
	}
	ev := Event{ProtocolMessage: ProtocolMessage{Type: "event"}, Event: event, Body: bodyRaw}
	return s.send(&ev.ProtocolMessage, &ev)
}

func (s *Server) send(msg *ProtocolMessage, v any) error {
	s.outMu.Lock()
	defer s.outMu.Unlock()
	s.seq++
	msg.Seq = s.seq
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(s.out, "Content-Length: %d\r\n\r\n", len(b)); err != nil {
		return err
	}
	_, err = s.out.Write(b)
	return err
}

// readMessage читает один DAP-фрейм: заголовки до пустой строки (только
// "Content-Length" реально используется - DAP допускает и другие,
// например, "Content-Type", но клиенты его не шлют), затем ровно N байт
// тела.
func readMessage(r *bufio.Reader) (json.RawMessage, error) {
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
		name, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(name), "Content-Length") {
			n, err := strconv.Atoi(strings.TrimSpace(value))
			if err != nil {
				return nil, fmt.Errorf("dap: некорректный Content-Length %q: %w", value, err)
			}
			length = n
		}
	}
	if length < 0 {
		return nil, fmt.Errorf("dap: сообщение без заголовка Content-Length")
	}
	body := make([]byte, length)
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, err
	}
	return body, nil
}
