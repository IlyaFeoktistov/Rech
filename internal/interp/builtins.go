package interp

import (
	"io"
	"math"
	"os"
	"strconv"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

// evalBuiltinCall исполняет встроенную функцию шага 8 (internal/builtins -
// ввод консоли, базовая математика) - см. package doc
// internal/backend/cbackend (readHelpers/builtinCallCode), это тот же
// набор функций, реализованный здесь через стандартную библиотеку Go
// (math.*, strconv.*) вместо <math.h>/<stdio.h>. ok == false - name не
// встроена, вызывающий код (evalExpr, *ir.CallExpr) ищет её среди
// пользовательских функций программы, как и раньше.
func (in *Interp) evalBuiltinCall(name string, args []Value) (Value, bool) {
	switch name {
	case "корень":
		return floatValue(math.Sqrt(args[0].F)), true
	case "степень":
		return floatValue(math.Pow(args[0].F, args[1].F)), true
	case "округлить":
		return intValue(int64(math.Round(args[0].F))), true
	case "потолок":
		return intValue(int64(math.Ceil(args[0].F))), true
	case "пол":
		return intValue(int64(math.Floor(args[0].F))), true
	case "модуль":
		return floatValue(math.Abs(args[0].F)), true
	case "прочитать_строку":
		return stringValue(in.readLine()), true
	case "прочитать_целое":
		line := in.readLine()
		v, err := strconv.ParseInt(line, 10, 64)
		if err != nil {
			panicRuntime("%s", rtmsg.ReadIntFormat(line))
		}
		return intValue(v), true
	case "прочитать_число":
		line := in.readLine()
		v, err := strconv.ParseFloat(line, 64)
		if err != nil {
			panicRuntime("%s", rtmsg.ReadFloatFormat(line))
		}
		return floatValue(v), true
	case "открыть":
		path := args[0].S
		f, err := os.Open(path)
		if err != nil {
			panicRuntime("%s", rtmsg.OpenFailedFormat(path))
		}
		return fileValue(f), true
	case "прочитать":
		max := args[1].I
		if max <= 0 {
			panicRuntime("%s", rtmsg.ReadInvalidMax)
		}
		buf := make([]byte, max)
		n, err := args[0].File.Read(buf)
		if err != nil && err != io.EOF {
			panicRuntime("%s", rtmsg.ReadIOFailed)
		}
		return stringValue(string(buf[:n])), true
	case "закрыть":
		args[0].File.Close()
		return Value{}, true
	default:
		return Value{}, false
	}
}

// readLine читает одну строку из in.stdin, обрезая завершающий перенос
// строки и (для прочитать_целое/прочитать_число, где важно совпадение с
// cbackend побайтово) пробелы/табы по краям - тот же набор символов, что и
// rech_trim_line в сгенерированном C. Конец ввода без единого прочитанного
// байта - panicRuntime (аварийное завершение, как и rech_panic в C), не
// error - в языке нет исключений (шаг 14), которые можно было бы этим
// error'ом наполнить.
func (in *Interp) readLine() string {
	line, err := in.stdin.ReadString('\n')
	if err != nil && line == "" {
		panicRuntime("%s", rtmsg.ReadEOF)
	}
	return strings.Trim(line, " \t\r\n")
}
