package interp

import (
	"strings"
	"testing"
)

// TestRunMultipartParsesFormFieldsAndFile - docs/ROADMAP.md, загрузка
// файлов через HTTP - настоящий multipart/form-data, как его строит curl
// -F/HTML-форма: одно текстовое поле и один файл с двоичным содержимым
// (включая нулевой байт - проверка, что "данные" части реально
// бинарно-безопасны, не обрезаются).
func TestRunMultipartParsesFormFieldsAndFile(t *testing.T) {
	boundary := "----testboundary123"
	fileData := []byte{0x89, 'P', 'N', 'G', 0, 1, 2, 255, 0}
	body := "--" + boundary + "\r\n" +
		"Content-Disposition: form-data; name=\"имя\"\r\n\r\n" +
		"тест" + "\r\n" +
		"--" + boundary + "\r\n" +
		"Content-Disposition: form-data; name=\"фото\"; filename=\"кот.png\"\r\n" +
		"Content-Type: image/png\r\n\r\n" +
		string(fileData) + "\r\n" +
		"--" + boundary + "--\r\n"
	headers := "Content-Type: multipart/form-data; boundary=" + boundary

	parts := parseMultipart([]byte(body), headers)
	elems := parts.Obj.coll.elems
	if len(elems) != 2 {
		t.Fatalf("ожидалось 2 части, получено %d", len(elems))
	}

	f0 := elems[0].Obj.fields
	if f0["имя"].S != "имя" || f0["имя_файла"].S != "" || f0["тип_содержимого"].S != "" {
		t.Fatalf("часть 0: получено имя=%q имя_файла=%q тип=%q", f0["имя"].S, f0["имя_файла"].S, f0["тип_содержимого"].S)
	}
	if got := string(bytesFromValue(f0["данные"])); got != "тест" {
		t.Fatalf("часть 0: данные = %q, хотели \"тест\"", got)
	}

	f1 := elems[1].Obj.fields
	if f1["имя"].S != "фото" || f1["имя_файла"].S != "кот.png" || f1["тип_содержимого"].S != "image/png" {
		t.Fatalf("часть 1: получено имя=%q имя_файла=%q тип=%q", f1["имя"].S, f1["имя_файла"].S, f1["тип_содержимого"].S)
	}
	gotData := bytesFromValue(f1["данные"])
	if string(gotData) != string(fileData) {
		t.Fatalf("часть 1: данные = % x, хотели % x", gotData, fileData)
	}
}

// TestRunMultipartMissingBoundaryPanics - "разобрать_multipart" без
// "boundary=" в Content-Type - паника (rtmsg.MultipartMissingBoundary),
// не пустой/непредсказуемый результат.
func TestRunMultipartMissingBoundaryPanics(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatalf("ожидалась паника")
		}
		re, ok := r.(*runtimeError)
		if !ok {
			t.Fatalf("ожидался *runtimeError, получено %T: %v", r, r)
		}
		if !strings.Contains(re.Error(), "boundary") {
			t.Fatalf("получено %q, ожидалось упоминание boundary", re.Error())
		}
	}()
	parseMultipart([]byte("данные"), "Content-Type: text/plain")
}
