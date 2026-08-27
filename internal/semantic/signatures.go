// Сигнатуры (подписи) функций, методов, конструкторов и т.д. в языке Речь.
package semantic

import (
	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// funcSig - сигнатура функции, собранная предварительным проходом
// (collectFuncSigs) до анализа тел - функции могут вызывать друг друга
// независимо от порядка объявления в файле (включая взаимную рекурсию),
// как и в AST-пути (cbackend.funcSig, тот же принцип, отдельный тип - IR
// не должен знать про cbackend).
type funcSig struct {
	Params     []ir.Type
	ReturnType ir.Type
	HasReturn  bool

	// Visibility - видимость метода (ast.MethodDecl.Visibility); всегда
	// ast.Public для обычных функций - у них нет понятия видимости.
	Visibility ast.Visibility

	// ParamStates - для параметра, объявленного производным типом
	// состояния (шаг 4.1, "параметр: АвторизованныйПользователь"), краткая
	// форма самого состояния ("авторизован") - тот же индекс, что и Params
	// (Params[i] уже резолвлен в тип БАЗОВОЙ сущности, alias-имя потеряно,
	// см. resolveTypeName - без этого поля flow-sensitive проверка шага 5
	// не знала бы, какой именно переход состояния требует параметр).
	// "" - параметр объявлен без производного типа состояния (обычный тип,
	// в т.ч. обычная сущность без каких-либо требований к состоянию).
	ParamStates []string

	// GuaranteedThisStates - только для методов (funcSig как элемент
	// methods[Сущность][Метод]): набор состояний, которые этот метод
	// доказанно устанавливает на "этот" на всех путях выполнения (см.
	// analyzeMethod) - используется analyzeMethodCallExpr, чтобы после
	// вызова метода-транзиции (например, "получатель.авторизовать()")
	// перенести доказанный переход на переменную-получателя вызывающей
	// стороны. nil у обычных функций (никогда не заполняется) и у методов,
	// ещё не проанализированных к моменту вызова (взаимная рекурсия
	// методов - безопасный дефолт "ничего не доказано").
	GuaranteedThisStates map[string]bool
}

// entitySig - сигнатура сущности: поля в порядке объявления (для
// конструктора и доступа к полю, шаг 4.b), собранная предварительным
// проходом (collectEntitySigs) - тот же принцип, что и funcSig, отдельный
// тип от cbackend.entitySig (IR не должен знать про cbackend).
type entitySig struct {
	FieldNames []string
	FieldTypes []ir.Type

	// FieldVisibility - видимость соответствующего поля (тот же индекс,
	// что и FieldNames/FieldTypes); nil у сущностей без ни одного
	// непубличного поля.
	FieldVisibility []ast.Visibility

	// States - краткие формы, объявленные под "состояние:" (шаг 4.1,
	// упрощённая версия) - для проверки BecomeStmt ("получатель становится
	// <состояние>"). nil у сущностей без "состояние:".
	States map[string]bool

	// IsList/ElemType - заполнены только для синтезированных инстанциаций
	// встроенных generic-коллекций (шаг 4.2, resolveTypeName) - IsList
	// true только у "Список<T>" (единственная коллекция, участвующая в
	// цикле "для x в списке"), ElemType - у всех трёх (Список/Стопка/
	// Очередь), чтобы Analyze мог собрать Program.Generics в конце без
	// повторного разбора имени. У обычных пользовательских сущностей -
	// нулевые значения.
	IsList   bool
	ElemType ir.Type

	// ConstNames/ConstExprs - константы, объявленные в теле сущности
	// (последовательность объявления). ConstExprs хранит AST-выражения
	// для значения константы, чтобы их можно было проанализировать позже
	// в контексте (analyzeEntity / analyzeExpr при обращении через
	// Сущность.Константа).
	ConstNames []string
	ConstExprs []ast.Expr
	// ConstVisibility - видимость соответствующей константы
	ConstVisibility []ast.Visibility

	// IsStruct - см. ast.EntityDecl.IsStruct - нужно здесь, чтобы
	// collectEntities мог отказать в "наследует"/"реализует" на базовую
	// сущность, которая сама - "структура" (наследование/интерфейсы
	// ограничены ссылочными типами), не только в обратную сторону
	// (парсер уже не пропускает "структура ... наследует ...").
	IsStruct bool
	// BaseName - имя базовой сущности ("наследует"), "" - нет
	// базовой. FieldNames/FieldTypes/... выше уже включают унаследованные
	// поля (плоский список, база первой) - см. collectEntities.
	BaseName string
	// OwnInterfaces - имена интерфейсов из СОБСТВЕННОГО "реализует" этой
	// сущности (не включает унаследованные через BaseName - см.
	// entitySatisfies, которая обходит цепочку BaseName сама, а не
	// полагается на предварительно свёрнутый список).
	OwnInterfaces []string
}

func (s entitySig) constIndex(name string) int {
	for i, n := range s.ConstNames {
		if n == name {
			return i
		}
	}
	return -1
}

func (s entitySig) fieldIndex(name string) int {
	for i, n := range s.FieldNames {
		if n == name {
			return i
		}
	}
	return -1
}
