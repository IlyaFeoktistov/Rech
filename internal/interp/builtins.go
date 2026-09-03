package interp

import (
	"fmt"
	"io"
	"math"
	"net"
	"os"
	"strconv"
	"strings"

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
		if args[0].Listener == nil {
			panicRuntime("%s", rtmsg.SocketWrongKind)
		}
		c, err := args[0].Listener.Accept()
		if err != nil {
			panicRuntime("%s", rtmsg.SocketAcceptFailed)
		}
		return connValue(c), true
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
	case "параметры_пути":
		return queryParams(args[0].S), true
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
	case "время_как_дата":
		return timeToDate(args[0].I), true
	case "дата_как_время":
		return intValue(dateToTime(args[0])), true
	case "дата_в_строку":
		return stringValue(dateToString(args[0])), true
	case "значение_куки":
		return cookieValue(args[0], args[1].S), true
	case "строка_куки":
		return stringValue(cookieString(args[0].S, args[1].S, args[2])), true
	case "подключиться_к_бд":
		return dbConnect(args[0].S), true
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
// соединения (io.ReadAll), не по Content-Length/chunked - оба backend'а
// должны разбирать ответ одинаково (в т.ч. одинаково НЕ уметь chunked),
// иначе интерпретатор был бы незаметно "умнее" собранной программы для
// одного и того же Rech-кода.
func (in *Interp) httpRequest(req Value) Value {
	f := req.Obj.fields
	method, host, path, headers, body := f["метод"].S, f["адрес"].S, f["путь"].S, f["заголовки"].S, f["тело"].S
	port := f["порт"].I

	conn, err := net.Dial("tcp", net.JoinHostPort(host, strconv.FormatInt(port, 10)))
	if err != nil {
		panicRuntime("%s", rtmsg.SocketConnectFailed)
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
