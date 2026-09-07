// HTTP/2-транспорт (internal/semantic/http2.go) - реализация через
// стандартный net/http + golang.org/x/net/http2, но с СИНХРОННЫМ Rech-API
// поверх (http2_принять_поток/http2_прочитать_данные/... блокируют вызывающую
// сторону, как и остальные сетевые builtin'ы этого языка) - мост между
// асинхронной моделью http2.Server (Handler вызывается в СВОЕЙ горутине на
// каждый входящий поток) и блокирующими вызовами Rech-программы через
// каналы: http2Session.incoming доставляет новый *http2StreamState в
// http2_принять_поток, а сам Handler блокируется на http2StreamState.finished
// до вызова http2_завершить_поток - до этого момента "владение" w/r.Body
// эффективно передано горутине, вызвавшей http2_принять_поток, у самого
// Handler'а больше нет причин их трогать.
//
// Клиентская сторона симметрична: http2_открыть_поток не ждёт ответных
// заголовков синхронно (это привело бы к взаимной блокировке с сервером,
// который может не отвечать, пока не получит часть/всё тело запроса,
// см. package doc openStream) - RoundTrip запускается в фоновой горутине,
// http2StreamState.headersReady закрывается, когда ответ (или ошибка)
// готовы; http2_статус/http2_заголовок/http2_прочитать_данные на клиентском
// потоке сами дожидаются этого канала при первом обращении.
//
// См. internal/backend/cbackend/http2.go для эквивалентной реализации на C
// (биндинг к nghttp2 - в C нет аналога net/http, дающего HTTP/2 "бесплатно").
package interp

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/net/http2"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

// http2StreamType - см. internal/semantic/http2.go, http2StreamType - тот
// же принцип дублирования имени константой в нескольких пакетах, что и
// byteListType (bytesio.go).
const http2StreamType = "HTTP2Поток"

// http2StreamState - один RPC/запрос внутри http2Session - см. package doc.
type http2StreamState struct {
	id int64

	// Серверная сторона (непусто, когда стрим пришёл через
	// http2_принять_поток).
	w          http.ResponseWriter
	flusher    http.Flusher
	r          *http.Request
	finished   chan struct{}
	finishOnce sync.Once

	// Клиентская сторона (непусто, когда стрим создан через
	// http2_открыть_поток).
	reqWriter    *io.PipeWriter
	headersReady chan struct{}
	resp         *http.Response
	respErr      error
}

// http2Session - см. package doc и ir.KindHTTP2Session.
type http2Session struct {
	conn net.Conn

	// Серверная сторона.
	incoming chan *http2StreamState

	// Клиентская сторона.
	clientConn *http2.ClientConn
	authority  string

	mu      sync.Mutex
	streams map[int64]*http2StreamState
	nextID  int64
}

func (sess *http2Session) registerStream(st *http2StreamState) int64 {
	sess.mu.Lock()
	defer sess.mu.Unlock()
	id := sess.nextID
	sess.nextID++
	st.id = id
	sess.streams[id] = st
	return id
}

func (sess *http2Session) lookupStream(id int64) *http2StreamState {
	sess.mu.Lock()
	defer sess.mu.Unlock()
	return sess.streams[id]
}

func sessionValue(sess *http2Session) Value {
	return Value{Type: ir.TypeHTTP2Session, HTTP2: sess}
}

func streamValue(sess Value, id int64) Value {
	fields := map[string]Value{
		"сессия": sess,
		"ид":     intValue(id),
	}
	return Value{Type: ir.EntityType(http2StreamType), Obj: &entityObj{typeName: http2StreamType, fields: fields}}
}

// resolveStream - достаёт *http2Session/*http2StreamState из Rech-значения
// "HTTP2Поток" (поля "сессия"/"ид", см. streamValue).
func resolveStream(v Value) (*http2Session, *http2StreamState) {
	sess := v.Obj.fields["сессия"].HTTP2
	id := v.Obj.fields["ид"].I
	return sess, sess.lookupStream(id)
}

