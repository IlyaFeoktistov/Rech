package interp

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/IlyaFeoktistov/Rech/internal/lexer"
	"github.com/IlyaFeoktistov/Rech/internal/parser"
	"github.com/IlyaFeoktistov/Rech/internal/semantic"
)

func runSrc(t *testing.T, src string) string {
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
	var buf bytes.Buffer
	if err := Run(irProg, &buf, strings.NewReader("")); err != nil {
		t.Fatalf("исполнение: %v", err)
	}
	return buf.String()
}

// TestRunAllExamplesMatchCompiledOutput - сквозная проверка интерпретатора
// на каждом примере из examples/: ожидаемый вывод захвачен вручную реальным
// запуском скомпилированной (через cbackend + настоящий cc) программы -
// байт в байт совпадение с интерпретатором подтверждает, что второй backend
// поверх IR даёт то же наблюдаемое поведение, что и первый. В отличие от
// одноразовых zz_smoke_test.go у cbackend (не коммитятся - зависят от
// настоящего cc), этот тест постоянный: интерпретатор не вызывает системный
// компилятор вообще.
func TestRunAllExamplesMatchCompiledOutput(t *testing.T) {
	root, err := filepath.Abs("../../examples")
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	// "открыть" в основы/файлы.rech читает путь относительно рабочего
	// каталога процесса (как и настоящий rech run) - тесты по умолчанию
	// запускаются с рабочим каталогом самого пакета, а не корня репозитория,
	// поэтому здесь он временно переключается на корень репозитория (тот же
	// каталог, откуда "rech run" ожидается запущенным - см. комментарий в
	// самом примере).
	repoRoot := filepath.Dir(root)
	origWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWD) })
	cases := map[string]string{
		"исключения/деление.rech":                          "деление: \nделение на ноль\nснаружи: \nнет данных\n",
		"исключения/встроенные.rech":                       "поймано: \nделение на ноль\nнедостаточно прав\n403\n",
		"исключения/окончательно.rech":                     "БД: \nнет соединения\nготово для режима \n1\nпаника или иное исключение: \nутверждение не выполнено (строка 23)\nготово для режима \n2\nуспех\nготово для режима \n3\nочистка перед возвратом\n42\n",
		"коллекции/очередь.rech":                           "истина\n3\n1\n1\n2\n1\nложь\n",
		"параллелизм/задачи.rech":                          "2\n4\n6\nпоймано: \nне удалось\n",
		"обобщённые/коробка.rech":                          "5\n1\nтекст\nпривет\nмяу\n",
		"коллекции/список.rech":                            "3\n10\n30\n99\nложь\n10\n99\n30\n",
		"коллекции/вложенные_списки.rech":                  "2\n2\n1\n2\n3\n4\n",
		"коллекции/безопасная_индексация.rech":             "10\n-1\n999\n1\n-1\n2\n",
		"коллекции/литерал_списка.rech":                    "0\n4\n1\n5\n3\nбб\n",
		"коллекции/словарь.rech":                           "2\n31\nистина\nложь\n1\nложь\n",
		"коллекции/стопка.rech":                            "истина\n3\n3\n3\n2\n1\nложь\n",
		"коллекции/передача_по_ссылке.rech":                "истина\n1\n1\n99\n2\n",
		"основы/выражения.rech":                            "Привет, Илья!\n13\n7\n30\n3\n-10\n4\nистина\nистина\nложь\nложь\nистина\nложь\nистина\n11\n",
		"основы/файлы.rech":                                "привет\n, файл\n\nистина\nпервая строка\nвторая строка\nперезаписано целиком\nи дописано\n\n",
		"основы/побитовые.rech":                            "7\n2\n5\n-1\n8\n8\n5\n",
		"основы/инкремент.rech":                            "5\n6\n7\n7\n8\n7\n1.5\n2.5\n",
		"основы/составное_присваивание.rech":               "15\n12\n24\n6\nпривет, мир\n7\n",
		"основы/значение_или_ничто.rech":                   "истина\n10\n10\n99\n5\n10\nАня\nне найден\n",
		"основы/константа_модуля.rech":                     "3\nдобро пожаловать\n5\n",
		"основы/привет.rech":                               "Привет, мир!\n",
		"основы/показать.rech":                             "5 (Целое)\n3.5 (Действительное)\n\"текст\" (Строка)\nистина (Логическое)\nТочка { x: 1 (Целое), y: 2.5 (Действительное) }\nСписок<Целое> [10 (Целое), 20 (Целое)]\n",
		"основы/строки.rech":                               "строка1\nстрока2\nколонка1\tколонка2\nон сказал \"привет\"\nпуть: C:\\rech\nистина\nистина\nxy\n",
		"основы/строковые_операции.rech":                   "12\nпривет\nмир\n8\n-1\nabc123\n",
		"основы/типы_и_касты.rech":                         "45.5\n6.21828\n1e+09\n0.0025\nя\n200\n1103\na\n200\n232\n2.71828\n1.25\nю\nю\n200\n",
		"основы/целые_ширины.rech":                         "127\n73\n-116\n44\n18446744073709551615\n9223372036854775807\n18446744073709551615\n1.84467e+19\n5\n-5\n-2000000000\n6\n5\nистина\nистина\n",
		"основы/регулярные_выражения.rech":                 "Привет\nМир\nRech\nвелик\nпервое число: 120, позиция: 6\nадрес: a@b.com, пользователь: a, домен: b.com\nадрес: x@y.org, пользователь: x, домен: y.org\nцифр не найдено\n4\n",
		"основы/рефлексия.rech":                            "Кот\n1\nимя\n1\n7\nМурка\nБарсик\nмяу\nРыжик\nМяукающий\n1\nмяу\nЖивотное\nистина\n1\nМяукающий\nистина\n",
		"основы/тег_поля.rech":                             "users\nтаблица:users, версия:2\nимя\nuser_name\nвозраст\nage\nактивен\n\njson:name, sql:user_name\nзапись\nвыполняет:запись\n",
		"основы/управление.rech":                           "больше пяти\n15\n120\nистина\n",
		"основы/выбор.rech":                                "два или три\nвыходной\nадминистратор\nобычный пользователь\n",
		"основы/время.rech":                                "2023\n11\n14\n22\n13\n20\n2023-11-14 22:13:20\n1700000000\n",
		"json/разобрать.rech":                              "Аня\n30\nистина\nМосква\n3\n3\nразобрать JSON: отсутствует поле \"имя\"\n",
		"json/сериализовать.rech":                          "{\"имя\":\"Аня\",\"возраст\":30,\"адрес\":{\"город\":\"Москва\"}}\n[1,2,3]\n\"текст с \\\"кавычками\\\"\"\nАня\nМосква\n",
		"json/разобрать_xml.rech":                          "Аня\n30\nистина\nМосква\n3\n3\nразобрать XML: отсутствует поле \"имя\"\n",
		"json/сериализовать_xml.rech":                      "<значение><имя>Аня</имя><возраст>30</возраст><адрес><город>Москва</город></адрес></значение>\n<значение><элемент>1</элемент><элемент>2</элемент><элемент>3</элемент></значение>\n<значение>текст с &lt;тегами&gt; и амперсандом &amp;</значение>\nАня\nМосква\n",
		"json/теги.rech":                                   "Илья\n30\nистина\n{\"name\":\"Илья\",\"age\":30,\"активен\":true}\n",
		"бд/подключение.rech":                              "1\n2\nИлья\n9.5\nАня\nНичто\n2\nИлья\n9.5\nАня\n-1\n",
		"бд/транзакции.rech":                               "70\nожидаемая ошибка - откат\n70\n",
		"указатели/адрес_и_разыменование.rech":             "5\n[указатель]\n99\n1\n42\n20\n77\n99\n123\n",
		"сущности/вложенные.rech":                          "Илья\nМосква\nСанкт-Петербург\nКазань\n",
		"сущности/класс.rech":                              "1\n2\n4\n6\n",
		"сущности/естественный_вызов_метода.rech":          "истина\nистина\n2\nложь\n3\n",
		"сущности/естественный_вызов_метода_параметр.rech": "истина\n",
		"сущности/естественный_вызов_метода_падежи.rech":   "500\n",
		"сущности/конструктор.rech":                        "Илья\n30\nМудрая\n800\n600\n1\n",
		"сущности/ковариантный_возврат.rech":               "Илья\n42\n",
		"сущности/наследование.rech":                       "мяу\nгав\nМурка\n",
		"сущности/наследование_интерфейсов.rech":           "мяу\nМурка\n",
		"сущности/причастие.rech":                          "Аня\n",
		"сущности/глагольные_семьи.rech":                   "Аня\nобъект типа \"Пользователь\" не находится в требуемом состоянии \"вошедший\"\nбольше не вошедший\nобъект типа \"Пользователь\" не находится в требуемом состоянии \"вошедший\"\n",
		"сущности/состояния.rech":                          "Профиль: Илья\nПрофиль: Аня\nобъект типа \"Пользователь\" не находится в требуемом состоянии \"авторизован\"\nОля не авторизована\n",
		"сущности/сущности.rech":                           "Илья\n30\n31\nИлья\n32\n150\n",
		"сущности/статика.rech":                            "150\n2\n2\n1\n2\n3\n",
		"сущности/явный_конструктор.rech":                  "Илья\n0\nАня\n0\n1000\n",
		"функции/вызываемый.rech":                          "42\n22\n1\n2\n0\n",
		"функции/естественный_вызов.rech":                  "Привет, Илья! Добро пожаловать в чат\n5\n5\n5\n",
		"функции/значения_по_умолчанию.rech":               "15\n6\n1\n5\nПривет, Аня\nУважаемая, Аня\n",
		"функции/падежные_роли.rech":                       "5\n10\n24\n14\n7\n",
		"функции/параметры.rech":                           "Оформлен: Илья\n1\nОформлен: Илья\n1\n",
		"функции/строгий_вызов.rech":                       "Привет, Илья! Добро пожаловать в чат\n5\n",
	}
	for rel, want := range cases {
		t.Run(rel, func(t *testing.T) {
			src, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			got := runSrc(t, string(src))
			if got != want {
				t.Fatalf("вывод не совпадает с эталонным (скомпилированным) -\nхотели:\n%q\nполучили:\n%q", want, got)
			}
		})
	}
}

