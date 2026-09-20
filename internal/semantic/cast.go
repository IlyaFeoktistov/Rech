package semantic

import (
	"github.com/IlyaFeoktistov/Rech/internal/ast"
	"github.com/IlyaFeoktistov/Rech/internal/diagnostics"
	"github.com/IlyaFeoktistov/Rech/internal/ir"
)

// numericRanks - относительный "вес" числовых типов, участвующих в
// арифметике (+ - * /) и унарном "-": чем больше значение, тем шире тип.
// Смешанная арифметика приводит результат к более широкому из двух
// операндов (Целое+Дробное -> Дробное, Дробное+Действительное ->
// Действительное, Целое+Действительное -> Действительное - как и раньше).
// Символ/Байт сюда не входят - арифметика над ними не поддерживается в
// этом срезе (это код/индекс, не число для вычислений).
var numericRanks = map[ir.Type]int{
	ir.TypeInt:     0,
	ir.TypeFloat32: 1,
	ir.TypeFloat:   2,
}

// isIntegerArithmeticType - true для "Целое" и любого фиксированной ширины
// Целое8/16/32/64/Натуральное/8/16/32/64 (analyzeArithmetic) - целочисленное
// "/"/"%" на этих типах - рантайм-ошибка деления на ноль (в отличие от
// Дробное/Действительное, где результат "/0"/"%0" - обычный IEEE 754
// бесконечность/NaN через fmod/fmodf, исключение не нужно, см.
// registerDivByZeroException).
func isIntegerArithmeticType(t ir.Type) bool {
	if t.Kind == ir.KindInt {
		return true
	}
	_, _, ok := t.Kind.IntWidthSigned()
	return ok
}

// orderableTypes - типы, поддерживающие "< > <= >=" (сравнение по величине,
// не только на равенство) - числа плюс Символ/Байт (у обоих естественный
// порядок - код точки/значение байта), но не Строка/Логическое.
var orderableTypes = map[ir.Type]bool{
	ir.TypeInt:     true,
	ir.TypeFloat32: true,
	ir.TypeFloat:   true,
	ir.TypeChar:    true,
	ir.TypeByte:    true,
}

// entityIsRefType - name ссылочная (entitySig.IsStruct == false) - тот же
// признак, что cbackend.entityIsRef (ir_generator.go), только через
// entitySig, уже доступную в internal/semantic (cbackend её видит через
// свою копию ir.Entity.IsStruct - обе стороны читают ОДНО И ТО ЖЕ поле,
// заполненное buildIR из entitySig.IsStruct, не дублируют логику разными
// путями). "Настоящая" пользовательская сущность ("сущность"/"класс") - как
// и generic-инстанциация (Список<T> и т. п. - synthесized entitySig без
// IsStruct, см. resolveGenericTypeName - всегда ссылочная тоже) - true;
// "структура" - false.
func entityIsRefType(entities map[string]entitySig, name string) bool {
	sig, ok := entities[name]
	return ok && !sig.IsStruct
}

// equatableTypes - типы, поддерживающие "==" "!=" помимо Строка (сравнение
// содержимого) и уже отдельно проверяемого случая "оба операнда - одна и та
// же сущность/Строка".
var equatableTypes = map[ir.Type]bool{
	ir.TypeInt:     true,
	ir.TypeFloat32: true,
	ir.TypeFloat:   true,
	ir.TypeBool:    true,
	ir.TypeChar:    true,
	ir.TypeByte:    true,
}