// http2AcceptSession - "http2_принять_сессию(слушающий: Сокет): HTTP2Сессия" -
// принимает ОДНО новое TCP-соединение с уже слушающего Сокет и сразу
// заводит на нём серверную HTTP/2-сессию (http2.Server.ServeConn сама
// проверяет клиентский preface - ошибка рукопожатия обнаружится при первом
// обращении к сессии, не здесь синхронно, тем же принципом отложенной
// проверки, что и h2c-часть). ЖИВОЙ НАЙДЕННЫЙ БАГ (grpcs, Шаг 95) - TLS-
// рукопожатие *tls.Conn у Go ленивое (на первом Read/Write), а http2.Server.
// ServeConn читает ConnectionState() СРАЗУ, ДО первого Read/Write - у ещё
// не рукопожатого соединения Version==0, что http2.Server интерпретирует
// как "TLS version too low" и рвёт соединение GOAWAY'ем ДО того, как
// программа вообще увидит хоть один поток (acceptWithTimeout, network_
// timeout.go, уже форсирует tc.Handshake() синхронно для ЭТОЙ ЖЕ причины у
// обычного "принять" - здесь пропущено не было замечено раньше просто
// потому, что до grpcs НИ ОДИН вызывающий http2_принять_сессию listener не
// бывал TLS-обёрнутым).
func (in *Interp) http2AcceptSession(listenerVal Value) Value {
	if listenerVal.Listener == nil {
		panicRuntime("%s", rtmsg.SocketWrongKind)
	}
	conn, err := listenerVal.Listener.Accept()
	if err != nil {
		panicRuntime("%s", rtmsg.HTTP2AcceptFailed)
	}
	if tc, ok := conn.(*tls.Conn); ok {
		if err := tc.Handshake(); err != nil {
			conn.Close()
			panicRuntime("%s", rtmsg.HTTP2HandshakeFailed)
		}
	}
	sess := &http2Session{conn: conn, incoming: make(chan *http2StreamState, 256), streams: map[int64]*http2StreamState{}}
	server := &http2.Server{}
	go server.ServeConn(conn, &http2.ServeConnOpts{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			st := &http2StreamState{w: w, r: r, finished: make(chan struct{})}
			if fl, ok := w.(http.Flusher); ok {
				st.flusher = fl
			}
			sess.registerStream(st)
			sess.incoming <- st
			<-st.finished
		}),
	})
	return sessionValue(sess)
}

// http2NewSession - "новая_http2_сессия(хост, порт): HTTP2Сессия" -
// подключается по обычному TCP (тем же net.Dial, что и "сетевой_узел" с
// непустым хостом) и сразу устанавливает клиентскую HTTP/2-сессию поверх
// него - h2c (cleartext), не TLS (AllowHTTP - штатный приём golang.org/
// x/net/http2 для клиента без TLS: DialTLSContext подменён обычным
// net.Dial, ALPN/сертификаты не участвуют вовсе).
func (in *Interp) http2NewSession(host string, port int64) Value {
	addr := net.JoinHostPort(host, strconv.FormatInt(port, 10))
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		panicRuntime("%s", rtmsg.HTTP2DialFailed)
	}
	transport := &http2.Transport{
		AllowHTTP: true,
		DialTLSContext: func(ctx context.Context, network, addr string, cfg *tls.Config) (net.Conn, error) {
			return net.Dial(network, addr)
		},
	}
	clientConn, err := transport.NewClientConn(conn)
	if err != nil {
		conn.Close()
		panicRuntime("%s", rtmsg.HTTP2ClientConnFailed)
	}
	return sessionValue(&http2Session{conn: conn, clientConn: clientConn, authority: addr, streams: map[int64]*http2StreamState{}})
}

// http2CloseSession - "http2_закрыть_сессию".
func (in *Interp) http2CloseSession(sessVal Value) {
	sessVal.HTTP2.conn.Close()
}

// http2AcceptStream - "http2_принять_поток(сессия): HTTP2Поток" - блокирует
// до прихода следующего входящего запроса на этой сессии (см. package doc).
func (in *Interp) http2AcceptStream(sessVal Value) Value {
	sess := sessVal.HTTP2
	st := <-sess.incoming
	return streamValue(sessVal, st.id)
}

