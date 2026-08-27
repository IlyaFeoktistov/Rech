// Package rpcframe - транспортный кадр, общий для DAP (internal/dap) и LSP
// (internal/lsp): заголовок "Content-Length: N\r\n\r\n" перед N байтами
// JSON-тела. Оба протокола используют этот же кадр, расходятся только в
// форме самого JSON внутри (DAP - собственный конверт seq/type/command,
// LSP - JSON-RPC 2.0 с method/params/id) - поэтому здесь только байты,
// без единого предположения о содержимом.
package rpcframe

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// ReadFrame читает один кадр: заголовки до пустой строки (значим только
// "Content-Length" - "Content-Type" и подобные допустимы протоколом, но
// клиенты его не шлют), затем ровно N байт тела.
func ReadFrame(r *bufio.Reader) (json.RawMessage, error) {
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
				return nil, fmt.Errorf("rpcframe: некорректный Content-Length %q: %w", value, err)
			}
			length = n
		}
	}
	if length < 0 {
		return nil, fmt.Errorf("rpcframe: сообщение без заголовка Content-Length")
	}
	body := make([]byte, length)
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, err
	}
	return body, nil
}

// WriteFrame маршалит v в JSON и пишет его в w с заголовком Content-Length.
func WriteFrame(w io.Writer, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Content-Length: %d\r\n\r\n", len(b)); err != nil {
		return err
	}
	_, err = w.Write(b)
	return err
}
