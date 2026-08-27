package interp

import "github.com/IlyaFeoktistov/Rech/internal/ir"

// findMethod ищет метод name у сущности typeName - сначала среди её
// собственных методов (ent.Methods, только новые/переопределённые, см.
// package doc ir.Entity.Methods), затем, если не нашёлся, вверх по цепочке
// BaseName ("наследует") - неизменённый унаследованный метод существует
// только на определяющей его сущности, не дублируется на потомков.
//
// Диспетчеризация уже полиморфна сама по себе, без какого-либо отдельного
// механизма: получатель резолвится в *entityObj с РЕАЛЬНЫМ типом объекта
// (recv.Obj.typeName), не со статическим типом выражения из IR (в отличие
// от cbackend, где статический тип решает вызов на этапе компиляции) -
// значит переопределённый метод уже находится по фактическому типу, даже
// если вызов написан через переменную базового/интерфейсного типа, вообще
// без vtable - тут просто нечего диспетчеризовать дополнительно.
func (in *Interp) findMethod(typeName, name string) *ir.Method {
	for typeName != "" {
		ent, ok := in.entities[typeName]
		if !ok {
			return nil
		}
		for _, m := range ent.Methods {
			if m.Name == name {
				return m
			}
		}
		typeName = ent.BaseName
	}
	return nil
}
