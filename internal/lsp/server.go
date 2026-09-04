package lsp

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"
)

// Run обслуживает одну сессию LSP поверх in/out (в реальном запуске -
// stdin/stdout процесса "rech lsp") - блокируется до "exit" или закрытия
// соединения клиентом.
func Run(in io.Reader, out io.Writer) error {
	s := &server{rpc: NewServer(in, out), docs: map[string]*doc{}}
	for {
		msg, err := s.rpc.ReadMessage()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		s.handle(msg)
		if s.exit {
			return nil
		}
	}
}

type server struct {
	rpc  *Server
	mu   sync.Mutex
	docs map[string]*doc
	exit bool
}

func (s *server) handle(msg *Message) {
	if msg.IsNotification() {
		s.handleNotification(msg)
		return
	}
	s.handleRequest(msg)
}

func (s *server) handleRequest(msg *Message) {
	switch msg.Method {
	case "initialize":
		_ = s.rpc.Reply(msg.ID, map[string]any{
			"capabilities": map[string]any{
				"textDocumentSync":       1, // Full - didChange несёт весь текст целиком, не диффы
				"hoverProvider":          true,
				"definitionProvider":     true,
				"implementationProvider": true,
				"completionProvider": map[string]any{
					"triggerCharacters": []string{"."},
				},
			},
		}, nil)
	case "shutdown":
		_ = s.rpc.Reply(msg.ID, nil, nil)
	case "textDocument/hover":
		s.handleHover(msg)
	case "textDocument/definition":
		s.handleDefinition(msg)
	case "textDocument/implementation":
		s.handleImplementation(msg)
	case "textDocument/completion":
		s.handleCompletion(msg)
	default:
		_ = s.rpc.Reply(msg.ID, nil, fmt.Errorf("метод %q не поддерживается", msg.Method))
	}
}

func (s *server) handleNotification(msg *Message) {
	switch msg.Method {
	case "exit":
		s.exit = true
	case "textDocument/didOpen":
		var p struct {
			TextDocument struct {
				URI  string `json:"uri"`
				Text string `json:"text"`
			} `json:"textDocument"`
		}
		if json.Unmarshal(msg.Params, &p) == nil {
			s.mu.Lock()
			s.docs[p.TextDocument.URI] = newDoc(p.TextDocument.URI, p.TextDocument.Text)
			s.mu.Unlock()
		}
	case "textDocument/didChange":
		var p struct {
			TextDocument struct {
				URI string `json:"uri"`
			} `json:"textDocument"`
			ContentChanges []struct {
				Text string `json:"text"`
			} `json:"contentChanges"`
		}
		if json.Unmarshal(msg.Params, &p) == nil && len(p.ContentChanges) > 0 {
			// textDocumentSync: Full (см. initialize) - клиент шлёт ровно
			// одно изменение с полным новым текстом, не список диффов.
			text := p.ContentChanges[len(p.ContentChanges)-1].Text
			s.mu.Lock()
			if d, ok := s.docs[p.TextDocument.URI]; ok {
				d.update(text)
			} else {
				s.docs[p.TextDocument.URI] = newDoc(p.TextDocument.URI, text)
			}
			s.mu.Unlock()
		}
	case "textDocument/didClose":
		var p struct {
			TextDocument struct {
				URI string `json:"uri"`
			} `json:"textDocument"`
		}
		if json.Unmarshal(msg.Params, &p) == nil {
			s.mu.Lock()
			delete(s.docs, p.TextDocument.URI)
			s.mu.Unlock()
		}
	}
}

// docPosition - общая форма параметров hover/completion:
// {textDocument: {uri}, position: {line, character}} - и line, и character
// в LSP 0-индексные, здесь сразу переводятся в 1-индексные (см. package doc
// resolve.go, lexer.Token.Line/Col).
type docPosition struct {
	TextDocument struct {
		URI string `json:"uri"`
	} `json:"textDocument"`
	Position struct {
		Line      int `json:"line"`
		Character int `json:"character"`
	} `json:"position"`
}

