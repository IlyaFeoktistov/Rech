package diagnostics

import (
	"errors"
	"fmt"
)

// AsDiagnostic разворачивает err до *Diagnostic через errors.As (проходит
// сквозь любую цепочку fmt.Errorf("...: %w", err) - см. package doc про
// постепенную миграцию: верхние уровни оборачивают ошибку нижнего уровня
// текстовым контекстом, но сама диагностика (код/позиция) остаётся той,
// что создана глубже всего, ближе к настоящему месту ошибки). err без
// *Diagnostic в цепочке - оборачивается как есть, без кода и без позиции
// (до-миграционный путь).
func AsDiagnostic(err error) *Diagnostic {
	if err == nil {
		return nil
	}
	var d *Diagnostic
	if errors.As(err, &d) {
		return d
	}
	return &Diagnostic{Message: err.Error()}
}

// WithFile проставляет Pos.File у диагностики внутри err, если она ещё не
// установлена - лексер/парсер сами не знают путь разбираемого файла (их
// вход - голый текст, см. Frontend.Parse(src string)), только вызывающая
// сторона (internal/module.loader.load, internal/compiler.parseFileRaw)
// знает, какой именно файл сейчас читается. Диагностика, у которой File
// уже стоит (ошибка всплыла из ГЛУБЖЕ - например, из файла, подключённого
// через "использовать", а не из самого path), остаётся как есть - иначе
// путь ошибки перезаписывался бы путём каждого файла на пути наверх, а не
// тем, где ошибка произошла на самом деле.
func WithFile(err error, file string) error {
	if err == nil {
		return nil
	}
	if b, ok := err.(*Bag); ok {
		b.SetFileIfEmpty(file)
		return b
	}
	d := AsDiagnostic(err)
	if d.Pos.File == "" {
		d.Pos.File = file
	}
	return d
}

// WrapAt разворачивает err до его *Diagnostic (см. AsDiagnostic - проходит
// сквозь fmt.Errorf("...: %w", err)-цепочки) и возвращает КОПИЮ с текстовым
// контекстом, добавленным ПЕРЕД текущим сообщением (тот же эффект, что и у
// сегодняшнего fmt.Errorf("%s %q: %w", label, name, err) - контекст не
// теряется при переходе на структурные диагностики), и с позицией pos,
// проставленной, ТОЛЬКО если у диагностики её ещё нет (Pos.Line == 0) -
// самая глубокая уже известная позиция побеждает, эта функция не
// перезаписывает её чужой. format == "" - контекст не добавляется (см.
// WithPos, тот же путь без текста).
func WrapAt(err error, pos Position, format string, args ...any) error {
	d := AsDiagnostic(err)
	cp := *d
	if format != "" {
		ctx := fmt.Sprintf(format, args...)
		if cp.Message == "" {
			cp.Message = ctx
		} else {
			cp.Message = ctx + ": " + cp.Message
		}
	}
	if cp.Pos.Line == 0 {
		cp.Pos = pos
	}
	return &cp
}

// Wrap - WrapAt без позиции (см. package doc WrapAt) - прямая замена
// fmt.Errorf("<контекст>: %w", err) без позиции в самом контексте.
func Wrap(err error, format string, args ...any) error {
	return WrapAt(err, Position{}, format, args...)
}

// WithPos - WrapAt без добавления текста, только простановка позиции, если
// её ещё нет - прямая замена fmt.Errorf("строка %d[, столбец %d]: %w", ...,
// err) там, где сама точка кода не добавляет собственный текст, только
// координаты (см. package doc WrapAt).
func WithPos(err error, pos Position) error {
	return WrapAt(err, pos, "")
}
