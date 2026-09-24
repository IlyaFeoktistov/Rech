package interp

import (
	"bytes"
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/lexer"
	"github.com/IlyaFeoktistov/Rech/internal/parser"
	"github.com/IlyaFeoktistov/Rech/internal/semantic"
)

// runSrcWithDBPoolTimeoutMs - как runSrc (interp_test.go), но с
// irProg.DBPoolTimeoutMs, выставленным ПОСЛЕ semantic.Analyze, ДО Run -
// тем же порядком, что и internal/compiler.defaultBackend.Generate
// (единственное реальное место, которое это делает вне тестов). Нужен
// отдельно от cbackend-аналога (internal/backend/cbackend/dbtimeout_test.go,
// generateFromSrcWithDBPoolTimeoutMs) - тот собирает C и запускает
// настоящий cc, здесь - чистый Go, без него.
func runSrcWithDBPoolTimeoutMs(t *testing.T, src string, timeoutMs int) string {
	t.Helper()
	toks, err := lexer.New(src).Tokenize()
	if err != nil {
		t.Fatalf("лексер: %v", err)
	}
	prog, err := parser.New(toks).ParseProgram()
	if err != nil {
		t.Fatalf("парсер: %v", err)
	}
	irProg, err := semantic.Analyze(prog)
	if err != nil {
		t.Fatalf("семантический анализ: %v", err)
	}
	irProg.DBPoolTimeoutMs = timeoutMs
	var buf bytes.Buffer
	if err := Run(irProg, &buf, strings.NewReader("")); err != nil {
		t.Fatalf("исполнение: %v", err)
	}
	return buf.String()
}

// dbPoolContentionTestSrc - пул размера 1, одна "запустить"-задача держит
// единственное соединение открытой транзакцией (не запросом - у interp
// таймаут кладётся под ОДИН контекст и ожидание слота пула, и время
// выполнения самого запроса, см. package doc dbCheckoutContext, db.go - и
// без транзакции "чем занять слот" пришлось бы делать медленным ЗАПРОСОМ,
// который сам бы тоже словил этот же таймаут, смешивая два разных случая)
// - занятость идёт обычным "пока"-циклом, без единого обращения к БД внутри
// самой транзакции, поэтому таймаут второго потока проверяет РОВНО ожидание
// свободного слота, не время выполнения чужого запроса. Главный поток (с
// небольшой головной задержкой, убирающей гонку планировщика - без неё он
// мог бы успеть сделать свой запрос ДО того, как задача вообще захватила
// соединение) пробует второй запрос на том же "БД".
const dbPoolContentionTestSrc = `
функция держать(бд: БД)
{
    начать_транзакцию(бд, функция(тр: Транзакция)
    {
        переменная i = 0
        пока i < 3000000
        {
            i = i + 1
        }
    })
}
функция главная
{
    переменная бд = подключиться_к_бд("sqlite::memory:")
    выполнить(бд, "create table t(id integer)", новый Список<Значение>())
    переменная задача = запустить держать(бд)
    переменная i = 0
    пока i < 500000
    {
        i = i + 1
    }
    попытаться
    {
        переменная строки = запросить(бд, "select id from t", новый Список<Значение>())
        вывести "второй запрос: успел\n"
    }
    поймать (сообщение: Паника)
    {
        вывести ` + "`${" + `"второй запрос: " + (Строка) сообщение}\n` + "`" + `
    }
    дождаться задача
    закрыть_бд(бд)
}
`

// TestDBPoolTimeoutMsInterpTimesOutUnderContention - НАСТОЯЩИЙ
// интерпретатор (Run, не "rech run" - тот компилирует и исполняет
// cbackend-бинарник, интерпретатора не касается вовсе), таймаут 300 мс -
// второй запрос обязан не дождаться свободного соединения и поймать
// rtmsg.DBCheckoutTimeout.
func TestDBPoolTimeoutMsInterpTimesOutUnderContention(t *testing.T) {
	out := runSrcWithDBPoolTimeoutMs(t, dbPoolContentionTestSrc, 300)
	if !strings.Contains(out, "БД: не удалось получить свободное соединение из пула вовремя") {
		t.Fatalf("ожидался таймаут пула, получено %q", out)
	}
}

// TestDBPoolTimeoutMsInterpDefaultWaitsForever - таймаут 0 (умолчание,
// поведение до Шага 69) - второй запрос ждёт и в итоге успевает, без
// регрессии.
func TestDBPoolTimeoutMsInterpDefaultWaitsForever(t *testing.T) {
	out := runSrcWithDBPoolTimeoutMs(t, dbPoolContentionTestSrc, 0)
	if !strings.Contains(out, "второй запрос: успел") {
		t.Fatalf("ожидался успешный второй запрос без таймаута, получено %q", out)
	}
}