// TestEvalAsInterfaceMismatchPanicsWithClearMessage - "значение как
// ИмяИнтерфейса" (docs/ROADMAP.md - находка "значение как ИмяИнтерфейса не
// распаковывает боксированную реализацию") для сущности, НЕ реализующей
// интерфейс, - паника с тем же текстом, что и cbackend
// (rtmsg.AsInterfaceMismatchFormat, см. internal/backend/cbackend/
// reflect_test.go, аналогичная проверка). Не через runSrc (тот
// t.Fatalf'ит на любую ошибку Run) - нужен именно текст ошибки.
func TestEvalAsInterfaceMismatchPanicsWithClearMessage(t *testing.T) {
	src := `
интерфейс Звучащий
{
    издать_звук(): Строка
}
сущность Стол
{
    имеет:
        ножки: Целое
}
функция главная
{
    переменная коробка = (Значение) новый Стол()
    переменная звучащий = коробка как Звучащий
}
`
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
	var buf bytes.Buffer
	err = Run(irProg, &buf, strings.NewReader(""))
	if err == nil || !strings.Contains(err.Error(), "фактический тип значения (Стол) не реализует этот интерфейс") {
		t.Fatalf("ожидалась паника про несоответствие интерфейсу, получено: %v", err)
	}
}

// TestRunBaseTypeAndInterfaceReflectionMethods - "базовый_тип"/
// "наследует_ли"/"число_интерфейсов"/"получить_интерфейс"/"реализует_ли"
// (docs/ROADMAP.md, паритет с C# Type.BaseType/IsSubclassOf/GetInterfaces/
// IsAssignableFrom) - транзитивно через ОБЕ цепочки ("наследует" у
// сущности И у самого интерфейса, "Звучащий наследует Именуемый", "Кот
// реализует" только "Звучащий").
func TestRunBaseTypeAndInterfaceReflectionMethods(t *testing.T) {
	got := runSrc(t, `
интерфейс Именуемый
{
    имя_объекта(): Строка
}
интерфейс Звучащий наследует Именуемый
{
    издать_звук(): Строка
}
сущность Животное
{
    имеет:
        имя: Строка
}
сущность Кот наследует Животное реализует Звучащий
{
    имя_объекта(): Строка
    {
        вернуть этот.имя
    }

    издать_звук(): Строка
    {
        вернуть "мяу"
    }
}
функция главная
{
    переменная типКота = тип Кот
    переменная типЖивотного = тип Животное

    переменная база = типКота.базовый_тип()
    вывести база.имя()
    переменная безБазы = типЖивотного.базовый_тип()
    вывести безБазы.имя()

    вывести типКота.наследует_ли(типЖивотного)
    вывести типЖивотного.наследует_ли(типКота)

    вывести типКота.число_интерфейсов()
    вывести типКота.реализует_ли(тип Звучащий)
    вывести типКота.реализует_ли(тип Именуемый)
    вывести типЖивотного.реализует_ли(тип Звучащий)
}
`)
	want := "Животное\nПусто\nистина\nложь\n2\nистина\nистина\nложь\n"
	if got != want {
		t.Fatalf("получили %q, хотели %q", got, want)
	}
}

