package interp

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// generateSelfSignedCertFiles - самоподписанный сертификат/ключ (RSA 2048,
// валиден для "127.0.0.1") в двух PEM-файлах во временном каталоге -
// "слушать_tcp_защищённо" (TLS) принимает ПУТИ к файлам,
// не сырой PEM-текст, тем же принципом, что и у OpenSSL
// SSL_CTX_use_certificate_file/SSL_CTX_use_PrivateKey_file на стороне
// cbackend (tls.go) - тест должен пройти через тот же файловый API, что и
// настоящая программа.
func generateSelfSignedCertFiles(t *testing.T) (certPath, keyPath string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "127.0.0.1"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("x509.CreateCertificate: %v", err)
	}
	dir := t.TempDir()
	certPath = filepath.Join(dir, "cert.pem")
	keyPath = filepath.Join(dir, "key.pem")
	certOut, err := os.Create(certPath)
	if err != nil {
		t.Fatalf("создание cert.pem: %v", err)
	}
	if err := pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: der}); err != nil {
		t.Fatalf("pem.Encode(cert): %v", err)
	}
	certOut.Close()
	keyOut, err := os.Create(keyPath)
	if err != nil {
		t.Fatalf("создание key.pem: %v", err)
	}
	if err := pem.Encode(keyOut, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}); err != nil {
		t.Fatalf("pem.Encode(key): %v", err)
	}
	keyOut.Close()
	return certPath, keyPath
}

// TestRunTLSListenerAcceptsHandshakeAndServesHTTP - "слушать_tcp_защищённо"
// сквозь ВСЮ уже существующую цепочку "принять"/"принять_http"/
// "отправить_http_ответ" (httpserver_test.go, TestRunHTTPServerBuiltins) -
// подтверждает, что TLS для них прозрачен (package doc network.go,
// listenTLS): ни одна из трёх функций не знает, что соединение защищено.
// Клиент - crypto/tls.Dial с InsecureSkipVerify (самоподписанный
// сертификат теста, доверять ему нечем, кроме факта совпадения с тем, что
// сервер реально предъявил - остальное проверяет сам факт успешного
// TLS-рукопожатия и корректного HTTP поверх него).
func TestRunTLSListenerAcceptsHandshakeAndServesHTTP(t *testing.T) {
	certPath, keyPath := generateSelfSignedCertFiles(t)

	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	port := probe.Addr().(*net.TCPAddr).Port
	probe.Close()

	type clientResult struct {
		raw string
		err error
	}
	clientDone := make(chan clientResult, 1)
	go func() {
		var conn *tls.Conn
		var dialErr error
		cfg := &tls.Config{InsecureSkipVerify: true} // #nosec G402 -- самоподписанный сертификат теста
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

		req := "GET /привет HTTP/1.1\r\nHost: localhost\r\nContent-Length: 0\r\n\r\n"
		if _, err := io.WriteString(conn, req); err != nil {
			clientDone <- clientResult{err: fmt.Errorf("отправка запроса: %w", err)}
			return
		}
		raw, err := io.ReadAll(conn)
		if err != nil {
			clientDone <- clientResult{err: fmt.Errorf("чтение ответа: %w", err)}
			return
		}
		clientDone <- clientResult{raw: string(raw)}
	}()

	got := runSrc(t, fmt.Sprintf(`
функция главная
{
    переменная слушатель = слушать_tcp_защищённо(%d, "%s", "%s")
    переменная соединение = принять(слушатель)
    если соединение != Ничто
    {
        переменная запрос = принять_http(соединение)
        вывести `+"`${"+`запрос.путь}\n`+"`"+`

        переменная ответ = новый Ответ(200, "Content-Type: text/plain", "по TLS")
        отправить_http_ответ(соединение, ответ)
        закрыть_сокет(соединение)
    }
    закрыть_сокет(слушатель)
}
`, port, filepath.ToSlash(certPath), filepath.ToSlash(keyPath)))

	res := <-clientDone
	if res.err != nil {
		t.Fatal(res.err)
	}
	if want := "HTTP/1.1 200 OK\r\n"; len(res.raw) < len(want) || res.raw[:len(want)] != want {
		t.Fatalf("ожидался статус 200, получено:\n%s", res.raw)
	}
	if want := "по TLS"; len(res.raw) < len(want) || res.raw[len(res.raw)-len(want):] != want {
		t.Fatalf("ожидалось тело \"по TLS\" в конце ответа, получено:\n%s", res.raw)
	}
	if want := "/привет\n"; got != want {
		t.Fatalf("получено %q, хотели %q", got, want)
	}
}

// TestRunTLSListenerRejectsMissingCertFile - "слушать_tcp_защищённо" с
// несуществующим файлом сертификата - ошибка сразу (rtmsg.TLSCertLoadFailed),
// не отложенная до первого "принять".
func TestRunTLSListenerRejectsMissingCertFile(t *testing.T) {
	err := runSrcExpectRuntimeError(t, `
функция главная
{
    переменная слушатель = слушать_tcp_защищённо(0, "нет-такого-файла.pem", "нет-такого-файла.pem")
}
`)
	if err == nil {
		t.Fatalf("ожидалась ошибка - файл сертификата не существует")
	}
}
