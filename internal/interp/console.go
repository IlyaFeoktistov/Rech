package interp

import (
	"os"

	"golang.org/x/term"
)

// consoleRawState - исходное состояние терминала, запомненное ОДИН раз при
// первом включении сырого режима - "выключить" обязано вернуть терминал
// как было, а не жёстко закодированные настройки (golang.org/x/term.Restore
// требует именно это значение, полученное от MakeRaw).
var consoleRawState *term.State

// setRawMode - "установить_сырой_режим(Логическое)" - тот же принцип, что
// и internal/backend/cbackend/console.go: golang.org/x/term вместо ручных
// termios/Windows-консольных вызовов - кроссплатформенно уже готовой,
// проверенной библиотекой стандартного расширения Go, не переизобретением
// системных вызовов вручную для каждой платформы отдельно.
func setRawMode(enable bool) {
	fd := int(os.Stdin.Fd())
	if enable {
		if consoleRawState != nil {
			return
		}
		state, err := term.MakeRaw(fd)
		if err == nil {
			consoleRawState = state
			suppressConsoleInputNoise(fd)
		}
		return
	}
	if consoleRawState != nil {
		term.Restore(fd, consoleRawState)
		consoleRawState = nil
	}
}

// terminalWidth/terminalHeight - "ширина_терминала()"/"высота_терминала()" -
// 80x24, если размер не удалось узнать (не терминал вовсе - перенаправлен
// в файл/канал).
func terminalWidth() int64 {
	w, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || w <= 0 {
		return 80
	}
	return int64(w)
}

func terminalHeight() int64 {
	_, h, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || h <= 0 {
		return 24
	}
	return int64(h)
}
