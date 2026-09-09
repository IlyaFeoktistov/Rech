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
				"textDocumentSync":          1, // Full - didChange несёт весь текст целиком, не диффы
				"hoverProvider":             true,
				"definitionProvider":        true,
				"implementationProvider":    true,
				"referencesProvider":        true,
				"documentHighlightProvider": true,
				"codeActionProvider":        true,
				"codeLensProvider":          map[string]any{},
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
	case "textDocument/references":
		s.handleReferences(msg)
	case "textDocument/documentHighlight":
		s.handleDocumentHighlight(msg)
	case "textDocument/codeAction":
		s.handleCodeAction(msg)
	case "textDocument/codeLens":
		s.handleCodeLens(msg)
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
			d := newDoc(p.TextDocument.URI, p.TextDocument.Text)
			s.docs[p.TextDocument.URI] = d
			s.mu.Unlock()
			s.publishDiagnostics(p.TextDocument.URI, d)
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
			d, ok := s.docs[p.TextDocument.URI]
			if ok {
				d.update(text)
			} else {
				d = newDoc(p.TextDocument.URI, text)
				s.docs[p.TextDocument.URI] = d
			}
			s.mu.Unlock()
			s.publishDiagnostics(p.TextDocument.URI, d)
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
			// Снимает подчёркивание, если оно было - закрытый документ
			// больше не должен показывать диагностику в списке "Проблемы".
			_ = s.rpc.Notify("textDocument/publishDiagnostics", map[string]any{
				"uri": p.TextDocument.URI, "diagnostics": []any{},
			})
		}
	}
}

// publishDiagnostics отправляет "textDocument/publishDiagnostics" для d -
// d.diagnostics (ошибки разбора/анализа - diagnostics.go, может быть
// несколько сразу, см. package doc document.go update), плюс по одной
// записи на каждое вхождение "@устарело"-имени (deprecated.go), плюс
// подсказки-упрощения (hints.go, hintSeverity - "три точки", не ошибка и
// не устаревание) - пустой список снимает раньше показанное подчёркивание/
// зачёркивание, когда документ снова стал разбираться/анализироваться
// успешно, устаревшее имя перестало где-либо встречаться, либо предложенное
// упрощение было применено/код изменился так, что предлагать больше нечего.
func (s *server) publishDiagnostics(uri string, d *doc) {
	diags := []any{}
	for _, diag := range d.diagnostics {
		diags = append(diags, diagnosticJSON(diag))
	}
	for _, dep := range d.deprecatedDiagnostics() {
		diags = append(diags, diagnosticJSON(dep))
	}
	for _, sug := range d.suggestions() {
		diags = append(diags, diagnosticJSON(Diagnostic{
			Line: sug.Line, Col: sug.Col, Len: sug.Len, Message: sug.Message, Severity: hintSeverity,
		}))
	}
	for _, diag := range d.unreachableDiagnostics() {
		diags = append(diags, diagnosticJSON(diag))
	}
	_ = s.rpc.Notify("textDocument/publishDiagnostics", map[string]any{"uri": uri, "diagnostics": diags})
}

// diagnosticJSON - Diagnostic (diagnostics.go/deprecated.go) в форме LSP
// Diagnostic - "tags" опускается вовсе, если пуст (не каждый клиент готов
// к пустому массиву именно в этом поле).
func diagnosticJSON(diag Diagnostic) map[string]any {
	m := map[string]any{
		"range": map[string]any{
			"start": map[string]any{"line": diag.Line - 1, "character": diag.Col - 1},
			"end":   map[string]any{"line": diag.Line - 1, "character": diag.Col - 1 + diag.Len},
		},
		"severity": diag.Severity,
		"message":  diag.Message,
	}
	if len(diag.Tags) > 0 {
		tags := make([]any, len(diag.Tags))
		for i, t := range diag.Tags {
			tags[i] = t
		}
		m["tags"] = tags
	}
	return m
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
		"uri": posURI(pos, p.TextDocument.URI),
		"range": map[string]any{
			"start": map[string]any{"line": pos.Line - 1, "character": pos.Col - 1},
			"end":   map[string]any{"line": pos.Line - 1, "character": pos.Col - 1 + pos.Len},
		},
	}, nil)
}