// http2OpenStream - "http2_открыть_поток(сессия, метод, путь): HTTP2Поток" -
// см. package doc: возвращается СРАЗУ (не дожидаясь ответа) - RoundTrip
// уходит в фоновую горутину, ответ подхватывается лениво при первом
// обращении (http2_статус/http2_заголовок/http2_прочитать_данные).
func (in *Interp) http2OpenStream(sessVal Value, method, path string) Value {
	sess := sessVal.HTTP2
	pr, pw := io.Pipe()
	st := &http2StreamState{reqWriter: pw, headersReady: make(chan struct{})}
	req, err := http.NewRequest(method, "http://"+sess.authority+path, pr)
	if err != nil {
		panicRuntime("%s", rtmsg.HTTP2OpenStreamFailed)
	}
	go func() {
		resp, err := sess.clientConn.RoundTrip(req)
		st.resp, st.respErr = resp, err
		close(st.headersReady)
	}()
	sess.registerStream(st)
	return streamValue(sessVal, st.id)
}

// awaitClientResponse - ждёт headersReady (только для клиентских потоков -
// серверные st.headersReady == nil, вызывать на них ошибочно, не
// защищено отдельно - симметрично cbackend, где это тоже статически
// известно по тому, какая функция вызвана: http2_принять_поток против
// http2_открыть_поток).
func awaitClientResponse(st *http2StreamState) *http.Response {
	<-st.headersReady
	if st.respErr != nil {
		panicRuntime("%s", rtmsg.HTTP2OpenStreamFailed)
	}
	return st.resp
}

func (in *Interp) http2Path(v Value) Value {
	_, st := resolveStream(v)
	if st.r != nil {
		return stringValue(st.r.URL.Path)
	}
	return stringValue("")
}

func (in *Interp) http2Method(v Value) Value {
	_, st := resolveStream(v)
	if st.r != nil {
		return stringValue(st.r.Method)
	}
	return stringValue("")
}

func (in *Interp) http2StatusOf(v Value) Value {
	_, st := resolveStream(v)
	resp := awaitClientResponse(st)
	return intValue(int64(resp.StatusCode))
}

func (in *Interp) http2HeaderOf(v Value, name string) Value {
	_, st := resolveStream(v)
	nullableType := ir.NullableType(ir.TypeString)
	var raw string
	var present bool
	if st.r != nil {
		raw = st.r.Header.Get(name)
		present = raw != ""
	} else {
		resp := awaitClientResponse(st)
		raw = resp.Header.Get(name)
		present = raw != ""
		// resp.Trailer - ЖИВОЙ НАЙДЕННЫЙ БАГ: трейлеры (http2_отправить_
		// трейлер, серверная сторона) прилетают клиенту НЕ в resp.Header, а
		// в ОТДЕЛЬНОМ resp.Trailer (стандартное поведение net/http -
		// заполняется только после того, как resp.Body прочитано ДО EOF,
		// вызывающая программа обязана сама прочитать тело раньше, чем
		// звать http2_заголовок за трейлером, тем же принципом, что и у
		// recv_eof в cbackend/http2.go) - без этой ветки http2_заголовок
		// НИКОГДА не находил бы трейлер на клиентском потоке.
		if !present {
			raw = resp.Trailer.Get(name)
			present = raw != ""
		}
	}
	if !present {
		return Value{Type: nullableType}
	}
	sv := stringValue(raw)
	return Value{Type: nullableType, Nullable: &sv}
}

// http2ReadData - "http2_прочитать_данные(поток, максимум): Список<Байт>" -
// см. package doc registerHTTP2Funcs (http2.go, semantic) - как
// "прочитать(файл, максимум)", НЕ всё тело целиком: пустой результат -
// конец потока (io.EOF), не "сейчас нет данных" (Read блокирует до хотя бы
// одного байта или EOF, обычное поведение io.Reader).
func (in *Interp) http2ReadData(v Value, max int64) Value {
	if max <= 0 {
		panicRuntime("%s", rtmsg.HTTP2RecvInvalidMax)
	}
	_, st := resolveStream(v)
	var reader io.Reader
	if st.r != nil {
		reader = st.r.Body
	} else {
		reader = awaitClientResponse(st).Body
	}
	buf := make([]byte, max)
	n, err := reader.Read(buf)
	if err != nil && err != io.EOF {
		panicRuntime("%s", rtmsg.HTTP2RecvFailed)
	}
	return bytesValue(buf[:n])
}

