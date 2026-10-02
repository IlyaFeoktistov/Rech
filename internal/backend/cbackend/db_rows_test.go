package cbackend

import (
	"strings"
	"testing"
)

// TestGenerateFromIRDBRowsStructTargetUsesValueAccess - "запросить(...) как
// Список<Тип>" для Тип == "структура" (не "сущность") раньше генерировало
// невалидный C: вызов несуществующего "rech_new_X" (структуры строятся
// compound literal'ом на месте, у них нет отдельной фабричной функции, см.
// package doc dbRowsRuntimeCode) и обращение к полю через "->" на значении,
// а не указателе - живая проверка (examples/бд/orm_легковесный.rech) не
// собиралась настоящим cc. Теперь item - "= {0}" (безопасно, потому что
// каждое поле присваивается явно СРАЗУ следом, ни одно не остаётся
// значимым нулевым байтом), доступ - "item.поле".
func TestGenerateFromIRDBRowsStructTargetUsesValueAccess(t *testing.T) {
	out := generateFromSrc(t, `
структура Пользователь
{
    имеет:
        Строка имя `+"`sql:name`"+`
}

функция главная
{
    переменная бд = подключиться_к_бд("sqlite::memory:")
    переменная пользователи = запросить(бд, "SELECT name FROM users") как Список<Пользователь>
    закрыть_бд(бд)
}
`)
	fn := "rech_db_rows_to_polzovatel"
	start := strings.Index(out, "static "+"rech_T_spisok_of_polzovatel"+" *"+fn)
	if start < 0 {
		t.Fatalf("не найдена функция маппинга %s, получено:\n%s", fn, out)
	}
	body := out[start:]
	if strings.Contains(body, "rech_new_polzovatel") {
		t.Fatalf("структура не должна вызывать несуществующий rech_new_X, получено:\n%s", body)
	}
	if !strings.Contains(body, "rech_T_polzovatel item = {0};") {
		t.Fatalf("ожидался \"item = {0}\" для структуры, получено:\n%s", body)
	}
	if !strings.Contains(body, "item.f_imya = ") {
		t.Fatalf("ожидался доступ к полю через \".\" (значение, не указатель), получено:\n%s", body)
	}
	if strings.Contains(body, "item->") {
		t.Fatalf("структура не должна использовать \"->\", получено:\n%s", body)
	}
}

// TestGenerateFromIRDBRowsEntityTargetUnchangedBehavior -
// TestGenerateFromIRDBRowsStructTargetUsesValueAccess выше не должен был
// сломать существующее поведение для "сущность"/"класс" (item - указатель,
// GC_MALLOC через rech_new_X, доступ через "->").
func TestGenerateFromIRDBRowsEntityTargetUnchangedBehavior(t *testing.T) {
	out := generateFromSrc(t, `
сущность Пользователь
{
    имеет:
        Строка имя `+"`sql:name`"+`
}

функция главная
{
    переменная бд = подключиться_к_бд("sqlite::memory:")
    переменная пользователи = запросить(бд, "SELECT name FROM users") как Список<Пользователь>
    закрыть_бд(бд)
}
`)
	fn := "rech_db_rows_to_polzovatel"
	start := strings.Index(out, "static "+"rech_T_spisok_of_polzovatel"+" *"+fn)
	if start < 0 {
		t.Fatalf("не найдена функция маппинга %s, получено:\n%s", fn, out)
	}
	body := out[start:]
	if !strings.Contains(body, "rech_T_polzovatel * item = rech_new_polzovatel();") {
		t.Fatalf("ожидался вызов rech_new_polzovatel() для сущности, получено:\n%s", body)
	}
	if !strings.Contains(body, "item->f_imya = ") {
		t.Fatalf("ожидался доступ к полю через \"->\" (указатель), получено:\n%s", body)
	}
}