// TestRunEntityMethodAndFieldTagReflection - docs/ROADMAP.md, паритет с
// C#-атрибутами на классе/методе/поле: "тег(ключ)"/"тег_текст()" на самой
// сущности (новый синтаксис "сущность Имя `ключ:значение`"), "тег_метода(i,
// ключ)"/"тег_метода_текст(i)" на методе (тег метода уже существовал
// синтаксически - "противоположность"/"деепричастие" - но не был виден
// рефлексии), "тег_поля_текст(i)" - сырой текст тега поля в дополнение к
// уже существующему "тег_поля(i, ключ)". У примитива - "" everywhere (тот
// же путь, что и число_полей/число_методов - см. её же ensure() в
// cbackend, найденный там баг с NULL вместо "").
func TestRunEntityMethodAndFieldTagReflection(t *testing.T) {
	got := runSrc(t, `
сущность Пользователь `+"`"+`таблица:users, версия:2`+"`"+`
{
    имеет:
        имя: Строка `+"`"+`json:name, sql:user_name`+"`"+`

    сохранить(): Строка `+"`"+`выполняет:запись`+"`"+`
    {
        вернуть "сохранён"
    }
}

функция главная
{
    переменная т = тип Пользователь
    вывести т.тег("таблица")
    вывести т.тег("версия")
    вывести т.тег_текст()
    вывести т.тег_поля(0, "json")
    вывести т.тег_поля_текст(0)
    вывести т.тег_метода(0, "выполняет")
    вывести т.тег_метода_текст(0)

    переменная типЦелого = тип Целое
    вывести типЦелого.тег("что-то")
    вывести типЦелого.тег_текст()
}
`)
	want := "users\n2\nтаблица:users, версия:2\nname\njson:name, sql:user_name\nзапись\nвыполняет:запись\n\n\n"
	if got != want {
		t.Fatalf("получили %q, хотели %q", got, want)
	}
}

