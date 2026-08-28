package semantic

import (
	"fmt"

	"github.com/IlyaFeoktistov/Rech/internal/ast"
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

// fixedWidthIntTypes - Целое8/16/32/64 и Натуральное/8/16/32/64 (шаг 12) -
// в отличие от Целое (numericRanks, неявная промоция к Дробное/
// Действительное), участвуют в "< > <= >= == !=" и в арифметике (+ - * /,
// analyzeArithmetic) СТРОГО между операндами одного и того же Kind, без
// какой-либо промоции - решение пользователя (см. docs/ROADMAP.md, Шаг 12:
// "Целое8 + Целое16 - ошибка компиляции, нужен явный каст"). Заполняет
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
	to, err := resolveTypeName(sc.entities, sc.methods, sc.aliases, sc.interfaces, e.Type)
	if err != nil {
		return nil, fmt.Errorf("строка %d: %w", e.Line, err)
	}
	value, err := analyzeExpr(sc, e.Value)
	if err != nil {
		return nil, err
	}
	from := ir.TypeOf(value)
	if from == to {
		return value, nil
	}
	if !allowedCasts[from][to] {
		return nil, fmt.Errorf("строка %d: нельзя привести %s к %s", e.Line, from, to)
	}
	return &ir.CastExpr{Value: value, To: to}, nil
}
