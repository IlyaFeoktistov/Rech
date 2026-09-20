package rtmsg

import "slices"

// httpReasonPhrases - код состояния HTTP -> стандартная (IANA) фраза-
// причина, курируемый список самых частых кодов, не исчерпывающий IANA
// registry (тот же принцип, что и StandardEnums.HTTPКод, internal/
// semantic/types.go). Единая точка истины для cbackend (эмитирует C-
// таблицу/switch) и interp (использует Go-карту напрямую) - до
// 2026-09-20 оба безусловно писали "OK" в статус-строке независимо от
// кода (`HTTP/1.1 404 OK` вместо `HTTP/1.1 404 Not Found`) - живая
// находка при аудите HTTP-сервера, не влияет на подавляющее большинство
// клиентов (они парсят только числовой код, фраза - для человека/лога),
// но нарушает RFC 7230 §3.1.2 без всякой причины, раз сама фраза уже
// известна.
var httpReasonPhrases = map[int64]string{
	100: "Continue",
	101: "Switching Protocols",
	200: "OK",
	201: "Created",
	202: "Accepted",
	203: "Non-Authoritative Information",
	204: "No Content",
	205: "Reset Content",
	206: "Partial Content",
	300: "Multiple Choices",
	301: "Moved Permanently",
	302: "Found",
	303: "See Other",
	304: "Not Modified",
	307: "Temporary Redirect",
	308: "Permanent Redirect",
	400: "Bad Request",
	401: "Unauthorized",
	402: "Payment Required",
	403: "Forbidden",
	404: "Not Found",
	405: "Method Not Allowed",
	406: "Not Acceptable",
	407: "Proxy Authentication Required",
	408: "Request Timeout",
	409: "Conflict",
	410: "Gone",
	411: "Length Required",
	412: "Precondition Failed",
	413: "Payload Too Large",
	414: "URI Too Long",
	415: "Unsupported Media Type",
	416: "Range Not Satisfiable",
	417: "Expectation Failed",
	418: "I'm a Teapot",
	422: "Unprocessable Entity",
	425: "Too Early",
	426: "Upgrade Required",
	428: "Precondition Required",
	429: "Too Many Requests",
	431: "Request Header Fields Too Large",
	451: "Unavailable For Legal Reasons",
	500: "Internal Server Error",
	501: "Not Implemented",
	502: "Bad Gateway",
	503: "Service Unavailable",
	504: "Gateway Timeout",
	505: "HTTP Version Not Supported",
	507: "Insufficient Storage",
	508: "Loop Detected",
	511: "Network Authentication Required",
}

// httpReasonClassFallback - код ВНЕ курируемого списка выше - общая фраза
// по первой цифре (та же деградация, что делают многие настоящие
// HTTP-серверы для нестандартных кодов), не "OK" безусловно и не пустая
// строка (клиенты ожидают ХОТЬ ЧТО-ТО после кода).
var httpReasonClassFallback = map[int64]string{
	1: "Informational",
	2: "Success",
	3: "Redirection",
	4: "Client Error",
	5: "Server Error",
}

// HTTPReasonPhrase - фраза-причина для code (курируемая таблица выше, иначе
// - по классу первой цифры, иначе - "Unknown").
func HTTPReasonPhrase(code int64) string {
	if phrase, ok := httpReasonPhrases[code]; ok {
		return phrase
	}
	if phrase, ok := httpReasonClassFallback[code/100]; ok {
		return phrase
	}
	return "Unknown"
}

// HTTPReasonPhrasesForC - весь httpReasonPhrases в стабильном (по коду)
// порядке - только для cbackend (эмитирует таблицу C-кода один раз при
// генерации), interp зовёт HTTPReasonPhrase напрямую в рантайме и в этом
// не нуждается.
func HTTPReasonPhrasesForC() []struct {
	Code   int64
	Phrase string
} {
	codes := make([]int64, 0, len(httpReasonPhrases))
	for code := range httpReasonPhrases {
		codes = append(codes, code)
	}
	slices.Sort(codes)
	out := make([]struct {
		Code   int64
		Phrase string
	}, len(codes))
	for i, code := range codes {
		out[i] = struct {
			Code   int64
			Phrase string
		}{Code: code, Phrase: httpReasonPhrases[code]}
	}
	return out
}
