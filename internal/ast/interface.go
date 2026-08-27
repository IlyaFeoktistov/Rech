package ast

// InterfaceDecl соответствует:
//
//	интерфейс <Имя>
//	{
//	    <метод1>(<парам>: <Тип>, ...): <ТипРезультата>
//	    <метод2>(...)
//	    ...
//	}
//
// Интерфейс - контракт на набор методов без реализации: только
// сигнатуры (имя/параметры/тип результата), без тел, без полей, без
// "имеет:" - сущность (не "структура", см. EntityDecl.IsStruct)
// "реализует" интерфейс через одноимённое слово, проверка соответствия -
// на этапе семантического анализа (internal/semantic).
type InterfaceDecl struct {
	Name    string
	Methods []InterfaceMethod
	Line    int
}

func (*InterfaceDecl) node() {}

// InterfaceMethod - одна сигнатура метода в теле "интерфейс" - та же форма,
// что и у MethodDecl.Params/ReturnType, но без Body/Private (интерфейс не
// содержит реализации и не различает видимость методов - соответствие
// интерфейсу либо есть, либо нет, целиком).
type InterfaceMethod struct {
	Name       string
	Params     []Param
	ReturnType string
	// SelfReturn - см. MethodDecl.SelfReturn: у метода интерфейса означает
	// "результат - конкретный тип реализующей сущности/тип получателя в
	// месте вызова", не сам интерфейс дословно.
	SelfReturn bool
	Line       int
}
