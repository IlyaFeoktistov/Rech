// gRPC - см. package doc internal/semantic/grpc.go. Реализация не заводит
// НИКАКОГО собственного состояния - только length-prefix фрейминг поверх
// уже существующих http2SendData/http2ReadData/http2SendTrailer/
// http2HeaderOf (http2.go) и content-type-хедер, зафиксированный этим
// протоколом ("application/grpc").
package interp

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"io"
	"net/http"
	"strconv"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

const grpcContentType = "application/grpc"

// grpcOpenStream - "grpc_открыть_поток(сессия, путь): HTTP2Поток" - метод
// ФИКСИРОВАН (POST, см. package doc grpc.go), в отличие от http2_открыть_
// поток - единственное отличие от него - заголовок content-type.
func (in *Interp) grpcOpenStream(sessVal Value, path string) Value {
	sess := sessVal.HTTP2
	pr, pw := io.Pipe()
	st := &http2StreamState{reqWriter: pw, headersReady: make(chan struct{})}
	req, err := http.NewRequest(http.MethodPost, "http://"+sess.authority+path, pr)
	if err != nil {
		throwRuntimeError(rtmsg.ErrorNetwork, "%s", rtmsg.HTTP2OpenStreamFailed)
	}
	req.Header.Set("content-type", grpcContentType)
	go func() {
		resp, err := sess.clientConn.RoundTrip(req)
		st.resp, st.respErr = resp, err
		close(st.headersReady)
	}()
	sess.registerStream(st)
	return streamValue(sessVal, st.id)
}

// grpcSendHeaders - "grpc_начать_ответ(поток)" - код ответа ФИКСИРОВАН
// (всегда 200, см. package doc grpc.go) - единственное отличие от
// http2_отправить_заголовки - заголовок content-type.
func (in *Interp) grpcSendHeaders(v Value) {
	_, st := resolveStream(v)
	st.w.Header().Set("content-type", grpcContentType)
	st.w.WriteHeader(http.StatusOK)
}

// grpcSendMessage - "grpc_отправить_сообщение(поток, тело, сжато)" -
// compressed==false (по умолчанию, docs/SPEC.md) - пишет length-prefix (1
// байт флага сжатия "identity" + 4 байта big-endian длины) и само тело как
// есть, ОДНОЙ записью через http2SendData, чтобы не дробить маленькое
// сообщение на два DATA-фрейма. compressed==true - тело сначала gzip-
// сжимается, флаг - 1. Пустое тело ВСЕГДА отправляется как identity, даже
// при compressed==true - сжимать 0 байт бессмысленно, а gzip-обёртка
// "пустых" данных не пуста (заголовок/трейлер формата).
func (in *Interp) grpcSendMessage(v Value, body []byte, compressed bool) {
	if !compressed || len(body) == 0 {
		framed := make([]byte, 5+len(body))
		binary.BigEndian.PutUint32(framed[1:5], uint32(len(body)))
		copy(framed[5:], body)
		in.http2SendData(v, bytesValue(framed))
		return
	}
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	if _, err := gw.Write(body); err != nil {
		panicRuntime("%s", rtmsg.GRPCCompressFailed)
	}
	if err := gw.Close(); err != nil {
		panicRuntime("%s", rtmsg.GRPCCompressFailed)
	}
	compressedBody := buf.Bytes()
	framed := make([]byte, 5+len(compressedBody))
	framed[0] = 1
	binary.BigEndian.PutUint32(framed[1:5], uint32(len(compressedBody)))
	copy(framed[5:], compressedBody)
	in.http2SendData(v, bytesValue(framed))
}

