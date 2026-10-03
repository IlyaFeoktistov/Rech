package cbackend

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

// #line на исходник Rech в отладочной сборке: отладчик (gdb/lldb) по
// -g показывает строки .rech, а не сгенерированного C.
//
// Кодогенерация обрамляет каждую инструкцию маркерами (lineMarkerPush с
// файлом и строкой перед ней, lineMarkerPop после), вложенные инструкции
// (тела если/пока/попытаться) - вложенными парами. resolveLineMarkers
// превращает их в настоящие #line: каждая C-строка внутри инструкции
// закрепляется за её строкой Rech (иначе компилятор считал бы следующие
// C-строки следующими строками .rech), "}"/"else" после вложенной
// инструкции - за внешней, а вне всех инструкций (заголовки функций,
// рантайм) счёт возвращается на сам .c - его имя на этапе генерации
// неизвестно, вместо него CFilePlaceholder (подставляет компилятор,
// ResolveCFile).

const (
	lineMarkerPush = "#rech-line "
	lineMarkerPop  = "#rech-line-end"
	// CFilePlaceholder - имя сгенерированного .c в #line, пока оно
	// неизвестно (см. ResolveCFile).
	CFilePlaceholder = "__RECH_C_FILE__"
)

// lineMarkerOpen - маркер начала инструкции строки line текущего файла
// ctx.srcFile; "" - в релизе, без файла или строки.
func lineMarkerOpen(ctx *irGenCtx, line int) string {
	if ctx.release || ctx.srcFile == "" || line <= 0 {
		return ""
	}
	return fmt.Sprintf("%s%d %s\n", lineMarkerPush, line, strconv.Quote(ctx.srcFile))
}

// lineMarkerClose - маркер конца инструкции, парный lineMarkerOpen.
func lineMarkerClose(open string) string {
	if open == "" {
		return ""
	}
	return lineMarkerPop + "\n"
}

// sourcePath - путь исходника для #line: абсолютный, чтобы отладчик нашёл
// файл из любого каталога запуска.
func sourcePath(file string) string {
	if file == "" {
		return ""
	}
	if abs, err := filepath.Abs(file); err == nil {
		return abs
	}
	return file
}

// resolveLineMarkers - маркеры src в директивы #line (см. package doc
// выше). Без маркеров src возвращается как есть.
func resolveLineMarkers(src string) string {
	if !strings.Contains(src, lineMarkerPush) {
		return src
	}
	lines := strings.Split(src, "\n")
	var out strings.Builder
	out.Grow(len(src) * 2)
	outLine := 0 // номер последней записанной строки
	write := func(s string) {
		out.WriteString(s)
		out.WriteByte('\n')
		outLine++
	}
	// Весь файл - под именем .c, которое увидит отладчик (а не временным,
	// из которого он компилируется).
	write(fmt.Sprintf("#line 2 \"%s\"", CFilePlaceholder))
	var stack []string // директивы #line открытых инструкций
	afterDirective := false
	prevContinues := false
	for i, l := range lines {
		if i == len(lines)-1 && l == "" {
			break
		}
		switch {
		case strings.HasPrefix(l, lineMarkerPush):
			d := "#line " + strings.TrimPrefix(l, lineMarkerPush)
			stack = append(stack, d)
			write(d)
			afterDirective = true
			continue
		case l == lineMarkerPop:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
			if len(stack) > 0 {
				write(stack[len(stack)-1])
			} else {
				// Следующая строка .c - outLine+2: сама директива - outLine+1.
				write(fmt.Sprintf("#line %d \"%s\"", outLine+2, CFilePlaceholder))
			}
			afterDirective = true
			continue
		}
		if len(stack) > 0 && !afterDirective && !prevContinues {
			write(stack[len(stack)-1])
		}
		write(l)
		afterDirective = false
		prevContinues = strings.HasSuffix(l, "\\")
	}
	return out.String()
}

// ResolveCFile - CFilePlaceholder в src -> путь cPath сгенерированного .c.
func ResolveCFile(src, cPath string) string {
	if abs, err := filepath.Abs(cPath); err == nil {
		cPath = abs
	}
	q := strconv.Quote(cPath)
	return strings.ReplaceAll(src, `"`+CFilePlaceholder+`"`, q)
}
