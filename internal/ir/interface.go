package ir

// Interface - интерфейс программы после семантического анализа - контракт
// на набор методов без реализации, которому может "реализует"
// соответствовать сущность (не "структура" - см. Entity.IsStruct). Methods
// в порядке объявления - тот же порядок, что участвует в назначении
// глобальных слотов vtable (cbackend, детерминированность нужна только для
// стабильности одной сборки, не между сборками).
type Interface struct {
	Name    string
	Methods []InterfaceMethod
}

// InterfaceMethod - одна сигнатура метода в контракте интерфейса - та же
// форма, что и у Method.Params/ReturnType/HasReturn, но без Body (у
// интерфейса нет реализации).
type InterfaceMethod struct {
	Name       string
	Params     []Param
	ReturnType Type
	HasReturn  bool
}