// TestRunFileReadPastEOFReturnsEmptyString - открыть/прочитать/закрыть
// (шаг 8, файловый ввод-вывод) через временный файл, не зависящий от
// examples/ (в отличие от TestRunAllExamplesMatchCompiledOutput выше) -
// повторное "прочитать" после конца файла должно возвращать "" каждый раз,
// не панику (в отличие от прочитать_строку/прочитать_целое/прочитать_число
// на конце стандартного ввода).
func TestRunFileReadPastEOFReturnsEmptyString(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "rech-file-*.txt")
	if err != nil {
		t.Fatalf("создать временный файл: %v", err)
	}
	if _, err := f.WriteString("ab"); err != nil {
		t.Fatalf("записать во временный файл: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("закрыть временный файл: %v", err)
	}
	path := strings.ReplaceAll(f.Name(), `\`, `\\`)
	got := runSrc(t, `
функция главная
{
    переменная ф = открыть("`+path+`")
    вывести прочитать(ф, 1)
    вывести прочитать(ф, 10)
    вывести прочитать(ф, 10)
    закрыть(ф)
}
`)
	if got != "a\nb\n\n" {
		t.Fatalf("получили %q, хотели \"a\\nb\\n\\n\"", got)
	}
}

// TestRunFileWriteAppendAndForEachLine - открыть_для_записи (обрезает),
// открыть_для_дозаписи (дописывает в конец) и "для <строка> в <Файл>"
// (потоковое построчное чтение, шаг 8) через один и тот же временный файл.
func TestRunFileWriteAppendAndForEachLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.txt")
	pathLit := strings.ReplaceAll(path, `\`, `\\`)
	got := runSrc(t, `
функция главная
{
    переменная запись = открыть_для_записи("`+pathLit+`")
    записать(запись, "первая\n")
    закрыть(запись)

    переменная дозапись = открыть_для_дозаписи("`+pathLit+`")
    записать(дозапись, "вторая\n")
    закрыть(дозапись)

    переменная файл = открыть("`+pathLit+`")
    для строка в файле
    {
        вывести строка
    }
    закрыть(файл)
}
`)
	if got != "первая\nвторая\n" {
		t.Fatalf("получили %q, хотели \"первая\\nвторая\\n\"", got)
	}
}

func TestRunWholeFileBuiltins(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.txt")
	pathLit := strings.ReplaceAll(path, `\`, `\\`)
	got := runSrc(t, `
функция главная
{
    записать_в_файл("`+pathLit+`", "первая")
    дописать_в_файл("`+pathLit+`", "\nвторая")
    вывести прочитать_файл("`+pathLit+`")
}
`)
	if got != "первая\nвторая\n" {
		t.Fatalf("получили %q, хотели \"первая\\nвторая\\n\"", got)
	}
}

// TestRunSocketTCPServerRoundTrip - сетевой_узел("tcp", "", порт)/принять/
// получить/отправить/закрыть_сокет через настоящий loopback-сокет ОС:
// серверная сторона - Rech-программа через интерпретатор (в отдельной
// goroutine, т.к. "принять" блокирует до входящего соединения - никакой
// конкурентности в самом Rech, синхронизация целиком на уровне настоящего
// сокета ОС, не языка), клиентская - обычный net.Dial (проверяет протокол
// снаружи, не только то, что интерпретатор сам с собой согласен). Ретраи
// подключения - "сетевой_узел" может ещё не успеть отработать в goroutine
// к моменту первой попытки net.Dial (обычная гонка старта сервера, не
// специфична для Rech).
func TestRunSocketTCPServerRoundTrip(t *testing.T) {
	port := 18099
	src := fmt.Sprintf(`
функция главная
{
    переменная слушатель = сетевой_узел("tcp", "", %d)
    переменная соединение = принять(слушатель)
    если соединение != Ничто
    {
        переменная сообщение = получить(соединение, 1024)
        отправить(соединение, "эхо: " + сообщение)
        закрыть_сокет(соединение)
    }
    закрыть_сокет(слушатель)
}
`, port)
	errCh := make(chan error, 1)
	go func() {
		toks, err := lexer.New(src).Tokenize()
		if err != nil {
			errCh <- err
			return
		}
		prog, err := parser.New(toks).ParseProgram()
		if err != nil {
			errCh <- err
			return
		}
		irProg, err := semantic.Analyze(prog)
		if err != nil {
			errCh <- err
			return
		}
		errCh <- Run(irProg, &bytes.Buffer{}, strings.NewReader(""))
	}()

	var conn net.Conn
	var err error
	for range 100 {
		conn, err = net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("не удалось подключиться к серверу: %v", err)
	}
	defer conn.Close()

	if _, err := conn.Write([]byte("привет")); err != nil {
		t.Fatalf("write: %v", err)
	}
	buf := make([]byte, 1024)
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got, want := string(buf[:n]), "эхо: привет"; got != want {
		t.Fatalf("получено %q, хотели %q", got, want)
	}

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("сервер завершился с ошибкой: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("сервер не завершился за отведённое время")
	}
}

// TestRunUDPServerRoundTrip - "сетевой_узел("udp", "", порт)"/получить_от/
// отправить_в через настоящий loopback-сокет ОС - симметрично
// TestRunSocketTCPServerRoundTrip выше, но для полноценного UDP-сервера
// (recvfrom/sendto с адресом ПРОИЗВОЛЬНОГО отправителя, не connect() на
// фиксированный адресат, как у udp-клиента).
func TestRunUDPServerRoundTrip(t *testing.T) {
	port := 18098
	src := fmt.Sprintf(`
функция главная
{
    переменная узел = сетевой_узел("udp", "", %d)
    переменная д = получить_от(узел, 1024)
    отправить_в(узел, "эхо: " + д.данные, д.адрес, д.порт)
    закрыть_сокет(узел)
}
`, port)
	errCh := make(chan error, 1)
	go func() {
		toks, err := lexer.New(src).Tokenize()
		if err != nil {
			errCh <- err
			return
		}
		prog, err := parser.New(toks).ParseProgram()
		if err != nil {
			errCh <- err
			return
		}
		irProg, err := semantic.Analyze(prog)
		if err != nil {
			errCh <- err
			return
		}
		errCh <- Run(irProg, &bytes.Buffer{}, strings.NewReader(""))
	}()

	// В отличие от TCP выше, "net.Dial(\"udp\", ...)" не проверяет
	// достижимость (UDP без соединения - Dial только создаёт локальный
	// сокет, ассоциированный с адресом) - ретраить нужно сам обмен
	// (запись+чтение), не сам Dial: пока сервер (goroutine) ещё не успел
	// добежать до "сетевой_узел", loopback отвечает ICMP port-unreachable,
	// всплывающим как ошибка на Read (connection refused), не на Write.
	conn, err := net.Dial("udp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatalf("net.Dial: %v", err)
	}
	defer conn.Close()

	var n int
	buf := make([]byte, 1024)
	for range 100 {
		if _, err = conn.Write([]byte("привет")); err != nil {
			t.Fatalf("write: %v", err)
		}
		conn.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
		n, err = conn.Read(buf)
		if err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got, want := string(buf[:n]), "эхо: привет"; got != want {
		t.Fatalf("получено %q, хотели %q", got, want)
	}

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("сервер завершился с ошибкой: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("сервер не завершился за отведённое время")
	}
}

// TestRunSocketClientConnects - "сетевой_узел" с непустым хостом (tcp и
// udp) с клиентской стороны (сервер - обычный Go net, не Rech) - дополняет
// TestRunSocketTCPServerRoundTrip выше (там Rech - сервер, Go - клиент;
// здесь наоборот), чтобы обе клиентские ветки были проверены реальным
// сокетом, а не только серверная (host == "").
func TestRunSocketClientConnects(t *testing.T) {
	tcpLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	defer tcpLn.Close()
	tcpPort := tcpLn.Addr().(*net.TCPAddr).Port
	go func() {
		c, err := tcpLn.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		buf := make([]byte, 64)
		n, _ := c.Read(buf)
		c.Write([]byte("tcp-ok:" + string(buf[:n])))
	}()

	udpConn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.ListenPacket: %v", err)
	}
	defer udpConn.Close()
	udpPort := udpConn.LocalAddr().(*net.UDPAddr).Port
	go func() {
		buf := make([]byte, 64)
		n, addr, err := udpConn.ReadFrom(buf)
		if err != nil {
			return
		}
		udpConn.WriteTo([]byte("udp-ok:"+string(buf[:n])), addr)
	}()

	got := runSrc(t, fmt.Sprintf(`
функция главная
{
    переменная tcp = сетевой_узел("tcp", "127.0.0.1", %d)
    отправить(tcp, "а")
    вывести получить(tcp, 64)
    закрыть_сокет(tcp)

    переменная udp = сетевой_узел("udp", "127.0.0.1", %d)
    отправить(udp, "б")
    вывести получить(udp, 64)
    закрыть_сокет(udp)
}
`, tcpPort, udpPort))
	if got != "tcp-ok:а\nudp-ok:б\n" {
		t.Fatalf("получено %q", got)
	}
}

// TestRunHTTPRequestBuiltin - "запрос" (шаг 9, HTTP-клиент типа curl) через
// настоящий TCP-сокет: сервер - обычный net.Listener, отвечающий
// вручную-собранным HTTP-ответом (не net/http.Server - нужен полный
// контроль над тем, что именно отправляется по проводу, включая
// нестандартный статус-код и заголовок, чтобы проверить разбор построчно).
func TestRunHTTPRequestBuiltin(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		buf := make([]byte, 4096)
		c.Read(buf)
		c.Write([]byte("HTTP/1.1 201 Created\r\nX-Test: yes\r\n\r\nсозданный ресурс"))
	}()

	got := runSrc(t, fmt.Sprintf(`
функция главная
{
    переменная р = новый Запрос("POST", "127.0.0.1", %d, "/create", "X-Custom: 1", "данные")
    переменная о = запрос(р)
    вывести о.код
    вывести о.тело
}
`, port))
	if got != "201\nсозданный ресурс\n" {
		t.Fatalf("получено %q", got)
	}
}

func TestRunContainsBuiltin(t *testing.T) {
	got := runSrc(t, `
функция главная
{
    вывести содержит("привет мир", "мир")
    вывести содержит("привет мир", "нет")
}
`)
	if got != "истина\nложь\n" {
		t.Fatalf("получили %q, хотели \"истина\\nложь\\n\"", got)
	}
}

// TestRunArgsBuiltinsHaveNoProgramArgs - "rech test" не передаёт
// интерпретируемой программе аргументов - "количество_аргументов" всегда
// 0, "аргумент" любого индекса паникует диапазоном.
func TestRunArgsBuiltinsHaveNoProgramArgs(t *testing.T) {
	got := runSrc(t, `
функция главная
{
    вывести количество_аргументов()
}
`)
	if got != "0\n" {
		t.Fatalf("получили %q, хотели \"0\\n\"", got)
	}

	toks, err := lexer.New(`
функция главная
{
    вывести аргумент(0)
}
`).Tokenize()
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
	var buf bytes.Buffer
	err = Run(irProg, &buf, strings.NewReader(""))
	if err == nil || !strings.Contains(err.Error(), "аргумент: индекс 0 вне диапазона") {
		t.Fatalf("ожидалась паника про индекс вне диапазона, получено: %v", err)
	}
}

func TestRunEntityFieldMutationVisibleAfterMethodReturn(t *testing.T) {
	got := runSrc(t, `
сущность Счётчик
{
    имеет:
        значение: Целое

    конструктор(значение: Целое)
    {
        этот.значение = значение
    }

    увеличить()
    {
        этот.значение = этот.значение + 1
    }
}

функция главная
{
    переменная c = новый Счётчик(0)
    c.увеличить()
    c.увеличить()
    вывести c.значение
}
`)
	if got != "2\n" {
		t.Fatalf("получили %q, хотели \"2\\n\"", got)
	}
}

// TestRunEntityFieldsWithoutDefaultsGetSafeZeroValues - сущность/класс без
// явного "конструктор(...)" (или с ним, но без "этот.поле = ..." для
// какого-то поля) должна получать БЕЗОПАСНОЕ нулевое значение для КАЖДОГО
// поля, не только для примитивов (Целое/Логическое) - Строка без "=
// выражение" должна стать "", generic-коллекция (Список<T> и т. п.) -
// настоящим пустым экземпляром, а не оставаться nil-плейсхолдером (живая
// проверка нашла именно это: "invalid memory address or nil pointer
// dereference" при первом же обращении к такому полю - internal/interp/
// eval.go, zeroFieldValue, до фикса использовавший голый zeroValue).
func TestRunEntityFieldsWithoutDefaultsGetSafeZeroValues(t *testing.T) {
	got := runSrc(t, `
сущность Модель
{
    имеет:
        имя: Строка
        числа: Список<Целое>
}

функция главная
{
    переменная м = новая Модель()
    вывести м.имя == ""
    м.числа.добавить(5)
    вывести м.числа.получить(0)
}
`)
	if got != "истина\n5\n" {
		t.Fatalf("получили %q, хотели \"истина\\n5\\n\"", got)
	}
}

func TestRunStructValueSemanticsCopyOnAssign(t *testing.T) {
	got := runSrc(t, `
структура Точка
{
    имеет:
        x: Целое
}

функция главная
{
    переменная a = новая Точка(1)
    переменная b = a
    b.x = 99
    вывести a.x
    вывести b.x
}
`)
	if got != "1\n99\n" {
		t.Fatalf("структуры должны копироваться при присваивании (значение, не ссылка), получили %q", got)
	}
}

func TestRunEntityReferenceSemanticsSharedOnAssign(t *testing.T) {
	got := runSrc(t, `
сущность Точка
{
    имеет:
        x: Целое

    конструктор(x: Целое)
    {
        этот.x = x
    }
}

функция главная
{
    переменная a = новая Точка(1)
    переменная b = a
    b.x = 99
    вывести a.x
    вывести b.x
}
`)
	if got != "99\n99\n" {
		t.Fatalf("сущности должны разделять один и тот же объект при присваивании (ссылка, не копия), получили %q", got)
	}
}

// TestRunCollectionOutOfRangePanics проверяет, что ошибка времени
// выполнения (docs/SPEC.md, раздел 8) возвращается из Run как обычная
// error (runtimeError, см. interp.go) - в отличие от rech_panic в
// сгенерированном C (там процесс завершается сам, exit(1)), интерпретатор
// не решает сам, что делать с ошибкой выполнения программы Rech - это
// нужно "rech test", чтобы продолжить проверку остальных тестов после
// одного неудачного, а не завершить весь процесс.
func TestRunIntDivisionByZeroReturnsErrorNotCrash(t *testing.T) {
	toks, err := lexer.New(`
функция главная
{
    вывести "до"
    переменная а = 1 / 0
    вывести а
}
`).Tokenize()
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
	var buf bytes.Buffer
	err = Run(irProg, &buf, strings.NewReader(""))
	if err == nil || !strings.Contains(err.Error(), "деление на ноль") {
		t.Fatalf("ожидалась ошибка про деление на ноль, получено: %v", err)
	}
	if buf.String() != "до\n" {
		t.Fatalf("получили вывод %q", buf.String())
	}
}

func TestRunModuloComputesRemainder(t *testing.T) {
	got := runSrc(t, `
функция главная
{
    вывести 17 % 5
    вывести -7 % 3
}
`)
	if got != "2\n-1\n" {
		t.Fatalf("получили %q", got)
	}
}

func TestRunFloatModuloComputesRemainder(t *testing.T) {
	got := runSrc(t, `
функция главная
{
    вывести 7.5 % 2.0
    вывести -7.5 % 2.0
}
`)
	if got != "1.5\n-1.5\n" {
		t.Fatalf("получили %q", got)
	}
}

func TestRunIntModuloByZeroReturnsErrorNotCrash(t *testing.T) {
	toks, err := lexer.New(`
функция главная
{
    вывести "до"
    переменная а = 1 % 0
    вывести а
}
`).Tokenize()
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
	var buf bytes.Buffer
	err = Run(irProg, &buf, strings.NewReader(""))
	if err == nil || !strings.Contains(err.Error(), "деление на ноль") {
		t.Fatalf("ожидалась ошибка про деление на ноль, получено: %v", err)
	}
	if buf.String() != "до\n" {
		t.Fatalf("получили вывод %q", buf.String())
	}
}

func TestRunNullableCastOnNichtoPanics(t *testing.T) {
	toks, err := lexer.New(`
функция главная
{
    вывести "до"
    переменная а: ?Целое = Ничто
    вывести (Целое) а
}
`).Tokenize()
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
	var buf bytes.Buffer
	err = Run(irProg, &buf, strings.NewReader(""))
	if err == nil || !strings.Contains(err.Error(), "обращение к пустому значению") {
		t.Fatalf("ожидалась ошибка про обращение к пустому значению (Ничто), получено: %v", err)
	}
	if buf.String() != "до\n" {
		t.Fatalf("получили вывод %q", buf.String())
	}
}

func TestRunFloatDivisionByZeroDoesNotPanic(t *testing.T) {
	got := runSrc(t, `
функция главная
{
    переменная а = 1.0 / 0.0
    вывести а
}
`)
	if got != "inf\n" {
		t.Fatalf("получили %q", got)
	}
}

func TestRunCollectionOutOfRangePanics(t *testing.T) {
	toks, err := lexer.New(`
функция главная
{
    переменная числа = новый Список<Целое>()
    вывести числа.получить(0)
}
`).Tokenize()
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
	var buf bytes.Buffer
	err = Run(irProg, &buf, strings.NewReader(""))
	if err == nil {
		t.Fatal("ожидалась ошибка времени выполнения (индекс вне границ)")
	}
	if !strings.Contains(err.Error(), "индекс вне границ") {
		t.Fatalf("ожидалось сообщение про индекс вне границ, получили: %v", err)
	}
}

func TestRunAssertPasses(t *testing.T) {
	got := runSrc(t, `
функция главная
{
    переменная а = 2
    утверждать а + 3 == 5
    вывести "ок"
}
`)
	if got != "ок\n" {
		t.Fatalf("получили %q", got)
	}
}

func TestRunAssertFailsWithLineNumber(t *testing.T) {
	toks, err := lexer.New(`
функция главная
{
    вывести "до"
    утверждать 1 == 2
    вывести "после"
}
`).Tokenize()
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
	var buf bytes.Buffer
	err = Run(irProg, &buf, strings.NewReader(""))
	if err == nil {
		t.Fatal("ожидалась ошибка - утверждение ложно")
	}
	if !strings.Contains(err.Error(), "строка 5") {
		t.Fatalf("ожидался номер строки в сообщении, получили: %v", err)
	}
	if buf.String() != "до\n" {
		t.Fatalf("после неудачного \"утверждать\" выполнение должно остановиться - получили вывод %q", buf.String())
	}
}

func TestRunMutualRecursion(t *testing.T) {
	got := runSrc(t, `
функция чётное(n: Целое): Логическое
{
    если n == 0
    {
        вернуть истина
    }
    вернуть нечётное(n - 1)
}

функция нечётное(n: Целое): Логическое
{
    если n == 0
    {
        вернуть ложь
    }
    вернуть чётное(n - 1)
}

функция главная
{
    вывести чётное(10)
    вывести нечётное(10)
}
`)
	if got != "истина\nложь\n" {
		t.Fatalf("получили %q", got)
	}
}

func TestRunBitwiseOperators(t *testing.T) {
	got := runSrc(t, `
функция главная
{
    вывести 6 | 3
    вывести 6 & 3
    вывести 6 ^ 3
    вывести ~0
    вывести 1 << 3
    вывести 128 >> 4
}
`)
	if got != "7\n2\n5\n-1\n8\n8\n" {
		t.Fatalf("получили %q", got)
	}
}

func TestRunIncDecExprPrefixAndPostfixValues(t *testing.T) {
	got := runSrc(t, `
функция главная
{
    переменная а = 5
    вывести а++
    вывести а
    вывести ++а
    вывести а
}
`)
	if got != "5\n6\n7\n7\n" {
		t.Fatalf("получили %q", got)
	}
}

func TestRunIncDecOnFloatVariable(t *testing.T) {
	got := runSrc(t, `
функция главная
{
    переменная а = 1.5
    вывести а++
    вывести а
}
`)
	if got != "1.5\n2.5\n" {
		t.Fatalf("получили %q", got)
	}
}

func TestRunAndOrSymbolSynonyms(t *testing.T) {
	got := runSrc(t, `
функция главная
{
    вывести истина && ложь
    вывести истина || ложь
}
`)
	if got != "ложь\nистина\n" {
		t.Fatalf("получили %q", got)
	}
}

func runSrcWithInput(t *testing.T, src, input string) string {
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
	var buf bytes.Buffer
	if err := Run(irProg, &buf, strings.NewReader(input)); err != nil {
		t.Fatalf("исполнение: %v", err)
	}
	return buf.String()
}

func TestRunMathBuiltins(t *testing.T) {
	got := runSrc(t, `
функция главная
{
    вывести корень(16.0)
    вывести степень(2.0, 10.0)
    вывести округлить(3.6)
    вывести потолок(3.1)
    вывести пол(3.9)
    вывести модуль(-5.5)
    вывести экспонента(0.0)
    вывести синус(0.0)
    вывести косинус(0.0)
    вывести тангенс(0.0)
    вывести арксинус(0.0)
    вывести арккосинус(1.0)
    вывести арктангенс(0.0)
    вывести арктангенс2(0.0, 1.0)
    вывести логарифм(1.0)
    вывести логарифм2(8.0)
    вывести логарифм10(1000.0)
    вывести минимум(3.0, 7.0)
    вывести максимум(3.0, 7.0)
    вывести пи()
}
`)
	want := "4\n1024\n4\n4\n3\n5.5\n1\n0\n1\n0\n0\n0\n0\n0\n0\n3\n3\n3\n7\n3.14159\n"
	if got != want {
		t.Fatalf("получили %q, ожидалось %q", got, want)
	}
}

func TestRunConsoleInputBuiltins(t *testing.T) {
	got := runSrcWithInput(t, `
функция главная
{
    переменная имя = прочитать_строку()
    переменная возраст = прочитать_целое()
    переменная рост = прочитать_число()
    вывести имя
    вывести возраст
    вывести рост
}
`, "Аня\n30\n1.75\n")
	want := "Аня\n30\n1.75\n"
	if got != want {
		t.Fatalf("получили %q, ожидалось %q", got, want)
	}
}

func TestRunConsoleInputInvalidIntFormatPanics(t *testing.T) {
	toks, err := lexer.New(`
функция главная
{
    переменная х = прочитать_целое()
}
`).Tokenize()
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
	var buf bytes.Buffer
	err = Run(irProg, &buf, strings.NewReader("не число"))
	if err == nil || !strings.Contains(err.Error(), "не является целым числом") {
		t.Fatalf("ожидалась ошибка формата, получено: %v", err)
	}
}

func TestRunConsoleInputEOFPanics(t *testing.T) {
	toks, err := lexer.New(`
функция главная
{
    переменная х = прочитать_строку()
}
`).Tokenize()
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
	var buf bytes.Buffer
	err = Run(irProg, &buf, strings.NewReader(""))
	if err == nil || !strings.Contains(err.Error(), "конец ввода") {
		t.Fatalf("ожидалась ошибка конца ввода, получено: %v", err)
	}
}

// TestRunRejectsExternFunc - Run сам отказывается исполнять программу с
// внешними функциями (semantic.Analyze их производит в IR как обычно, см.
// TestAnalyzeExternFuncBuildsIR в internal/semantic - отказ ради того, что
// у интерпретатора нет способа вызвать произвольный C-символ без настоящей
// динамической линковки, находится здесь, а не в Analyze).
func TestRunRejectsExternFunc(t *testing.T) {
	toks, err := lexer.New(`
внешняя функция abs(n: Целое): Целое
функция главная { вывести abs(0 - 5) }
`).Tokenize()
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
	var buf bytes.Buffer
	err = Run(irProg, &buf, strings.NewReader(""))
	if err == nil || !strings.Contains(err.Error(), "не поддерживаются интерпретатором") {
		t.Fatalf("ожидалась ошибка про неподдержку внешних функций интерпретатором, получено: %v", err)
	}
}

func TestRunShowStmtPrimitives(t *testing.T) {
	got := runSrc(t, `
функция главная
{
	показать 5
	показать 3.5
	показать "текст"
	показать истина
	показать ложь
}
`)
	want := "5 (Целое)\n3.5 (Действительное)\n\"текст\" (Строка)\nистина (Логическое)\nложь (Логическое)\n"
	if got != want {
		t.Fatalf("получили %q, хотели %q", got, want)
	}
}

func TestRunShowStmtEntityRecursesIntoFields(t *testing.T) {
	got := runSrc(t, `
сущность Точка
{
	имеет:
		x: Целое
		y: Действительное

	конструктор(x: Целое, y: Действительное)
	{
		этот.x = x
		этот.y = y
	}
}

функция главная
{
	переменная p = новая Точка(1, 2.5)
	показать p
}
`)
	want := "Точка { x: 1 (Целое), y: 2.5 (Действительное) }\n"
	if got != want {
		t.Fatalf("получили %q, хотели %q", got, want)
	}
}

func TestRunShowStmtNestedEntity(t *testing.T) {
	got := runSrc(t, `
сущность Точка
{
	имеет:
		x: Целое
		y: Целое

	конструктор(x: Целое, y: Целое)
	{
		этот.x = x
		этот.y = y
	}
}
сущность Отрезок
{
	имеет:
		начало: Точка
		конец: Точка

	конструктор(начало: Точка, конец: Точка)
	{
		этот.начало = начало
		этот.конец = конец
	}
}

функция главная
{
	переменная о = новый Отрезок(новая Точка(0, 0), новая Точка(1, 1))
	показать о
}
`)
	want := "Отрезок { начало: Точка { x: 0 (Целое), y: 0 (Целое) }, конец: Точка { x: 1 (Целое), y: 1 (Целое) } }\n"
	if got != want {
		t.Fatalf("получили %q, хотели %q", got, want)
	}
}

func TestRunShowStmtList(t *testing.T) {
	got := runSrc(t, `
функция главная
{
	переменная список = новый Список<Целое>()
	список.добавить(10)
	список.добавить(20)
	показать список
}
`)
	want := "Список<Целое> [10 (Целое), 20 (Целое)]\n"
	if got != want {
		t.Fatalf("получили %q, хотели %q", got, want)
	}
}

// TestRunPrintOnEntityShowsFriendlyPlaceholder - "вывести" на сущности не
// ошибка компиляции (дружелюбнее для новичков) и не печатает внутреннее
// представление значения - только маркер вида "[сущность]<Точка>".
// Подробности значения - через "показать" (см. TestRunShowStmt*).
func TestRunPrintOnEntityShowsFriendlyPlaceholder(t *testing.T) {
	got := runSrc(t, `
сущность Точка
{
	имеет:
		x: Целое

	конструктор(x: Целое)
	{
		этот.x = x
	}
}
функция главная
{
	переменная p = новая Точка(1)
	вывести p
}
`)
	if got != "[сущность]<Точка>\n" {
		t.Fatalf("получили %q, хотели \"[сущность]<Точка>\\n\"", got)
	}
}