// structFieldsEquatable проверяет, годится ли "структура" name для "=="/
// "!=" (поле-в-поле) - КАЖДОЕ поле обязано быть либо уже
// equatableTypes/Строка (сравнение содержимого), либо KindEntity -
// сущностью-ссылкой ИЛИ generic-коллекцией (Список<T> и т. п. -
// entityIsRefType считает их ссылочными безусловно, см. её package doc) -
// сравнивается по IDENTITY, не по содержимому (та же семантика, что и у
// самого "==" для ссылочной сущности верхнего уровня), либо вложенной
// "структурой" (рекурсивно, той же проверкой - структура физически не может
// содержать сама себя полем, C не допустил бы такой typedef, значит
// рекурсия гарантированно конечна, циклы невозможны). Любой ДРУГОЙ тип поля
// (nullable/Вызываемый/Указатель<T>/интерфейс и т. п. - НЕ KindEntity)
// делает саму "структуру" целиком НЕ сравнимой - имя и тип проблемного поля
// называются в ошибке явно, не молча пропускаются и не сравниваются неверно
// во время выполнения.
func structFieldsEquatable(entities map[string]entitySig, name string) error {
	sig := entities[name]
	for i, ft := range sig.FieldTypes {
		if ft == ir.TypeString || equatableTypes[ft] {
			continue
		}
		if ft.Kind == ir.KindEntity {
			if entityIsRefType(entities, ft.Entity) {
				continue
			}
			if err := structFieldsEquatable(entities, ft.Entity); err != nil {
				return diagnostics.Wrap(err, "поле %q (структура %s)", sig.FieldNames[i], ft)
			}
			continue
		}
		return errNoPos(CodeTypeMismatch, "поле %q имеет тип %s, сравнение структур с таким полем не поддерживается", sig.FieldNames[i], ft)
	}
	return nil
}

// elemEquatable - t годится элементом (значением) "Список<T>"/"Словарь<К,
// В>" для метода "равен" (глубокое сравнение содержимого -
// найдено по ходу разбора юнит-тестирования: "==" у самих коллекций
// сравнивает по identity, содержимое сравнить было нечем без ручного
// цикла) - той же трёхчастной проверкой, что и structFieldsEquatable выше
// (примитив/Строка, ссылочная сущность - identity, "структура" - рекурсивно
// поле-в-поле), плюс четвёртый случай, которого нет у полей структуры:
// вложенная встроенная коллекция (Список<Список<Целое>>) - тогда решает уже
// вычисленное Equatable ЭТОЙ инстанциации (entitySig.Equatable) - к моменту
// вызова оно уже заполнено, потому что resolveTypeName сперва рекурсивно
// разрешает внутренний тип-аргумент, значит внешний resolveGenericTypeName/
// resolveDictTypeName видит его entitySig уже полностью готовой.
func elemEquatable(entities map[string]entitySig, t ir.Type) bool {
	if t == ir.TypeString || equatableTypes[t] {
		return true
	}
	if t.Kind != ir.KindEntity {
		return false
	}
	sig, ok := entities[t.Entity]
	if !ok {
		return false
	}
	if sig.IsList || sig.IsDict {
		return sig.Equatable
	}
	if entityIsRefType(entities, t.Entity) {
		return true
	}
	return structFieldsEquatable(entities, t.Entity) == nil
}

// fixedWidthIntTypes - Целое8/16/32/64 и Натуральное/8/16/32/64 - в отличие
// от Целое (numericRanks, неявная промоция к Дробное/Действительное),
// участвуют в "< > <= >= == !=" и в арифметике (+ - * /, analyzeArithmetic)
// СТРОГО между операндами одного и того же Kind, без какой-либо промоции
// ("Целое8 + Целое16" - ошибка компиляции, нужен явный каст). Заполняет
// orderableTypes/equatableTypes и симметричный allowedCasts между собой и
// уже существующими числовыми/Строка/Символ/Байт - вручную выписывать ~90
// пар было бы избыточным риском опечатки/пропуска.
var fixedWidthIntTypes = []ir.Type{
	ir.TypeInt8, ir.TypeInt16, ir.TypeInt32, ir.TypeInt64,
	ir.TypeUint, ir.TypeUint8, ir.TypeUint16, ir.TypeUint32, ir.TypeUint64,
}

