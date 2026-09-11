package ast

// OperatorMethodPrefix - префикс синтетического имени метода, которое
// парсер генерирует для перегрузки бинарного оператора ("статический
// публичный оператор +(...)", internal/parser/entity.go, parseMethod) -
// MethodDecl.Name становится OperatorMethodPrefix+"+" и т. д. Дальше по
// всему компилятору (сбор сигнатур, разрешение "a + b" в
// internal/semantic/operator.go, оба backend'а через уже существующий
// ir.StaticMethodCallExpr) такой метод неотличим от обычного статического
// метода со "странным" именем - обычный IDENT никогда не содержит символов
// операторов, коллизия с настоящим объявленным именем невозможна.
const OperatorMethodPrefix = "оператор"

// OperatorMethodName строит синтетическое имя метода для перегрузки
// оператора symbol ("+", "==", "<=", ...).
func OperatorMethodName(symbol string) string {
	return OperatorMethodPrefix + symbol
}
