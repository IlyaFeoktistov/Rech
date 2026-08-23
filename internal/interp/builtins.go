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
		return in.openFile(name, args[0].S, os.O_RDONLY, 0), true
	case "открыть_для_записи":
		return in.openFile(name, args[0].S, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644), true
	case "открыть_для_дозаписи":
		return in.openFile(name, args[0].S, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644), true
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
	case "записать":
		n, err := args[0].File.WriteString(args[1].S)
		if err != nil || n != len(args[1].S) {
			panicRuntime("%s", rtmsg.WriteIOFailed)
		}
		return Value{}, true
	case "закрыть":
		args[0].File.Close()
		return Value{}, true
	case "стандартный_ввод":
		// os.Stdin напрямую, не через in.stdin (bufio.Reader, которым уже
		// читают прочитать_строку/прочитать_целое/прочитать_число) -
		// известное ограничение: смешивать эти встроенные функции с
		// "прочитать"/"для X в стандартный_ввод()" в одной программе не
		// поддерживается (у обоих независимая буферизация, возможна потеря
		// уже прочитанных, но не возвращённых вызывающему коду байт) - в
		// отличие от cbackend, где и то, и другое читает через один и тот
		// же C stdio `stdin`.
		return fileValue(os.Stdin), true
	case "стандартный_вывод":
		return fileValue(os.Stdout), true
	case "стандартный_вывод_ошибок":
		return fileValue(os.Stderr), true
	case "количество_аргументов":
		// "rech test <файл>" не передаёт интерпретируемой программе своих
		// аргументов (аргументы после <файл> - аргументы самого rech, не
		// программы) - всегда 0, "аргумент" ниже всегда паникует диапазоном.
		return intValue(0), true
	case "аргумент":
		panicRuntime("%s", rtmsg.ArgIndexOutOfRangeFormat(args[0].I))
		return Value{}, true
	case "содержит":
		return boolValue(strings.Contains(args[0].S, args[1].S)), true
	default:
		return Value{}, false
	}
}

// openFile - общая реализация "открыть"/"открыть_для_записи"/
// "открыть_для_дозаписи" - name (реальное имя вызванной функции) идёт в
// сообщение об ошибке (rtmsg.OpenFailedFormat), 0o644 - права нового файла
// для режимов записи/дозаписи (POSIX-права ничего не значат при чтении
// уже существующего файла, поэтому там передаётся 0 - os.OpenFile его
// просто игнорирует без os.O_CREATE).
func (in *Interp) openFile(name, path string, flag int, perm os.FileMode) Value {
	f, err := os.OpenFile(path, flag, perm)
	if err != nil {
		panicRuntime("%s", rtmsg.OpenFailedFormat(name, path))
	}
	return fileValue(f)
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
