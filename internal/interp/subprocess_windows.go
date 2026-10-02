//go:build windows

package interp

import "golang.org/x/sys/windows"

// decodeShellOutput - "выполнить_команду" перехватывает stdout/stderr
// cmd.exe В ПАЙП (не в настоящую консоль) - живой прогон на Windows нашёл:
// "chcp 65001" (переключение НАСТОЯЩЕЙ консоли на UTF-8) на такой вывод НЕ
// действует вовсе - "echo привет" продолжает писать байты в кодовой
// странице OEM (windows-866/437/850 - зависит от локали машины, не от
// chcp), интерпретация их как UTF-8 (то, чем является ЛЮБАЯ Rech-строка)
// дала бы нечитаемый мусор. Перекодируем ПОСЛЕ запуска через собственный
// Win32 API конвертации кодовых страниц, не жёстко зашитую кодовую
// страницу - CP_OEMCP (1) означает "текущая активная OEM-страница ЭТОЙ
// системы", Windows сама разрешает её в правильное число.
const cpOEM = 1

func decodeShellOutput(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	// Дочерний cmd.exe пишет в кодировке консоли, которую унаследовал
	// (GetConsoleOutputCP; без консоли - OEM) - то же правило, что у
	// cbackend (rech_win_oem_to_utf8).
	cp := uint32(cpOEM)
	if c, err := windows.GetConsoleOutputCP(); err == nil && c != 0 {
		cp = c
	}
	n, err := windows.MultiByteToWideChar(cp, 0, &b[0], int32(len(b)), nil, 0)
	if err != nil || n == 0 {
		return string(b)
	}
	buf := make([]uint16, n)
	if _, err := windows.MultiByteToWideChar(cp, 0, &b[0], int32(len(b)), &buf[0], n); err != nil {
		return string(b)
	}
	return windows.UTF16ToString(buf)
}