func init() {
	allNumeric := append([]ir.Type{ir.TypeInt, ir.TypeFloat32, ir.TypeFloat}, fixedWidthIntTypes...)
	for _, t := range fixedWidthIntTypes {
		orderableTypes[t] = true
		equatableTypes[t] = true
		if allowedCasts[t] == nil {
			allowedCasts[t] = map[ir.Type]bool{}
		}
		for _, other := range allNumeric {
			if other == t {
				continue
			}
			allowedCasts[t][other] = true
			if allowedCasts[other] == nil {
				allowedCasts[other] = map[ir.Type]bool{}
			}
			allowedCasts[other][t] = true
		}
		for _, other := range []ir.Type{ir.TypeString, ir.TypeChar, ir.TypeByte} {
			allowedCasts[t][other] = true
			allowedCasts[other][t] = true
		}
	}
}

// allowedCasts - допустимые пары (исходный тип -> целевой тип) для явного
// приведения "(Тип) значение". Строка на обоих концах - через разбор/
// форматирование в рантайме (может провалиться на исходном значении,
// например "(Целое) \"abc\"" - паника, тот же принцип, что и у
// прочитать_целое); Символ на строковом конце - именно как один символ
// (длина строки должна быть ровно 1 кодовая точка), не как код числом -
// для кода числом есть Символ<->Целое отдельно.
var allowedCasts = map[ir.Type]map[ir.Type]bool{
	ir.TypeInt: {
		ir.TypeFloat:   true,
		ir.TypeFloat32: true,
		ir.TypeString:  true,
		ir.TypeChar:    true,
		ir.TypeByte:    true,
	},
	ir.TypeFloat: {
		ir.TypeInt:     true,
		ir.TypeFloat32: true,
		ir.TypeString:  true,
	},
	ir.TypeFloat32: {
		ir.TypeInt:    true,
		ir.TypeFloat:  true,
		ir.TypeString: true,
	},
	ir.TypeString: {
		ir.TypeInt:     true,
		ir.TypeFloat:   true,
		ir.TypeFloat32: true,
		ir.TypeChar:    true,
		ir.TypeByte:    true,
		ir.TypeBool:    true,
	},
	ir.TypeBool: {
		// Строка<->Логическое: "истина"/"ложь" - тот же текст, что и вывод
		// "вывести"/"показать" для Логическое (format.go/show.go) - каст
		// туда и обратно, значит, обратим (round-trip). Найденный пробел -
		// раньше единственным способом получить строковое представление
		// Логическое было "вывести"/"показать" (инструкции, не выражения) -
		// "текст + (Строка) булево" не работал вовсе, обнаружено при
		// добавлении шаблонных строк (интерполяция ${булево}).
		ir.TypeString: true,
	},
	ir.TypeChar: {
		ir.TypeInt:    true,
		ir.TypeString: true,
		ir.TypeByte:   true,
	},
	ir.TypeByte: {
		ir.TypeInt:     true,
		ir.TypeFloat:   true,
		ir.TypeFloat32: true,
		ir.TypeString:  true,
		ir.TypeChar:    true,
	},
}

