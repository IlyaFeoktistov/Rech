package interp

import (
	"bytes"
	"crypto/tls"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/net/http2"
)

// grpcEncodeFrame/grpcDecodeFrame - тот же length-prefix wire-формат, что
// grpcSendMessage/grpcReadMessage (grpc.go) - здесь переписан вручную (не
// импортирует их напрямую), потому что клиент теста нарочно строит запрос
// низкоуровневым http2.Transport, а не через grpc_*-builtin'ы (сама
// "новая_grpcs_сессия" сознательно не обходит проверку сертификата, см.
// package doc grpcs.go - тестовый сертификат самоподписанный).
func grpcEncodeFrame(msg []byte) []byte {
	out := make([]byte, 5+len(msg))
	binary.BigEndian.PutUint32(out[1:5], uint32(len(msg)))
	copy(out[5:], msg)
	return out
}

func grpcDecodeFrame(t *testing.T, r io.Reader) []byte {
	t.Helper()
	header := make([]byte, 5)
	if _, err := io.ReadFull(r, header); err != nil {
		t.Fatalf("чтение префикса gRPC-кадра: %v", err)
	}
	n := binary.BigEndian.Uint32(header[1:5])
	body := make([]byte, n)
	if _, err := io.ReadFull(r, body); err != nil {
		t.Fatalf("чтение тела gRPC-кадра: %v", err)
	}
	return body
}

// TestGRPCSServerAcceptsRealTLSClientOverALPN - "слушать_grpcs" (реальный
// builtin, серверная сторона, полный grpc_*-путь) против клиента,
// собранного вручную ТЕМ ЖЕ способом, что grpcsNewSession (grpcs.go), но с
// InsecureSkipVerify (самоподписанный тестовый сертификат не имеет
// доверенного CA - production-путь "новая_grpcs_сессия" сознательно НЕ
// обходит проверку, см. её package doc) - подтверждает, что ALPN "h2" +
// TLS-рукопожатие + HTTP/2-сессия поверх него реально работают против
// настоящего TLS-клиента (golang.org/x/net/http2.Transport), не только
// между двумя процессами этого же компилятора.
func TestGRPCSServerAcceptsRealTLSClientOverALPN(t *testing.T) {
	certPath, keyPath := generateSelfSignedCertFiles(t)

	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	port := probe.Addr().(*net.TCPAddr).Port
	probe.Close()

	type clientResult struct {
		msg string
		err error
	}
	clientDone := make(chan clientResult, 1)
	go func() {
		var conn *tls.Conn
		var dialErr error
		cfg := &tls.Config{NextProtos: []string{"h2"}, InsecureSkipVerify: true} // #nosec G402 -- самоподписанный сертификат теста
		for range 200 {
			conn, dialErr = tls.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port), cfg)
			if dialErr == nil {
				break
			}
			time.Sleep(5 * time.Millisecond)
		}
		if dialErr != nil {
			clientDone <- clientResult{err: fmt.Errorf("tls.Dial: %w", dialErr)}
			return
		}
		defer conn.Close()
		if conn.ConnectionState().NegotiatedProtocol != "h2" {
			clientDone <- clientResult{err: fmt.Errorf("ALPN не согласовал h2, получено %q", conn.ConnectionState().NegotiatedProtocol)}
			return
		}

		transport := &http2.Transport{}
		clientConn, err := transport.NewClientConn(conn)
		if err != nil {
			clientDone <- clientResult{err: fmt.Errorf("NewClientConn: %w", err)}
			return
		}

		body := grpcEncodeFrame([]byte("запрос по TLS"))
		req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("https://127.0.0.1:%d/эхо.Сервис/Метод", port), bytes.NewReader(body))
		if err != nil {
			clientDone <- clientResult{err: fmt.Errorf("NewRequest: %w", err)}
			return
		}
		resp, err := clientConn.RoundTrip(req)
		if err != nil {
			clientDone <- clientResult{err: fmt.Errorf("RoundTrip: %w", err)}
			return
		}
		defer resp.Body.Close()
		msg := grpcDecodeFrame(t, resp.Body)
		clientDone <- clientResult{msg: string(msg)}
	}()

	out := runSrc(t, fmt.Sprintf(`
функция главная
{
    переменная слушающий = слушать_grpcs(%d, "%s", "%s")
    переменная сессия = http2_принять_сессию(слушающий)
    переменная поток = http2_принять_поток(сессия)
    переменная тело = grpc_прочитать_сообщение(поток)
    если тело != Ничто {
        вывести `+"`${"+`"сервер получил: " + строка_из_байт(тело)}\n`+"`"+`
    }
    grpc_начать_ответ(поток)
    grpc_отправить_сообщение(поток, байты_из_строки("ответ по TLS"))
    grpc_завершить(поток, 0, "")
}
`, port, filepath.ToSlash(certPath), filepath.ToSlash(keyPath)))

	res := <-clientDone
	if res.err != nil {
		t.Fatal(res.err)
	}
	if res.msg != "ответ по TLS" {
		t.Fatalf("неожиданное сообщение от сервера: %q", res.msg)
	}
	if want := "сервер получил: запрос по TLS\n"; out != want {
		t.Fatalf("вывод сервера: получено %q, ожидалось %q", out, want)
	}
}