// posURI - definitionPos.URI, если он задан (позиция в ДРУГОМ файле, см.
// package doc definition.go/multifile.go), иначе requestURI (позиция в том
// же документе, к которому обращён запрос).
func posURI(pos definitionPos, requestURI string) string {
	if pos.URI != "" {
		return pos.URI
	}
	return requestURI
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
			"uri": posURI(pos, p.TextDocument.URI),
			"range": map[string]any{
				"start": map[string]any{"line": pos.Line - 1, "character": pos.Col - 1},
				"end":   map[string]any{"line": pos.Line - 1, "character": pos.Col - 1 + pos.Len},
			},
		}
	}
	_ = s.rpc.Reply(msg.ID, locations, nil)
}

// handleReferences отвечает на "textDocument/references" - все вхождения
// имени под курсором в ТЕКУЩЕМ документе (occurrences.go); областью
// действия LSP-запрос допускает несколько файлов, здесь - сознательно
// только текущий (см. package doc occurrences). context.includeDeclaration
// запроса не читается - объявление включается всегда (самый частый выбор
// клиентов по умолчанию), различать это не стоило ради этого среза.
func (s *server) handleReferences(msg *Message) {
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
	positions, ok := d.occurrences(p.Position.Line+1, p.Position.Character+1)
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

// handleDocumentHighlight отвечает на "textDocument/documentHighlight" - то
// же множество вхождений, что и handleReferences (occurrences.go), но без
// "uri" (запрос по определению документа-локальный) и с "kind" - здесь
// всегда 1 (Text), различать чтение/запись не стоило ради этого среза.
func (s *server) handleDocumentHighlight(msg *Message) {
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
	positions, ok := d.occurrences(p.Position.Line+1, p.Position.Character+1)
	if !ok {
		_ = s.rpc.Reply(msg.ID, []any{}, nil)
		return
	}
	highlights := make([]any, len(positions))
	for i, pos := range positions {
		highlights[i] = map[string]any{
			"range": map[string]any{
				"start": map[string]any{"line": pos.Line - 1, "character": pos.Col - 1},
				"end":   map[string]any{"line": pos.Line - 1, "character": pos.Col - 1 + pos.Len},
			},
			"kind": 1,
		}
	}
	_ = s.rpc.Reply(msg.ID, highlights, nil)
}

// handleCodeAction отвечает на "textDocument/codeAction" - два независимых
// источника quick fix'ов: автоимпорт (codeaction.go, вставка новой строки
// целиком) и подсказки-упрощения (hints.go, замена произвольного диапазона
// существующего текста - "i = i + 1" -> "i++", "статический" перед именем
// метода и т. п.). Автоимпорт НЕ зависит от запрошенного диапазона (LSP
// это разрешает, context.diagnostics - лишь ПОДСКАЗКА клиента, не
// обязательное условие - сама ошибка "не объявлено и не импортировано"
// обычно ровно там, где курсор, а вот МЕСТО ВСТАВКИ строки "использовать" -
// нет, фильтровать по нему было бы неверно). Подсказки-упрощения, В
// ОТЛИЧИЕ от автоимпорта, - ОБЯЗАНЫ фильтроваться по запрошенному
// диапазону: каждая относится ровно к ОДНОМУ конкретному месту кода, без
// фильтра меню "Быстрое исправление" в любой точке документа предлагало бы
// ВСЕ найденные в файле упрощения разом, не только то, что под курсором
// (репортнуто пользователем - "j = j + 1" на одной строке показывало
// quick fix ещё и для "i++"/"+=" с других, ничем не связанных строк файла).
func (s *server) handleCodeAction(msg *Message) {
	var p struct {
		TextDocument struct {
			URI string `json:"uri"`
		} `json:"textDocument"`
		Range struct {
			Start struct {
				Line int `json:"line"`
			} `json:"start"`
			End struct {
				Line int `json:"line"`
			} `json:"end"`
		} `json:"range"`
	}
	if err := json.Unmarshal(msg.Params, &p); err != nil {
		_ = s.rpc.Reply(msg.ID, nil, err)
		return
	}
	d := s.getDoc(p.TextDocument.URI)
	if d == nil {
		_ = s.rpc.Reply(msg.ID, []any{}, nil)
		return
	}
	reqStart, reqEnd := p.Range.Start.Line+1, p.Range.End.Line+1 // 1-индексные, как и весь остальной пакет
	var out []any
	for _, a := range d.codeActions() {
		out = append(out, quickFixJSON(p.TextDocument.URI, a.Title,
			a.NewLine, 0, a.NewLine, 0, a.NewText))
	}
	for _, sug := range d.suggestions() {
		if sug.EndLine < reqStart || sug.StartLine > reqEnd {
			continue // не пересекается с запрошенным диапазоном - см. package doc выше
		}
		out = append(out, quickFixJSON(p.TextDocument.URI, sug.Title,
			sug.StartLine, sug.StartCol-1, sug.EndLine, sug.EndCol-1, sug.NewText))
	}
	if out == nil {
		out = []any{}
	}
	_ = s.rpc.Reply(msg.ID, out, nil)
}

// quickFixJSON - один LSP CodeAction (kind "quickfix") - startLine/endLine
// 1-индексные (как и весь остальной этот пакет), startChar/endChar - УЖЕ
// 0-индексные (вызывающая сторона сама переводит из 1-индексных Col, где
// это применимо - см. package doc handleCodeAction).
func quickFixJSON(uri, title string, startLine, startChar, endLine, endChar int, newText string) map[string]any {
	return map[string]any{
		"title": title,
		"kind":  "quickfix",
		"edit": map[string]any{
			"changes": map[string]any{
				uri: []any{
					map[string]any{
						"range": map[string]any{
							"start": map[string]any{"line": startLine - 1, "character": startChar},
							"end":   map[string]any{"line": endLine - 1, "character": endChar},
						},
						"newText": newText,
					},
				},
			},
		},
	}
}

// handleCodeLens отвечает на "textDocument/codeLens" - "N ссылок" над
// каждым собственным объявлением документа (codelens.go), кликабельное
// через клиентскую (встроенную в VS Code, ничего своего исполнять не
// нужно) команду "editor.action.showReferences": ей передаются готовые
// locations (позиции ссылок), а не URI+range для повторного запроса -
// сервер не поддерживает codeLens/resolve вовсе (declared без
// "resolveProvider", initialize), в этом нет необходимости, раз все данные
// уже под рукой при самом ответе на codeLens.
func (s *server) handleCodeLens(msg *Message) {
	var p struct {
		TextDocument struct {
			URI string `json:"uri"`
		} `json:"textDocument"`
	}
	if err := json.Unmarshal(msg.Params, &p); err != nil {
		_ = s.rpc.Reply(msg.ID, nil, err)
		return
	}
	d := s.getDoc(p.TextDocument.URI)
	if d == nil {
		_ = s.rpc.Reply(msg.ID, []any{}, nil)
		return
	}
	lenses := d.codeLenses()
	out := make([]any, len(lenses))
	for i, cl := range lenses {
		locations := make([]any, len(cl.Refs))
		for j, ref := range cl.Refs {
			uri := p.TextDocument.URI
			if ref.File != "" {
				uri = fileURI(ref.File)
			}
			locations[j] = map[string]any{
				"uri": uri,
				"range": map[string]any{
					"start": map[string]any{"line": ref.Tok.Line - 1, "character": ref.Tok.Col - 1},
					"end":   map[string]any{"line": ref.Tok.Line - 1, "character": ref.Tok.Col - 1 + len([]rune(ref.Tok.Value))},
				},
			}
		}
		lensPos := map[string]any{"line": cl.Line - 1, "character": 0}
		out[i] = map[string]any{
			"range": map[string]any{"start": lensPos, "end": lensPos},
			"command": map[string]any{
				"title":     cl.Title,
				"command":   "editor.action.showReferences",
				"arguments": []any{p.TextDocument.URI, lensPos, locations},
			},
		}
	}
	_ = s.rpc.Reply(msg.ID, out, nil)
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