// analyzeCastExpr - "(Тип) значение" - Тип уже ограничен парсером
// фиксированным набором примитивов (parser.primitiveCastTypes), здесь
// только проверяется, что конкретная пара (исходный тип значения,
// целевой тип) вообще имеет смысл (allowedCasts) - сама успешность
// приведения runtime-значения (разбор строки как числа и т.п.) проверяется
// во время исполнения, не здесь.
func analyzeCastExpr(sc *ctx, e *ast.CastExpr) (ir.Expr, error) {
	to, err := resolveTypeName(sc.entities, sc.methods, sc.aliases, sc.interfaces, sc.enums, e.Type)
	if err != nil {
		return nil, diagnostics.WithPos(err, diagnostics.Position{File: currentFile, Line: e.Line})
	}
	value, err := analyzeExpr(sc, e.Value)
	if err != nil {
		return nil, err
	}
	// Доказанная непустота (established, "если а != Ничто {...}")
	// разворачивает nullable ДО решения from/to - без этого "(Целое)
	// из_окружения" (из_окружения: ?Строка, уже сужена) не находила пути:
	// цель (Целое) не совпадает с внутренним типом (Строка) буквально, а
	// allowedCasts ниже не знает про "?Строка" вовсе - НАЙДЕННОЕ И
	// ИСПРАВЛЕННОЕ ограничение (Шаг 41/44). Не
	// сужена - maybeUnwrapNarrowed возвращает value как есть, ветка
	// "from.Kind == ir.KindNullable" ниже отрабатывает по-прежнему (каст в
	// ИМЕННО внутренний тип - единственный путь развернуть nullable БЕЗ
	// доказанной непустоты, с рантайм-паникой на "Ничто").
	value = maybeUnwrapNarrowed(sc, value)
	from := ir.TypeOf(value)
	if from == to {
		return value, nil
	}
	// Разворот "?Тип" в Тип - явный каст, единственный путь ВНЕ доказанной
	// непустоты (см. package doc ir.NullUnwrapExpr) - паникует в рантайме на
	// "Ничто", тем же принципом, что и остальные касты, способные
	// провалиться (Строка -> число). Не через allowedCasts ниже - у
	// nullable-типов нет фиксированной записи там (их бесконечно много, по
	// одной на каждый внутренний Тип), проверяется структурно: "to" должен
	// быть именно внутренним типом "from".
	if from.Kind == ir.KindNullable {
		if nullableInner(sc, from) == to {
			return &ir.NullUnwrapExpr{Value: value, Type: to, Line: e.Line}, nil
		}
		return nil, errAtLine(e.Line, CodeTypeMismatch, "нельзя привести %s к %s", from, to)
	}
	// Боксирование в "Значение" - разрешено для ЛЮБОГО from (примитив,
	// сущность, структура), не только перечисленных в allowedCasts пар -
	// иначе пришлось бы вручную выписывать пару на каждый существующий и
	// будущий тип программы. Разбоксирование - отдельная конструкция
	// "значение как Тип" (analyzeAsExpr), не эта функция.
	//
	// ИСКЛЮЧЕНИЕ - from.Kind == ir.KindInterface (значение статически
	// типизировано интерфейсом): НЕ разрешено, в отличие от остальных.
	// Боксирование другого направления (сущность -> "значение как
	// ИмяИнтерфейса") работает - дескриптор боксированного
	// значения ВСЕГДА принадлежит КОНКРЕТНОЙ сущности (см. её package doc в
	// internal/backend/cbackend/reflect.go, ensureInterface), а на этапе
	// компиляции здесь неизвестно, какая ИМЕННО сущность стоит за
	// интерфейсно-типизированным выражением - у cbackend/interp попросту
	// нет способа узнать это в рантайме без отдельного канала передачи
	// runtime-типа (интерфейсное значение представлено голым "void *", без
	// сопровождающего тега типа - см. ctx.receiverIsPointer, vtable.go).
	// Обход - боксировать КОНКРЕТНОЕ значение до того, как оно попадёт в
	// интерфейсно-типизированную переменную/параметр.
	if to == ir.TypeAny {
		if from.Kind == ir.KindInterface {
			return nil, errAtLine(e.Line, CodeTypeMismatch, "нельзя боксировать в \"Значение\" интерфейсно-типизированное выражение (%s) - боксируйте конкретное значение до того, как оно станет %s", from, from)
		}
		sc.a.markUsesReflection()
		if from.Kind == ir.KindEntity {
			sc.a.registerReflectTarget(from)
		}
		return &ir.CastExpr{Value: value, To: to}, nil
	}
	if !allowedCasts[from][to] {
		return nil, errAtLine(e.Line, CodeTypeMismatch, "нельзя привести %s к %s", from, to)
	}
	return &ir.CastExpr{Value: value, To: to}, nil
}
