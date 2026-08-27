// Интерфейсы - контракт на набор методов без реализации, которому сущность
// (не "структура" - см. entitySig.IsStruct) может "реализует" соответствовать.
// Наследование ("наследует") - в internal/semantic/inheritance.go, отдельно:
// два разных отношения между типами, у каждого свой файл.
package semantic

import (
	"fmt"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// interfaceSig - сигнатура интерфейса: имена методов и их funcSig (только
// Params/ReturnType/HasReturn несут смысл - Private/ParamStates/
// GuaranteedThisStates нулевые, у интерфейса нет ни видимости, ни typestate-
// параметров, ни тела метода, из которого их можно было бы вывести).
type interfaceSig struct {
	MethodNames []string
	MethodSigs  []funcSig
}

func (s interfaceSig) methodIndex(name string) int {
	for i, n := range s.MethodNames {
		if n == name {
			return i
		}
	}
	return -1
}

// collectInterfaces - предварительный проход: сигнатуры методов всех
// интерфейсов программы, до collectEntities ("реализует" сущности должен
// резолвиться против уже готовых interfaceSig) - тот же принцип, что и
// collectFuncSigs/collectEntities. Известное ограничение: параметры/тип
// результата метода интерфейса могут быть только примитивами
// или именами УЖЕ собранных интерфейсов (не сущностей - у collectEntities
// собственная, более сложная граница видимости, интерфейсы сознательно её
// не используют, чтобы не заводить двухпроходное разрешение вперёд/назад
// между сущностями и интерфейсами).
func (a *Analyzer) collectInterfaces(prog *ast.Program) error {
	interfaces := map[string]interfaceSig{}
	for _, iface := range prog.Interfaces {
		if _, exists := interfaces[iface.Name]; exists {
			return fmt.Errorf("интерфейс %q объявлен более одного раза", iface.Name)
		}
		var sig interfaceSig
		for _, m := range iface.Methods {
			if sig.methodIndex(m.Name) != -1 {
				return fmt.Errorf("интерфейс %q: метод %q объявлен более одного раза", iface.Name, m.Name)
			}
			params := make([]ir.Type, len(m.Params))
			for i, p := range m.Params {
				typ, err := resolveTypeName(nil, nil, nil, interfaces, p.Type)
				if err != nil {
					return fmt.Errorf("интерфейс %q: метод %q: параметр %q: %w", iface.Name, m.Name, p.Name, err)
				}
				params[i] = typ
			}
			msig := funcSig{Params: params}
			if m.ReturnType != "" {
				typ, err := resolveTypeName(nil, nil, nil, interfaces, m.ReturnType)
				if err != nil {
					return fmt.Errorf("интерфейс %q: метод %q: тип результата: %w", iface.Name, m.Name, err)
				}
				msig.ReturnType = typ
				msig.HasReturn = true
			}
			sig.MethodNames = append(sig.MethodNames, m.Name)
			sig.MethodSigs = append(sig.MethodSigs, msig)
		}
		interfaces[iface.Name] = sig
	}
	a.interfaces = interfaces
	return nil
}

// checkImplements - проверяет, что сущность ent (уже полностью собранная -
// flatMethods несёт её собственные и унаследованные методы, см.
// collectEntities) реализует интерфейс ifaceName целиком: каждый метод
// контракта должен присутствовать у сущности с точно совпадающей
// сигнатурой (число/типы параметров, тип результата) - без ковариантности
// и без частичной реализации, тот же принцип строгости, что и у остальных
// проверок типов языка ("без неявных преобразований, ни здесь").
func checkImplements(interfaces map[string]interfaceSig, entityName string, flatMethods map[string]funcSig, ifaceName string, line int) error {
	iface, ok := interfaces[ifaceName]
	if !ok {
		return fmt.Errorf("строка %d: сущность %q: интерфейс %q не объявлен", line, entityName, ifaceName)
	}
	for i, name := range iface.MethodNames {
		want := iface.MethodSigs[i]
		got, ok := flatMethods[name]
		if !ok {
			return fmt.Errorf("строка %d: сущность %q не реализует метод %q интерфейса %q", line, entityName, name, ifaceName)
		}
		if !sigsMatch(want, got) {
			return fmt.Errorf("строка %d: сущность %q: метод %q не совпадает по сигнатуре с контрактом интерфейса %q", line, entityName, name, ifaceName)
		}
		// Вызов через интерфейсное значение не проверяет видимость (методы
		// интерфейса - по определению публичный контракт) - значит и
		// реализация обязана быть публичной, иначе приватный/защищённый
		// метод стал бы вызываемым откуда угодно в обход собственной
		// видимости, просто через интерфейсную переменную.
		if got.Visibility != ast.Public {
			return fmt.Errorf("строка %d: сущность %q: метод %q реализует интерфейс %q, но не публичный - методы интерфейса вызываются без проверки видимости, реализация обязана быть публичной", line, entityName, name, ifaceName)
		}
	}
	return nil
}

// sigsMatch - совпадают ли две сигнатуры по числу/типам параметров и типу
// результата (Private/ParamStates/GuaranteedThisStates не сравниваются -
// это свойства конкретного объявления, не часть контракта вызова).
func sigsMatch(a, b funcSig) bool {
	if a.HasReturn != b.HasReturn || (a.HasReturn && a.ReturnType != b.ReturnType) {
		return false
	}
	if len(a.Params) != len(b.Params) {
		return false
	}
	for i := range a.Params {
		if a.Params[i] != b.Params[i] {
			return false
		}
	}
	return true
}

// entitySatisfies - реализует ли entityName (напрямую или через цепочку
// "наследует") интерфейс ifaceName - обходит BaseName вверх (защита от
// цикла - циклы в BaseName и так невозможны, см. inheritance.go, базовая
// сущность обязана быть объявлена раньше в файле, но bound на число
// итераций всё равно ставится, а не полагается на это молча).
func entitySatisfies(entities map[string]entitySig, entityName, ifaceName string) bool {
	name := entityName
	for range entities {
		sig, ok := entities[name]
		if !ok {
			return false
		}
		for _, own := range sig.OwnInterfaces {
			if own == ifaceName {
				return true
			}
		}
		if sig.BaseName == "" {
			return false
		}
		name = sig.BaseName
	}
	return false
}

// paramsFromTypes объединяет имена параметров из AST (astParams) с уже
// резолвленными типами (types, тот же порядок и длина - собраны вместе
// раньше, в collectInterfaces) в []ir.Param - buildIR строит ir.Interface
// только один раз для готовой interfaceSig, отдельная функция вместо
// повторения этого зипа инлайн.
func paramsFromTypes(astParams []ast.Param, types []ir.Type) []ir.Param {
	out := make([]ir.Param, len(astParams))
	for i, p := range astParams {
		out[i] = ir.Param{Name: p.Name, Type: types[i]}
	}
	return out
}

// entityIsOrExtends - name сам ИмяЦели или наследует его (напрямую или
// транзитивно) - используется isAssignable (inheritance.go) для "значение
// производного типа можно использовать там, где ожидается базовый".
func entityIsOrExtends(entities map[string]entitySig, name, target string) bool {
	cur := name
	for range entities {
		if cur == target {
			return true
		}
		sig, ok := entities[cur]
		if !ok || sig.BaseName == "" {
			return false
		}
		cur = sig.BaseName
	}
	return false
}