func (s *server) getDoc(uri string) *doc {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.docs[uri]
}

func (s *server) handleHover(msg *Message) {
	var p docPosition
	if err := json.Unmarshal(msg.Params, &p); err != nil {
		_ = s.rpc.Reply(msg.ID, nil, err)
		return
	}
	d := s.getDoc(p.TextDocument.URI)
	if d == nil {
		_ = s.rpc.Reply(msg.ID, nil, nil)
		return
	}
	text, ok := d.hover(p.Position.Line+1, p.Position.Character+1)
	if !ok {
		_ = s.rpc.Reply(msg.ID, nil, nil)
		return
	}
	_ = s.rpc.Reply(msg.ID, map[string]any{
		"contents": map[string]any{"kind": "markdown", "value": text},
	}, nil)
}

func (s *server) handleDefinition(msg *Message) {
	var p docPosition
	if err := json.Unmarshal(msg.Params, &p); err != nil {
		_ = s.rpc.Reply(msg.ID, nil, err)
		return
	}
	d := s.getDoc(p.TextDocument.URI)
	if d == nil {
		_ = s.rpc.Reply(msg.ID, nil, nil)
		return
	}
	pos, ok := d.definition(p.Position.Line+1, p.Position.Character+1)
	if !ok {
		_ = s.rpc.Reply(msg.ID, nil, nil)
		return
	}
	_ = s.rpc.Reply(msg.ID, map[string]any{
		"uri": p.TextDocument.URI,
		"range": map[string]any{
			"start": map[string]any{"line": pos.Line - 1, "character": pos.Col - 1},
			"end":   map[string]any{"line": pos.Line - 1, "character": pos.Col - 1 + pos.Len},
		},
	}, nil)
}

// handleImplementation отвечает на "textDocument/implementation" - в
// отличие от handleDefinition (ровно одна позиция), список позиций может
// содержать НЕСКОЛЬКО мест (см. package doc implementation.go) - LSP
// допускает как один Location, так и массив в ответе на этот метод, здесь
// всегда массив (пустой, если ничего не найдено - тот же контракт, что и
// "null" у handleDefinition, но для единообразия клиента проще всегда
// массив).
func (s *server) handleImplementation(msg *Message) {
	var p docPosition
	if err := json.Unmarshal(msg.Params, &p); err != nil {
		_ = s.rpc.Reply(msg.ID, nil, err)
		return
	}
	d := s.getDoc(p.TextDocument.URI)
	if d == nil {
		_ = s.rpc.Reply(msg.ID, []any{}, nil)
		return
	}
	positions, ok := d.implementations(p.Position.Line+1, p.Position.Character+1)
	if !ok {
		_ = s.rpc.Reply(msg.ID, []any{}, nil)
		return
	}
	locations := make([]any, len(positions))
	for i, pos := range positions {
		locations[i] = map[string]any{
			"uri": p.TextDocument.URI,
			"range": map[string]any{
				"start": map[string]any{"line": pos.Line - 1, "character": pos.Col - 1},
				"end":   map[string]any{"line": pos.Line - 1, "character": pos.Col - 1 + pos.Len},
			},
		}
	}
	_ = s.rpc.Reply(msg.ID, locations, nil)
}

func (s *server) handleCompletion(msg *Message) {
	var p docPosition
	if err := json.Unmarshal(msg.Params, &p); err != nil {
		_ = s.rpc.Reply(msg.ID, nil, err)
		return
	}
	d := s.getDoc(p.TextDocument.URI)
	if d == nil {
		_ = s.rpc.Reply(msg.ID, []CompletionItem{}, nil)
		return
	}
	items := d.completion(p.Position.Line+1, p.Position.Character+1)
	_ = s.rpc.Reply(msg.ID, items, nil)
}
