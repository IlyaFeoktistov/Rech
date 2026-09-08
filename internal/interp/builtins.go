package interp

import (
	"crypto/tls"
	"fmt"
	"io"
	"math"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

// evalBuiltinCall исполняет встроенную функцию шага 8 (internal/builtins -
// ввод консоли, базовая математика) - см. package doc
// internal/backend/cbackend (readHelpers/builtinCallCode), это тот же
// набор функций, реализованный здесь через стандартную библиотеку Go
// (math.*, strconv.*) вместо <math.h>/<stdio.h>. ok == false - name не
// встроена, вызывающий код (evalExpr, *ir.CallExpr) ищет её среди
// пользовательских функций программы, как и раньше.
func (in *Interp) evalBuiltinCall(name string, args []Value) (Value, bool) {
	switch name {
	case "корень":
		return floatValue(math.Sqrt(args[0].F)), true
	case "степень":
		return floatValue(math.Pow(args[0].F, args[1].F)), true
	case "округлить":
		return intValue(int64(math.Round(args[0].F))), true
	case "потолок":
		return intValue(int64(math.Ceil(args[0].F))), true
	case "пол":
		return intValue(int64(math.Floor(args[0].F))), true
	case "модуль":
		return floatValue(math.Abs(args[0].F)), true
	case "экспонента":
		return floatValue(math.Exp(args[0].F)), true
	case "синус":
		return floatValue(math.Sin(args[0].F)), true
	case "косинус":
		return floatValue(math.Cos(args[0].F)), true
	case "тангенс":
		return floatValue(math.Tan(args[0].F)), true
	case "арксинус":
		return floatValue(math.Asin(args[0].F)), true
	case "арккосинус":
		return floatValue(math.Acos(args[0].F)), true
	case "арктангенс":
		return floatValue(math.Atan(args[0].F)), true
	case "арктангенс2":
		return floatValue(math.Atan2(args[0].F, args[1].F)), true
	case "логарифм":
		return floatValue(math.Log(args[0].F)), true
	case "логарифм2":
		return floatValue(math.Log2(args[0].F)), true
	case "логарифм10":
		return floatValue(math.Log10(args[0].F)), true
	case "минимум":
		return floatValue(math.Min(args[0].F, args[1].F)), true
	case "максимум":
		return floatValue(math.Max(args[0].F, args[1].F)), true
	case "пи":
		return floatValue(math.Pi), true
	case "прочитать_строку":
		return stringValue(in.readLine()), true
	case "прочитать_целое":
		line := in.readLine()
		v, err := strconv.ParseInt(line, 10, 64)
		if err != nil {
			panicRuntime("%s", rtmsg.ReadIntFormat(line))
		}
		return intValue(v), true
	case "прочитать_число":
		line := in.readLine()
		v, err := strconv.ParseFloat(line, 64)
		if err != nil {
			panicRuntime("%s", rtmsg.ReadFloatFormat(line))
		}
		return floatValue(v), true
	case "открыть":
		return in.openFile(name, args[0].S, os.O_RDONLY, 0), true
	case "открыть_для_записи":
		return in.openFile(name, args[0].S, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644), true
	case "открыть_для_дозаписи":
		return in.openFile(name, args[0].S, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644), true
	case "прочитать":
		max := args[1].I
		if max <= 0 {
			panicRuntime("%s", rtmsg.ReadInvalidMax)
		}
		buf := make([]byte, max)
		n, err := args[0].File.Read(buf)
		if err != nil && err != io.EOF {
			panicRuntime("%s", rtmsg.ReadIOFailed)
		}
		return stringValue(string(buf[:n])), true
	case "записать":
		n, err := args[0].File.WriteString(args[1].S)
		if err != nil || n != len(args[1].S) {
			panicRuntime("%s", rtmsg.WriteIOFailed)
		}
		return Value{}, true
	case "закрыть":
		args[0].File.Close()
		return Value{}, true
	case "прочитать_файл":
		f := in.openFile(name, args[0].S, os.O_RDONLY, 0).File
		data, err := io.ReadAll(f)
		f.Close()
		if err != nil {
			panicRuntime("%s", rtmsg.ReadIOFailed)
		}
		return stringValue(string(data)), true
	case "записать_в_файл":
		in.writeWholeFile(name, args[0].S, args[1].S, os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
		return Value{}, true
	case "дописать_в_файл":
		in.writeWholeFile(name, args[0].S, args[1].S, os.O_WRONLY|os.O_CREATE|os.O_APPEND)
		return Value{}, true
	case "прочитать_байты":
		max := args[1].I
		if max <= 0 {
			panicRuntime("%s", rtmsg.ReadBytesInvalidMax)
		}
		buf := make([]byte, max)
		n, err := args[0].File.Read(buf)
		if err != nil && err != io.EOF {
			panicRuntime("%s", rtmsg.ReadBytesIOFailed)
		}
		return bytesValue(buf[:n]), true
	case "записать_байты":
		data := bytesFromValue(args[1])
		n, err := args[0].File.Write(data)
		if err != nil || n != len(data) {
			panicRuntime("%s", rtmsg.WriteBytesIOFailed)
		}
		return Value{}, true
	case "прочитать_файл_байты":
		f := in.openFile(name, args[0].S, os.O_RDONLY, 0).File
		data, err := io.ReadAll(f)
		f.Close()
		if err != nil {
			panicRuntime("%s", rtmsg.ReadBytesIOFailed)
		}
		return bytesValue(data), true
	case "записать_в_файл_байты":
		f := in.openFile(name, args[0].S, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644).File
		data := bytesFromValue(args[1])
		n, err := f.Write(data)
		f.Close()
		if err != nil || n != len(data) {
			panicRuntime("%s", rtmsg.WriteBytesIOFailed)
		}
		return Value{}, true
	case "стандартный_ввод":
		// os.Stdin напрямую, не через in.stdin (bufio.Reader, которым уже
		// читают прочитать_строку/прочитать_целое/прочитать_число) -
		// известное ограничение: смешивать эти встроенные функции с
		// "прочитать"/"для X в стандартный_ввод()" в одной программе не
		// поддерживается (у обоих независимая буферизация, возможна потеря
		// уже прочитанных, но не возвращённых вызывающему коду байт) - в
		// отличие от cbackend, где и то, и другое читает через один и тот
		// же C stdio `stdin`.
		return fileValue(os.Stdin), true
	case "стандартный_вывод":
		return fileValue(os.Stdout), true
	case "стандартный_вывод_ошибок":
		return fileValue(os.Stderr), true
	case "количество_аргументов":
		// "rech test <файл>" не передаёт интерпретируемой программе своих
		// аргументов (аргументы после <файл> - аргументы самого rech, не
		// программы) - всегда 0, "аргумент" ниже всегда паникует диапазоном.
		return intValue(0), true
	case "аргумент":
		panicRuntime("%s", rtmsg.ArgIndexOutOfRangeFormat(args[0].I))
		return Value{}, true
	case "содержит":
		return boolValue(strings.Contains(args[0].S, args[1].S)), true
	case "сетевой_узел":
		return in.networkNode(args[0].S, args[1].S, args[2].I), true
	case "принять":
		return in.acceptWithTimeout(args[0]), true
	case "установить_таймаут":
		setSocketTimeout(args[0], args[1].I)
		return Value{}, true
	case "запрошена_остановка":
		return boolValue(shutdownRequested()), true
	case "получен_сигнал":
		return boolValue(signalReceived(args[0].I)), true
	case "отправить_сигнал":
		signalSend(args[0].I, args[1].I)
		return Value{}, true
	case "установить_сырой_режим":
		setRawMode(args[0].B)
		return Value{}, true
	case "ширина_терминала":
		return intValue(terminalWidth()), true
	case "высота_терминала":
		return intValue(terminalHeight()), true
	case "можно_прочитать_байт":
		return boolValue(canRead(args[0].File, args[1].I)), true
	case "дождаться_всех_задач":
		return boolValue(waitAllTasks(args[0].I)), true
	case "слушать_tcp_защищённо":
		return listenTLS(args[0].I, args[1].S, args[2].S), true
	case "слушать_grpcs":
		return listenGRPCS(args[0].I, args[1].S, args[2].S), true
	case "новая_grpcs_сессия":
		return in.grpcsNewSession(args[0].S, args[1].I), true
	case "целое_в_hex":
		return intToBaseString(args[0].I, "0x", 16), true
	case "целое_из_hex":
		return intFromBaseString("целое_из_hex", args[0].S, "0x", 16), true
	case "целое_в_oct":
		return intToBaseString(args[0].I, "0o", 8), true
	case "целое_из_oct":
		return intFromBaseString("целое_из_oct", args[0].S, "0o", 8), true
	case "целое_в_bin":
		return intToBaseString(args[0].I, "0b", 2), true
	case "целое_из_bin":
		return intFromBaseString("целое_из_bin", args[0].S, "0b", 2), true
	case "спать_мс":
		time.Sleep(time.Duration(args[0].I) * time.Millisecond)
		return Value{}, true
	case "выйти":
		os.Exit(int(args[0].I))
		return Value{}, true
	case "получить_от":
		return in.recvFrom(args[0], args[1].I), true
	case "отправить_в":
		in.sendTo(args[0], args[1].S, args[2].S, args[3].I)
		return Value{}, true
	case "отправить":
		if args[0].Conn == nil {
			panicRuntime("%s", rtmsg.SocketWrongKind)
		}
		if _, err := io.WriteString(args[0].Conn, args[1].S); err != nil {
			panicRuntime("%s", rtmsg.SocketSendFailed)
		}
		return Value{}, true
	case "получить":
		if args[0].Conn == nil {
			panicRuntime("%s", rtmsg.SocketWrongKind)
		}
		max := args[1].I
		if max <= 0 {
			panicRuntime("%s", rtmsg.SocketRecvInvalidMax)
		}
		buf := make([]byte, max)
		applyDeadline(args[0].Conn, args[0].Conn)
		n, err := args[0].Conn.Read(buf)
		if err != nil && err != io.EOF {
			panicRuntime("%s", rtmsg.SocketRecvFailed)
		}
		return stringValue(string(buf[:n])), true
	case "запрос":
		return in.httpRequest(args[0]), true
	case "принять_http":
		if args[0].Conn == nil {
			panicRuntime("%s", rtmsg.SocketWrongKind)
		}
		return in.acceptHTTPRequest(args[0].Conn), true
	case "отправить_http_ответ":
		if args[0].Conn == nil {
			panicRuntime("%s", rtmsg.SocketWrongKind)
		}
		in.sendHTTPResponse(args[0].Conn, args[1])
		return Value{}, true
	case "разобрать_multipart":
		return parseMultipart(bytesFromValue(args[0]), args[1].S), true
	case "строка_из_байт":
		return stringValue(string(bytesFromValue(args[0]))), true
	case "байты_из_строки":
		return bytesValue([]byte(args[0].S)), true
	case "принять_http_байты":
		if args[0].Conn == nil {
			panicRuntime("%s", rtmsg.SocketWrongKind)
		}
		return in.acceptHTTPRequestBytes(args[0].Conn), true
	case "отправить_http_ответ_байты":
		if args[0].Conn == nil {
			panicRuntime("%s", rtmsg.SocketWrongKind)
		}
		in.sendHTTPResponseBytes(args[0].Conn, args[1])
		return Value{}, true
	case "параметры_пути":
		return queryParams(args[0].S), true
	case "разбить":
		return stringSplit(args[0].S, args[1].S), true
	case "регулярное":
		return regexFindAll(args[0].S, args[1].S), true
	case "длина_строки":
		return intValue(stringLength(args[0].S)), true
	case "подстрока":
		return stringValue(substring(args[0].S, args[1].I, args[2].I)), true
	case "позиция_в_строке":
		return intValue(stringFind(args[0].S, args[1].S)), true
	case "случайное":
		return floatValue(randomFloat()), true
	case "случайное_целое":
		return intValue(randomInt(args[0].I, args[1].I)), true
	case "время":
		return intValue(nowUnix()), true
	case "хеш":
		return stringValue(hashByAlgorithm(int(args[0].I), args[1].S)), true
	case "подпись_hmac_sha256":
		return stringValue(hmacSHA256Hex(args[0].S, args[1].S)), true
	case "закодировать_base64url":
		return stringValue(base64URLEncode(args[0].S)), true
	case "декодировать_base64url":
		return stringValue(base64URLDecode(args[0].S)), true
	case "зашифровать":
		return stringValue(aesEncryptHex(args[0].S, args[1].S)), true
	case "расшифровать":
		return stringValue(aesDecryptHex(args[0].S, args[1].S)), true
	case "сгенерировать_приватный_ключ":
		return stringValue(generatePrivateKeyPEM(int(args[0].I))), true
	case "получить_публичный_ключ":
		return stringValue(getPublicKeyPEM(args[0].S)), true
	case "подписать":
		return stringValue(signHex(args[0].S, args[1].S)), true
	case "проверить_подпись":
		return boolValue(verifySignatureHex(args[0].S, args[1].S, args[2].S)), true
	case "запустить_процесс":
		elems := args[1].Obj.coll.elems
		procArgs := make([]string, len(elems))
		for i, e := range elems {
			procArgs[i] = e.S
		}
		return spawnProcess(args[0].S, procArgs), true
	case "дождаться_процесса":
		return waitProcess(args[0]), true
	case "выполнить_команду":
		return runCommand(args[0].S, args[1].S), true
	case "время_как_дата":
		return timeToDate(args[0].I), true
	case "дата_как_время":
		return intValue(dateToTime(args[0])), true
	case "дату_в_строку":
		return stringValue(dateToString(args[0])), true
	case "rgbцвет_из_hex":
		return hexToColor(args[0].S), true
	case "rgbцвет_в_hex":
		return stringValue(colorToHex(args[0])), true
	case "декодировать_изображение":
		return decodeImageFile(args[0].S, args[1].I), true
	case "закодировать_изображение":
		encodeImageFile(args[0].S, args[1].I, args[2].I, bytesFromValue(args[3]), args[4].I, args[5].I)
		return Value{}, true
	case "значение_куки":
		return cookieValue(args[0], args[1].S), true
	case "значение_заголовка":
		return headerValue(args[0].S, args[1].S), true
	case "получить_переменную_окружения":
		return envValue(args[0].S), true
	case "строка_куки":
		return stringValue(cookieString(args[0].S, args[1].S, args[2])), true
	case "подключиться_к_бд":
		return in.dbConnect(args[0].S), true
	case "запросить":
		return in.dbQuery(args[0].DB, args[1].S, reflectListElems(args[2])), true
	case "выполнить":
		return in.dbExec(args[0].DB, args[1].S, reflectListElems(args[2])), true
	case "закрыть_бд":
		args[0].DB.Close()
		return Value{}, true
	case "закрыть_сокет":
		switch {
		case args[0].Conn != nil:
			args[0].Conn.Close()
		case args[0].Listener != nil:
			args[0].Listener.Close()
		case args[0].PacketConn != nil:
			args[0].PacketConn.Close()
		default:
			panicRuntime("%s", rtmsg.SocketWrongKind)
		}
		return Value{}, true
	case "http2_принять_сессию":
		return in.http2AcceptSession(args[0]), true
	case "новая_http2_сессия":
		return in.http2NewSession(args[0].S, args[1].I), true
	case "http2_закрыть_сессию":
		in.http2CloseSession(args[0])
		return Value{}, true
	case "http2_принять_поток":
		return in.http2AcceptStream(args[0]), true
	case "http2_открыть_поток":
		return in.http2OpenStream(args[0], args[1].S, args[2].S), true
	case "http2_путь":
		return in.http2Path(args[0]), true
	case "http2_метод":
		return in.http2Method(args[0]), true
	case "http2_статус":
		return in.http2StatusOf(args[0]), true
	case "http2_заголовок":
		return in.http2HeaderOf(args[0], args[1].S), true
	case "http2_прочитать_данные":
		return in.http2ReadData(args[0], args[1].I), true
	case "http2_прочитать_всё":
		return in.http2ReadAll(args[0]), true
	case "http2_отправить_заголовки":
		in.http2SendHeaders(args[0], args[1].I)
		return Value{}, true
	case "http2_отправить_данные":
		in.http2SendData(args[0], args[1])
		return Value{}, true
	case "http2_отправить_трейлер":
		in.http2SendTrailer(args[0], args[1].S, args[2].S)
		return Value{}, true
	case "http2_завершить_поток":
		in.http2FinishStream(args[0])
		return Value{}, true
	case "grpc_открыть_поток":
		return in.grpcOpenStream(args[0], args[1].S), true
	case "grpc_начать_ответ":
		in.grpcSendHeaders(args[0])
		return Value{}, true
	case "grpc_прочитать_сообщение":
		return in.grpcReadMessage(args[0]), true
	case "grpc_отправить_сообщение":
		in.grpcSendMessage(args[0], bytesFromValue(args[1]))
		return Value{}, true
	case "grpc_завершить":
		in.grpcFinish(args[0], args[1].I, args[2].S)
		return Value{}, true
	case "grpc_статус":
		return in.grpcStatus(args[0]), true
	case "grpc_сообщение_ошибки":
		return in.grpcErrorMessage(args[0]), true
	default:
		return Value{}, false
	}
}

// openFile - общая реализация "открыть"/"открыть_для_записи"/
// "открыть_для_дозаписи" - name (реальное имя вызванной функции) идёт в
// сообщение об ошибке (rtmsg.OpenFailedFormat), 0o644 - права нового файла
// для режимов записи/дозаписи (POSIX-права ничего не значат при чтении
// уже существующего файла, поэтому там передаётся 0 - os.OpenFile его
// просто игнорирует без os.O_CREATE).
func (in *Interp) openFile(name, path string, flag int, perm os.FileMode) Value {
	f, err := os.OpenFile(path, flag, perm)
	if err != nil {
		panicRuntime("%s", rtmsg.OpenFailedFormat(name, path))
	}
	return fileValue(f)
}

// writeWholeFile - общая реализация "записать_в_файл"/"дописать_в_файл"
// (шаг 8, высокоуровневые обёртки без ручного открыть/закрыть) - flag
// отличает запись (обрезает существующее содержимое) от дозаписи, как и у
// openFile выше.
func (in *Interp) writeWholeFile(name, path, content string, flag int) {
	f := in.openFile(name, path, flag, 0o644).File
	n, err := f.WriteString(content)
	f.Close()
	if err != nil || n != len(content) {
		panicRuntime("%s", rtmsg.WriteIOFailed)
	}
}

// readLine читает одну строку из in.stdin, обрезая завершающий перенос
// строки и (для прочитать_целое/прочитать_число, где важно совпадение с
// cbackend побайтово) пробелы/табы по краям - тот же набор символов, что и
// rech_trim_line в сгенерированном C. Конец ввода без единого прочитанного
// байта - panicRuntime (аварийное завершение, как и rech_panic в C), не
// пользовательское исключение - конец стандартного ввода не то, что
// программа обычно ожидает перехватывать и продолжать после.
func (in *Interp) readLine() string {
	line, err := in.stdin.ReadString('\n')
	if err != nil && line == "" {
		panicRuntime("%s", rtmsg.ReadEOF)
	}
	return strings.Trim(line, " \t\r\n")
}

// httpRequest - "запрос" (шаг 9, HTTP-клиент типа curl) - тот же простой
// протокол, что и cbackend.rech_http_request (см. package doc там):
// "Connection: close" в каждом запросе, ответ читается до закрытия
// соединения (io.ReadAll), не по Content-Length - простой способ получить
// ВСЕ байты ответа целиком. Chunked transfer encoding - РАЗБИРАЕТСЯ
// (docs/ROADMAP.md, HTTP highload пункт 4, chunked.go) - раз io.ReadAll уже
// дочитал весь чанкованный поток целиком (до закрытия соединения),
// достаточно раскодировать его одним проходом (dechunk) уже после приёма.
func (in *Interp) httpRequest(req Value) Value {
	f := req.Obj.fields
	method, host, path, headers, body := f["метод"].S, f["адрес"].S, f["путь"].S, f["заголовки"].S, f["тело"].S
	port := f["порт"].I

	// "https://"/"http://" в начале "адреса" - тем же принципом, что и
	// cbackend.rech_http_request (см. её package doc) - обе схемы
	// отрезаются перед подключением/заголовком "Host:".
	isHTTPS := false
	if rest, ok := strings.CutPrefix(host, "https://"); ok {
		isHTTPS = true
		host = rest
	} else if rest, ok := strings.CutPrefix(host, "http://"); ok {
		host = rest
	}

	addr := net.JoinHostPort(host, strconv.FormatInt(port, 10))
	var conn net.Conn
	var err error
	if isHTTPS {
		// tls.Dial - TCP-соединение + рукопожатие ОДНИМ вызовом (в отличие
		// от cbackend, где это два раздельных шага, rech_connect_tcp +
		// SSL_connect, - здесь их незачем разделять, у Go нет отдельного
		// "голого" TCP-соединения, которое нужно было бы регистрировать в
		// каком-то реестре, как в cbackend, tlsConnectCode) - ServerName
		// для проверки сертификата берётся из host (SNI).
		conn, err = tls.Dial("tcp", addr, &tls.Config{ServerName: host})
		if err != nil {
			panicRuntime("%s", rtmsg.TLSClientHandshakeFailed)
		}
	} else {
		conn, err = net.Dial("tcp", addr)
		if err != nil {
			panicRuntime("%s", rtmsg.SocketConnectFailed)
		}
	}
	defer conn.Close()

	hdrSep := ""
	if headers != "" {
		hdrSep = "\r\n"
	}
	head := fmt.Sprintf("%s %s HTTP/1.1\r\nHost: %s\r\n%s%sConnection: close\r\nContent-Length: %d\r\n\r\n",
		method, path, host, headers, hdrSep, len(body))
	if _, err := io.WriteString(conn, head+body); err != nil {
		panicRuntime("%s", rtmsg.SocketSendFailed)
	}
	raw, err := io.ReadAll(conn)
	if err != nil {
		panicRuntime("%s", rtmsg.HTTPRecvFailed)
	}
	rawS := string(raw)

	var code int64
	if _, rest, ok := strings.Cut(rawS, " "); ok {
		code = parseLeadingInt(rest)
	}
	respHeaders, respBody, ok := strings.Cut(rawS, "\r\n\r\n")
	if !ok {
		respHeaders, respBody = "", ""
	} else if isChunkedHeaders([]byte(respHeaders)) {
		respBody = string(dechunk([]byte(respBody)))
	}
	return Value{Type: ir.EntityType("Ответ"), Obj: &entityObj{typeName: "Ответ", fields: map[string]Value{
		"код":       intValue(code),
		"заголовки": stringValue(respHeaders),
		"тело":      stringValue(respBody),
	}}}
}

// parseLeadingInt разбирает ведущее (возможно со знаком) десятичное число в
// начале s, останавливаясь на первом не-цифровом символе - тот же принцип,
// что и C strtoll (rech_http_request), не strconv.ParseInt (которому нужна
// строка ЦЕЛИКОМ числом, "200 OK" не подходит).
func parseLeadingInt(s string) int64 {
	end := 0
	if end < len(s) && (s[end] == '+' || s[end] == '-') {
		end++
	}
	for end < len(s) && s[end] >= '0' && s[end] <= '9' {
		end++
	}
	v, _ := strconv.ParseInt(s[:end], 10, 64)
	return v
}
