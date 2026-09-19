package interp

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRunBytesIORoundTripsArbitraryBinaryData - загрузка/
// раздача файлов - "Список<Байт>" (в отличие от "Строка") обязан
// сохранять ЛЮБОЙ байт, включая 0x00, без обрезания. Круговой путь целиком:
// прочитать_файл_байты -> записать_в_файл_байты И открыть/прочитать_байты/
// записать_байты/закрыть на уже открытом "Файл".
func TestRunBytesIORoundTripsArbitraryBinaryData(t *testing.T) {
	dir := t.TempDir()
	original := filepath.Join(dir, "original.bin")
	data := []byte{0, 1, 2, 255, 254, 0, 128, 10, 13, 0}
	if err := os.WriteFile(original, data, 0o644); err != nil {
		t.Fatalf("запись исходного файла: %v", err)
	}
	copyWhole := filepath.Join(dir, "copy_whole.bin")
	copyManual := filepath.Join(dir, "copy_manual.bin")

	originalLit := strings.ReplaceAll(original, `\`, `\\`)
	copyWholeLit := strings.ReplaceAll(copyWhole, `\`, `\\`)
	copyManualLit := strings.ReplaceAll(copyManual, `\`, `\\`)

	runSrc(t, `
функция главная
{
    переменная данные = прочитать_файл_байты("`+originalLit+`")
    вывести `+"`${"+`данные.длина()}\n`+"`"+`
    записать_в_файл_байты("`+copyWholeLit+`", данные)

    переменная файл = открыть("`+originalLit+`")
    переменная часть = прочитать_байты(файл, 1000)
    закрыть(файл)
    переменная файл2 = открыть_для_записи("`+copyManualLit+`")
    записать_байты(файл2, часть)
    закрыть(файл2)
}
`)

	gotWhole, err := os.ReadFile(copyWhole)
	if err != nil {
		t.Fatalf("чтение copy_whole.bin: %v", err)
	}
	if !bytes.Equal(gotWhole, data) {
		t.Fatalf("copy_whole.bin: получено % x, хотели % x", gotWhole, data)
	}
	gotManual, err := os.ReadFile(copyManual)
	if err != nil {
		t.Fatalf("чтение copy_manual.bin: %v", err)
	}
	if !bytes.Equal(gotManual, data) {
		t.Fatalf("copy_manual.bin: получено % x, хотели % x", gotManual, data)
	}
}

// TestRunByteStringConvertersRoundTrip - "байты_из_строки"/"строка_из_
// байт" - мост между "Список<Байт>" и "Строка".
func TestRunByteStringConvertersRoundTrip(t *testing.T) {
	got := runSrc(t, `
функция главная
{
    переменная байты = байты_из_строки("привет, мир!")
    вывести `+"`${"+`байты.длина()}\n`+"`"+`
    вывести `+"`${"+`строка_из_байт(байты)}\n`+"`"+`
}
`)
	if got != "21\nпривет, мир!\n" {
		t.Fatalf("получено %q, хотели \"21\\nпривет, мир!\\n\"", got)
	}
}
