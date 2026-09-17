package cbackend

import (
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/backend/cbackend/runtime/regex"
)

// TestGenerateFromIRRegexEmitsFindAllHelperAndCorrectFieldAccess -
// "регулярное(строка, шаблон): Список<Совпадение>" - проверяет, что
// codegen действительно эмитит rech_regex_find_all и обращается к полям
// "Совпадение" через "." (структура по значению, не указатель - см.
// package doc regex.go), а не через несуществующий rech_new_sovpadenie.
func TestGenerateFromIRRegexEmitsFindAllHelperAndCorrectFieldAccess(t *testing.T) {
	out := generateFromSrc(t, `
функция главная
{
    переменная м = регулярное("a1 b2", "\\w+")
    вывести м.длина()
}
`)
	if !strings.Contains(out, "rech_regex_find_all(") {
		t.Fatalf("ожидался вызов rech_regex_find_all, получено:\n%s", out)
	}
	start := strings.Index(out, "static "+mangleType(regex.ResultType)+" *rech_regex_find_all")
	if start < 0 {
		t.Fatalf("не найден хелпер rech_regex_find_all, получено:\n%s", out)
	}
	body := out[start:]
	if strings.Contains(body, "rech_new_sovpadenie") {
		t.Fatalf("\"Совпадение\" - структура по значению, не должна вызывать rech_new_X, получено:\n%s", body)
	}
	if !strings.Contains(body, "item."+mangleField("текст")+" = ") {
		t.Fatalf("ожидался доступ к полю через \".\" (значение, не указатель), получено:\n%s", body)
	}
	if strings.Contains(body, "item->") {
		t.Fatalf("\"Совпадение\" не должна использовать \"->\", получено:\n%s", body)
	}
	if !strings.Contains(out, "#include <regex.h>") || !strings.Contains(out, "#include <locale.h>") {
		t.Fatalf("ожидались #include <regex.h>/<locale.h>, получено:\n%s", out)
	}
}

// TestGenerateFromIRRegexTranslatesPerlEscapesToPOSIX - "\d"/"\D"/"\w"/"\W" -
// GNU-расширение glibc НЕ включает "\d"/"\D" вовсе, а "\w"/"\W" у него
// ASCII/locale-осознанные иначе, чем у Go regexp/RE2 (см. package doc
// regex.go) - rech_regex_translate_pattern обязан переводить их в
// "[0-9]"/"[^0-9]"/"[[:alnum:]_]"/"[^[:alnum:]_]" ДО regcomp.
func TestGenerateFromIRRegexTranslatesPerlEscapesToPOSIX(t *testing.T) {
	out := generateFromSrc(t, `
функция главная
{
    переменная м = регулярное("текст", "\\w+")
}
`)
	for _, want := range []string{`"[0-9]"`, `"[^0-9]"`, `"[[:alnum:]_]"`, `"[^[:alnum:]_]"`} {
		if !strings.Contains(out, want) {
			t.Fatalf("ожидался перевод шаблона, содержащий %s, получено:\n%s", want, out)
		}
	}
}
