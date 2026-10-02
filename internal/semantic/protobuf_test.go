package semantic

import "testing"

// TestAnalyzeProtobufParseNarrowingAllowsByteListSource - "разобрать
// PROTOBUF" обязан разворачивать доказанно непустой "?Список<Байт>" тем же
// путём, что и "сериализовать PROTOBUF"/JSON/XML (analyzeParseExpr,
// parse.go) - НАЙДЕННЫЙ ЖИВОЙ БАГ (при написании examples/сеть/
// grpc_привет.rech, grpc_прочитать_сообщение возвращает "?Список<Байт>"):
// analyzeProtobufParseExpr звал requireByteListType(ir.TypeOf(source), ...)
// напрямую, без maybeUnwrapNarrowed - "если тело != Ничто { разобрать
// PROTOBUF тело как ... }" отклонялось компилятором, хотя "тело" уже
// доказанно не Ничто в этой точке (та же программа с "сериализовать
// PROTOBUF" вместо "разобрать" компилировалась без ошибок - асимметрия).
func TestAnalyzeProtobufParseNarrowingAllowsByteListSource(t *testing.T) {
	_, err := AnalyzeSource(t, `
структура Сообщение
{
    имеет:
        Строка текст `+"`protobuf:1`"+`
}

функция главная
{
    переменная байты: ?Список<Байт> = Ничто
    байты = байты_из_строки("x")
    если байты != Ничто {
        переменная разобранный = разобрать PROTOBUF байты как Сообщение
        вывести разобранный.текст
    }
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}
