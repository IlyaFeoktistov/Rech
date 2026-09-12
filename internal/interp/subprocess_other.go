//go:build !windows

package interp

// decodeShellOutput - на любой ОС, кроме Windows, "sh" пишет байты в той же
// UTF-8-локали, что и весь остальной Rech - декодировать заново нечего (см.
// subprocess_windows.go - там наоборот, обязательно нужно перекодировать).
func decodeShellOutput(b []byte) string {
	return string(b)
}