// http2ReadAll - "http2_прочитать_всё(поток): Список<Байт>" - ждёт конца
// потока (END_STREAM) и отдаёт тело целиком - удобно, когда сторона
// гарантированно шлёт РОВНО ОДНО сообщение за весь поток (как в
// echo-примере), но НЕ годится для gRPC server-streaming/bidi (несколько
// сообщений в одном потоке) - там нужен именно цикл на http2ReadData с
// известной длиной каждого сообщения, см. комментарий у неё выше.
func (in *Interp) http2ReadAll(v Value) Value {
	_, st := resolveStream(v)
	var reader io.Reader
	if st.r != nil {
		reader = st.r.Body
	} else {
		reader = awaitClientResponse(st).Body
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		panicRuntime("%s", rtmsg.HTTP2ReadAllFailed)
	}
	return bytesValue(data)
}

// http2SendHeaders - "http2_отправить_заголовки(поток, код)" - только
// серверная сторона (у клиента заголовки запроса уже отправлены самим
// http2_открыть_поток).
func (in *Interp) http2SendHeaders(v Value, code int64) {
	_, st := resolveStream(v)
	st.w.WriteHeader(int(code))
}

// http2SendData - "http2_отправить_данные(поток, данные)" - работает
// симметрично на обеих сторонах (сервер пишет в ResponseWriter, клиент - в
// pipe-конец тела запроса) - Flush после каждой записи на сервере, чтобы
// DATA-фрейм ушёл немедленно (тот же принцип синхронной отправки, что и
// "отправить" на обычном Сокете).
func (in *Interp) http2SendData(v Value, dataVal Value) {
	_, st := resolveStream(v)
	data := bytesFromValue(dataVal)
	if st.w != nil {
		if _, err := st.w.Write(data); err != nil {
			panicRuntime("%s", rtmsg.HTTP2SendFailed)
		}
		if st.flusher != nil {
			st.flusher.Flush()
		}
		return
	}
	if _, err := st.reqWriter.Write(data); err != nil {
		panicRuntime("%s", rtmsg.HTTP2SendFailed)
	}
}

// http2SendTrailer - "http2_отправить_трейлер(поток, имя, значение)" -
// только серверная сторона (HTTP-трейлеры - часть ОТВЕТА; gRPC, следующий
// шаг, шлёт grpc-status/grpc-message именно так) - http.TrailerPrefix
// работает без предварительного объявления имени трейлера в заголовке
// "Trailer:" (в отличие от классического механизма net/http).
func (in *Interp) http2SendTrailer(v Value, name, value string) {
	if !isHTTPToken(name) {
		panicRuntime("%s", rtmsg.HTTP2InvalidTrailerName)
	}
	_, st := resolveStream(v)
	st.w.Header().Set(http.TrailerPrefix+name, value)
}

// isHTTPToken - см. rtmsg.HTTP2InvalidTrailerName - RFC 7230 "token"
// (field-name): непустая строка из ASCII-букв/цифр и "!#$%&'*+-.^_`|~".
func isHTTPToken(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		case strings.ContainsRune("!#$%&'*+-.^_`|~", rune(c)):
		default:
			return false
		}
	}
	return true
}

// http2FinishStream - "http2_завершить_поток(поток)" - завершает СВОЮ
// сторону потока: сервер - отпускает Handler (см. http2AcceptSession,
// http2.Server сама шлёт END_STREAM и уже установленные трейлеры, когда
// Handler возвращается), клиент - закрывает pipe-конец тела запроса
// (сигнал EOF/END_STREAM для запроса - ответ мог быть, а мог и не быть уже
// получен к этому моменту, независимая половина потока).
func (in *Interp) http2FinishStream(v Value) {
	_, st := resolveStream(v)
	if st.w != nil {
		st.finishOnce.Do(func() { close(st.finished) })
		return
	}
	st.reqWriter.Close()
}
