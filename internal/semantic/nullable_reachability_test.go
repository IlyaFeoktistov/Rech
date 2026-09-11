package semantic

import "testing"

// TestNullablesOmitSocketWithoutNetworkCall - НАЙДЕННЫЙ И ИСПРАВЛЕННЫЙ
// структурный риск ("Архитектура компилятора -
// структурные риски"): раньше "?Сокет" попадал в prog.Nullables БЕЗУСЛОВНО
// для ЛЮБОЙ программы (registerNetworkFuncs регистрировал entitySig сразу
// при настройке анализатора) - программа без единого сетевого вызова не
// должна нести эту nullable-инстанциацию вовсе.
func TestNullablesOmitSocketWithoutNetworkCall(t *testing.T) {
	prog, err := AnalyzeSource(t, `
функция главная {
    вывести "привет"
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	for _, n := range prog.Nullables {
		if n.Name == "?Сокет" {
			t.Fatalf("\"?Сокет\" не должен попадать в Program.Nullables без единого сетевого вызова")
		}
	}
}

// TestNullablesIncludeSocketOnAcceptCall - "принять" - прямой производитель
// "?Сокет", должен зарегистрировать его лениво (registerSocketNullableOnCall).
func TestNullablesIncludeSocketOnAcceptCall(t *testing.T) {
	prog, err := AnalyzeSource(t, `
функция главная {
    переменная слушатель = сетевой_узел(СетевойПротокол.TCP, "", 18080)
    переменная соединение = принять(слушатель)
    закрыть_сокет(слушатель)
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	found := false
	for _, n := range prog.Nullables {
		if n.Name == "?Сокет" {
			found = true
		}
	}
	if !found {
		t.Fatalf("\"?Сокет\" должен попасть в Program.Nullables после вызова \"принять\", Nullables: %+v", prog.Nullables)
	}
}

// TestNullablesIncludeSocketOnClientCallWithoutAccept - НАЙДЕННЫЙ РЕГРЕСС
// первой версии этой правки, пойманный check_examples.sh (не гипотетически):
// TCP/UDP-клиент вызывает "сетевой_узел"/"отправить"/"получить", ни разу не
// вызывая "принять" - cbackend всё равно эмитирует "socketHelpers"
// (rech_accept - часть общего сетевого рантайма, включается при usesSockets
// независимо от того, какая ИМЕННО сетевая функция вызвана, см. package doc
// funcSigTouchesSocket, network.go), а rech_accept ссылается на обёртку
// "?Сокет" - значит она обязана быть в Program.Nullables и здесь тоже, не
// только при вызове "принять".
func TestNullablesIncludeSocketOnClientCallWithoutAccept(t *testing.T) {
	prog, err := AnalyzeSource(t, `
функция главная {
    переменная соединение = сетевой_узел(СетевойПротокол.TCP, "127.0.0.1", 18080)
    отправить(соединение, "привет")
    закрыть_сокет(соединение)
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	found := false
	for _, n := range prog.Nullables {
		if n.Name == "?Сокет" {
			found = true
		}
	}
	if !found {
		t.Fatalf("\"?Сокет\" должен попасть в Program.Nullables у клиента без \"принять\" (socketHelpers всё равно эмитируется), Nullables: %+v", prog.Nullables)
	}
}

// TestNullablesIncludeSocketOnHTTPRequestCall - "запрос" (HTTP-клиент) -
// единственное ИЗВЕСТНОЕ исключение из funcSigTouchesSocket (её сигнатура
// "Запрос -> Ответ" не упоминает "Сокет" вовсе, хотя rech_http_request
// внутри использует сокет) - обрабатывается отдельной явной проверкой имени
// в analyzeCallExpr.
func TestNullablesIncludeSocketOnHTTPRequestCall(t *testing.T) {
	prog, err := AnalyzeSource(t, `
функция главная {
    переменная запрос_ = новый Запрос("GET", "127.0.0.1", 80, "/", "", "")
    переменная ответ = запрос(запрос_)
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	found := false
	for _, n := range prog.Nullables {
		if n.Name == "?Сокет" {
			found = true
		}
	}
	if !found {
		t.Fatalf("\"?Сокет\" должен попасть в Program.Nullables при вызове \"запрос\", Nullables: %+v", prog.Nullables)
	}
}

// TestNullablesIncludeStringOnEnvCall - "получить_переменную_окружения"
// возвращает "?Строка" - тот же принцип ленивой регистрации, что и у
// "?Сокет" (registerOptionalStringOnCall, nullable.go), только сама "?Строка"
// не несёт риска (её C-представление, const char*, не нуждается в условно
// эмитируемом typedef) - проверяется здесь то же самое поведение, не
// отдельная опасность.
func TestNullablesIncludeStringOnEnvCall(t *testing.T) {
	prog, err := AnalyzeSource(t, `
функция главная {
    переменная порт = получить_переменную_окружения("ПОРТ")
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	foundString, foundSocket := false, false
	for _, n := range prog.Nullables {
		if n.Name == "?Строка" {
			foundString = true
		}
		if n.Name == "?Сокет" {
			foundSocket = true
		}
	}
	if !foundString {
		t.Fatalf("\"?Строка\" должен попасть в Program.Nullables после вызова \"получить_переменную_окружения\", Nullables: %+v", prog.Nullables)
	}
	if foundSocket {
		t.Fatalf("\"?Сокет\" не должен попадать в Program.Nullables у программы без единого сетевого вызова, Nullables: %+v", prog.Nullables)
	}
}
