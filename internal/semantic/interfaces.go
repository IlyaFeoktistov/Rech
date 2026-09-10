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
	// IRMethods - готовые ir.InterfaceMethod (с ИМЕНАМИ параметров, не
	// только типами, как в MethodSigs) в том же порядке, что MethodNames/
	// MethodSigs - buildIR (analyzer.go) просто копирует их в ir.Interface,
	// не переизвлекает из iface.Methods по индексу: при "наследует"
	// унаследованные методы сдвигают индексы относительно AST-списка
	// СОБСТВЕННЫХ методов этого интерфейса (см. collectInterfaces), поэтому
	// "iface.Methods[i]" для унаследованного i была бы неверной сущностью
	// вообще (или паникой за границей среза).
	IRMethods []ir.InterfaceMethod
	// Extends - интерфейсы, перечисленные после "наследует" (см. package
	// doc ast.InterfaceDecl.Extends) - собственный список ЭТОГО интерфейса
	// (не транзитивно свёрнутый), используется interfaceIsOrExtends. Методы
	// уже свёрнуты в MethodNames/MethodSigs (collectInterfaces) - Extends
	// нужен отдельно только для проверки "интерфейс X - это (тоже) Y"
	// (entitySatisfies через цепочку "реализует").
	Extends []string
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
// collectFuncSigs/collectEntities. Параметры/тип результата метода
// интерфейса при этом МОГУТ ссылаться на сущность/структуру программы
// (Наблюдатель.на_событие(событие: СобытиеПроцесса) и т. п.) - без этого
// интерфейсы не годились бы для типичных ООП-паттернов вроде Observer/
// Strategy, у которых метод контракта принимает доменный объект, не только
// примитив. paramEntities ниже даёт resolveTypeName увидеть ИМЕНА всех
// сущностей программы заранее - placeholder entitySig{} (без полей/
// методов) достаточно, полные сигнатуры соберёт collectEntities позже, уже
// для тел функций/сущностей - collectInterfaces никогда не заглядывает в
// entitySig глубже самого факта, что имя существует. Настоящего
// двухпроходного резолвинга это не заводит: "реализует" (обратная
// сторона - сущность против интерфейса) по-прежнему проверяется только в
// collectEntities, здесь сущности используются исключительно КАК ТИПЫ
// параметров/результата, без всякой связи с "реализует".
func (a *Analyzer) collectInterfaces(prog *ast.Program) error {
	interfaces := map[string]interfaceSig{}
	paramEntities := map[string]entitySig{}
	for _, ent := range prog.Entities {
		paramEntities[ent.Name] = entitySig{IsStruct: ent.IsStruct}
	}
	paramMethods := map[string]map[string]funcSig{}
	for _, iface := range prog.Interfaces {
		if _, exists := interfaces[iface.Name]; exists {
			return fmt.Errorf("интерфейс %q объявлен более одного раза", iface.Name)
		}
		// "наследует А, Б" - методы каждого перечисленного интерфейса
		// становятся частью контракта этого интерфейса тоже (сплющивание,
		// тот же принцип, что и у FieldNames/FieldTypes сущности при
		// "наследует" - internal/semantic/inheritance.go) - базовый
		// интерфейс обязан быть уже собран (объявлен раньше в файле, как и
		// у сущностей).
		var sig interfaceSig
		sig.Extends = iface.Extends
		for _, baseName := range iface.Extends {
			base, ok := interfaces[baseName]
			if !ok {
				return fmt.Errorf("интерфейс %q: наследует %q - базовый интерфейс не объявлен (или объявлен позже в файле)", iface.Name, baseName)
			}
			for i, name := range base.MethodNames {
				if sig.methodIndex(name) != -1 {
					return fmt.Errorf("интерфейс %q: метод %q конфликтует с унаследованным методом того же имени из другого базового интерфейса", iface.Name, name)
				}
				sig.MethodNames = append(sig.MethodNames, name)
				sig.MethodSigs = append(sig.MethodSigs, base.MethodSigs[i])
				sig.IRMethods = append(sig.IRMethods, base.IRMethods[i])
			}
		}
		for _, m := range iface.Methods {
			if sig.methodIndex(m.Name) != -1 {
				return fmt.Errorf("интерфейс %q: метод %q объявлен более одного раза", iface.Name, m.Name)
			}
			params := make([]ir.Type, len(m.Params))
			for i, p := range m.Params {
				typ, err := resolveTypeName(paramEntities, paramMethods, nil, interfaces, p.Type)
				if err != nil {
					return fmt.Errorf("интерфейс %q: метод %q: параметр %q: %w", iface.Name, m.Name, p.Name, err)
				}
				params[i] = typ
			}
			msig := funcSig{Params: params}
			switch {
			case m.SelfReturn:
				// "этот" как тип результата контракта - через интерфейсное
				// значение конкретный тип реализующей сущности статически
				// не известен, поэтому резолвится в сам интерфейс
				// (InterfaceType) - самое узкое, что можно утверждать в
				// месте вызова через интерфейсную ссылку (см. package doc
				// funcSig.SelfReturn/sigsMatch).
				msig.ReturnType = ir.InterfaceType(iface.Name)
				msig.HasReturn = true
				msig.SelfReturn = true
			case m.ReturnType != "":
				typ, err := resolveTypeName(paramEntities, paramMethods, nil, interfaces, m.ReturnType)
				if err != nil {
					return fmt.Errorf("интерфейс %q: метод %q: тип результата: %w", iface.Name, m.Name, err)
				}
				msig.ReturnType = typ
				msig.HasReturn = true
			}
			sig.MethodNames = append(sig.MethodNames, m.Name)
			sig.MethodSigs = append(sig.MethodSigs, msig)
			sig.IRMethods = append(sig.IRMethods, ir.InterfaceMethod{
				Name: m.Name, Params: paramsFromTypes(m.Params, params),
				ReturnType: msig.ReturnType, HasReturn: msig.HasReturn,
			})
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
// сигнатурой (число/типы параметров, тип результата) - без частичной
// реализации, тот же принцип строгости, что и у остальных проверок типов
// языка ("без неявных преобразований, ни здесь"). Единственное управляемое
// исключение - тип результата "этот" (SelfReturn, sigsMatch) - у него
// сравнивается сам факт "результат - self", не конкретный резолвленный
// тип (тот у интерфейса и у реализации законно разный).
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
//
// Тип результата "этот" (SelfReturn) сравнивается ПО ФЛАГУ, а не по уже
// резолвленному ReturnType: у a и b он почти всегда законно разный
// (EntityType/InterfaceType СВОЕЙ сущности/интерфейса - см. package doc
// funcSig.SelfReturn) - это и есть совместимость, а не расхождение.
// Переопределение/реализация обязаны совпадать именно по факту "результат -
// self или нет" - переопределить SelfReturn-метод обычным конкретным типом
// результата (и наоборот) запрещено, чтобы не разбирать на этапе
// диспетчеризации (cbackend/vtable.go), какой из двух смыслов имелся в
// виду у конкретного слота.
func sigsMatch(a, b funcSig) bool {
	if a.SelfReturn != b.SelfReturn {
		return false
	}
	if !a.SelfReturn && (a.HasReturn != b.HasReturn || (a.HasReturn && a.ReturnType != b.ReturnType)) {
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
// итераций всё равно ставится, а не полагается на это молча). Каждый
// "реализует"-интерфейс сущности (own) засчитывается и тогда, когда сам
// own лишь наследует ifaceName (не обязательно совпадает с ним дословно) -
// см. interfaceIsOrExtends.
func entitySatisfies(entities map[string]entitySig, interfaces map[string]interfaceSig, entityName, ifaceName string) bool {
	name := entityName
	for range entities {
		sig, ok := entities[name]
		if !ok {
			return false
		}
		for _, own := range sig.OwnInterfaces {
			if interfaceIsOrExtends(interfaces, own, ifaceName) {
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

// interfaceIsOrExtends - name сам ifaceName или наследует его (напрямую
// или транзитивно, через "наследует" у самого интерфейса) - тот же принцип,
// что entityIsOrExtends (inheritance.go), но по interfaceSig.Extends
// (список, не одно имя - интерфейс может наследовать несколько сразу,
// поэтому обход в глубину по всем, а не цепочкой).
func interfaceIsOrExtends(interfaces map[string]interfaceSig, name, target string) bool {
	if name == target {
		return true
	}
	sig, ok := interfaces[name]
	if !ok {
		return false
	}
	for _, base := range sig.Extends {
		if interfaceIsOrExtends(interfaces, base, target) {
			return true
		}
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