// grpcReadMessage - "grpc_прочитать_сообщение(поток): ?Список<Байт>" - см.
// rtmsg.GRPCFramingError/GRPCCompressedUnsupported - Ничто ТОЛЬКО на
// чистом конце потока (первый же читаемый байт - EOF), обрыв посреди
// префикса/тела - паника, не Ничто. Флаг сжатия (header[0]) 1 - тело
// распаковывается как gzip (единственный поддержанный grpc-encoding) -
// неудачная распаковка (не gzip/испорчено) - паника, не тихий пропуск.
func (in *Interp) grpcReadMessage(v Value) Value {
	nullableType := ir.NullableType(ir.EntityType(byteListType))
	header := in.grpcReadExactly(v, 5, true)
	if header == nil {
		return Value{Type: nullableType}
	}
	compressed := header[0] != 0
	msgLen := binary.BigEndian.Uint32(header[1:5])
	body := in.grpcReadExactly(v, int64(msgLen), false)
	if !compressed {
		inner := bytesValue(body)
		return Value{Type: nullableType, Nullable: &inner}
	}
	plain, err := gzipDecompress(body)
	if err != nil {
		throwRuntimeError(rtmsg.ErrorNetwork, "%s", rtmsg.GRPCCompressedUnsupported)
	}
	inner := bytesValue(plain)
	return Value{Type: nullableType, Nullable: &inner}
}

// gzipDecompress - распаковывает gzip-тело целиком (gRPC-сообщения
// ограничены разумным размером самим приложением, потоковое чтение здесь
// не нужно - тем же принципом, что и остальной grpcReadMessage, читающий
// тело целиком по длине из префикса).
func gzipDecompress(data []byte) ([]byte, error) {
	gr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer gr.Close()
	return io.ReadAll(gr)
}

// grpcReadExactly - читает ровно n байт через http2ReadData в цикле (та
// сама может вернуть меньше запрошенного за одно обращение - см. её
// package doc). allowCleanEOF - true только для префикса длины (там
// пустой результат ПЕРВОГО чтения - легитимный конец потока, не ошибка) -
// возвращает nil в этом единственном случае, отличимый от "0 байт тела"
// (grpcReadExactly(v, 0, false) возвращает пустой, но НЕ nil, срез).
func (in *Interp) grpcReadExactly(v Value, n int64, allowCleanEOF bool) []byte {
	if n == 0 {
		return []byte{}
	}
	buf := make([]byte, 0, n)
	for int64(len(buf)) < n {
		chunk := bytesFromValue(in.http2ReadData(v, n-int64(len(buf))))
		if len(chunk) == 0 {
			if allowCleanEOF && len(buf) == 0 {
				return nil
			}
			throwRuntimeError(rtmsg.ErrorNetwork, "%s", rtmsg.GRPCFramingError)
		}
		buf = append(buf, chunk...)
	}
	return buf
}

// grpcFinish - "grpc_завершить(поток, код, сообщение)" - серверная
// сторона: трейлеры "grpc-status"/"grpc-message" (пустое сообщение не
// отправляется вовсе - легитимный успех без текста) + завершает поток.
func (in *Interp) grpcFinish(v Value, code int64, message string) {
	in.http2SendTrailer(v, "grpc-status", strconv.FormatInt(code, 10))
	if message != "" {
		in.http2SendTrailer(v, "grpc-message", message)
	}
	in.http2FinishStream(v)
}

// grpcStatus - "grpc_статус(поток): Целое" - клиентская сторона, см.
// rtmsg.GRPCStatusMissing - вызывающая программа обязана дочитать поток
// (grpc_прочитать_сообщение до Ничто) РАНЬШЕ, иначе трейлер ещё не виден
// (resp.Trailer, http2HeaderOf, http2.go).
func (in *Interp) grpcStatus(v Value) Value {
	raw := in.http2HeaderOf(v, "grpc-status")
	if raw.Nullable == nil {
		throwRuntimeError(rtmsg.ErrorNetwork, "%s", rtmsg.GRPCStatusMissing)
	}
	code, err := strconv.ParseInt(raw.Nullable.S, 10, 64)
	if err != nil {
		throwRuntimeError(rtmsg.ErrorNetwork, "%s", rtmsg.GRPCStatusInvalid)
	}
	return intValue(code)
}

// grpcErrorMessage - "grpc_сообщение_ошибки(поток): Строка" - "" если
// трейлер отсутствует (легитимный успех без текста, см. grpcFinish).
func (in *Interp) grpcErrorMessage(v Value) Value {
	raw := in.http2HeaderOf(v, "grpc-message")
	if raw.Nullable == nil {
		return stringValue("")
	}
	return stringValue(raw.Nullable.S)
}
