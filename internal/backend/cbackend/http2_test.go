package cbackend

import (
	"strings"
	"testing"
)

// TestGenerateFromIRHTTP2WithoutSessionEstablishingCallIncludesSocketHeaders -
// ЖИВОЙ НАЙДЕННЫЙ БАГ (Шаг 96/97): функция, работающая только НАД уже
// открытой HTTP2Сессия (grpc_отправить_сообщение и т. п. - сама сессия
// открывается ГДЕ-ТО ЕЩЁ, например вызывающей программой через "rech
// proto"-сгенерированную обёртку), ни разу не звала http2_принять_сессию/
// новая_http2_сессия/запрос - ЕДИНСТВЕННЫЕ места, раньше резолвившие
// "?Сокет" - при этом usesHTTP2 всё равно заставляет ir_generator.go
// включить usesSockets (rech_http2_io_thread/accept_session зовут accept()/
// recv() напрямую), а socketHelpers БЕЗУСЛОВНО несёт rech_accept,
// возвращающую "?Сокет" - без регистрации внутри registerHTTP2TypesOnCall
// (http2.go) сборка настоящим cc падала на "unknown type name" именно на
// "rech build --lib" (Шаг 96/97) - здесь достаточно
// проверить, что "?Сокет"-обёртка и заголовки сокетов попали в вывод, без
// реального cc (тот уже прогнан живьём при отладке).
func TestGenerateFromIRHTTP2WithoutSessionEstablishingCallIncludesSocketHeaders(t *testing.T) {
	out := generateFromSrc(t, `
функция отправитьПривет(сессия: HTTP2Сессия)
{
    переменная поток = grpc_открыть_поток(сессия, "/пакет.Сервис/Метод")
    grpc_отправить_сообщение(поток, байты_из_строки("привет"))
    http2_завершить_поток(поток)
}

функция главная
{
    вывести 1
}
`)
	for _, want := range []string{
		"#include <sys/socket.h>",
		"#include <errno.h>",
		"rech_accept(",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("ожидалось вхождение %q, получено:\n%s", want, out)
		}
	}
}
