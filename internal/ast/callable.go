package ast

// FuncRefExpr - "&имя" - ссылка на уже объявленную функцию как на значение
// типа "Вызываемый", без вызова.
type FuncRefExpr struct {
	Name string
	Line int
}

func (*FuncRefExpr) node() {}
func (*FuncRefExpr) expr() {}

// LambdaExpr - функция без имени в позиции выражения:
//
//	функция(<парам>: <Тип>, ...): <ТипРезультата> { <тело> }
//
// Та же форма параметров/тела, что и у именованной FunctionDecl (Params,
// ReturnType, Body), но без Name и без TypeParams - лямбда не может быть
// generic. Свободные переменные тела (замыкание) находит семантический
// анализ, не парсер - здесь только синтаксическая форма.
type LambdaExpr struct {
	Params     []Param
	ReturnType string
	Body       []Stmt
	Line       int
}

func (*LambdaExpr) node() {}
func (*LambdaExpr) expr() {}
