package semantic

import (
	"strings"
	"testing"
)

func TestAnalyzeNullableVarDeclOK(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная {
    переменная а: ?Целое = Ничто
    а = 10
    утверждать а != Ничто
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

func TestAnalyzeNullLiteralWithoutContextRejected(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная {
    переменная а = Ничто
}
`)
	if err == nil || !strings.Contains(err.Error(), "неоткуда вывести") {
		t.Fatalf("ожидалась ошибка про отсутствие контекста типа у \"Ничто\", получено: %v", err)
	}
}

func TestAnalyzeNullableUseWithoutNarrowingRejected(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная {
    переменная а: ?Целое = Ничто
    вывести а + 1
}
`)
	if err == nil || !strings.Contains(err.Error(), "не поддерживается") {
		t.Fatalf("ожидалась ошибка компиляции при использовании nullable-значения без сужения, получено: %v", err)
	}
}

func TestAnalyzeNullableNarrowingInIfAllowsUse(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная {
    переменная а: ?Целое = Ничто
    а = 5
    если а != Ничто {
        вывести а + 1
    }
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

func TestAnalyzeNullableNarrowingInElseBranch(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная {
    переменная а: ?Целое = Ничто
    а = 5
    если а == Ничто {
        вывести 0
    } иначе {
        вывести а + 1
    }
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

// TestAnalyzeNullableNarrowingInWhileAllowsUse - "пока а != Ничто {...}"
// сужает "а" внутри тела - тем же принципом, что и "если" (Шаг 97/98,
// docs/ROADMAP.md - раньше не поддержано, найденное ограничение снято):
// условие проверяется заново перед каждым проходом, поэтому "а != Ничто"
// доказанно истинно в начале ЛЮБОГО прохода, не только первого.
func TestAnalyzeNullableNarrowingInWhileAllowsUse(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная {
    переменная а: ?Целое = Ничто
    а = 5
    пока а != Ничто {
        вывести а + 1
        а = Ничто
    }
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

// TestAnalyzeNullableNarrowingDoesNotSurviveAfterWhile - сужение внутри
// "пока" НЕ переживает сам цикл (после выхода "а" вполне может быть Ничто -
// именно оно и остановило цикл) - использование "а" как ненулевого СРАЗУ
// после "пока", без повторной проверки, должно остаться ошибкой компиляции,
// иначе фикс Шага 97/98 был бы несостоятельным (доказывал бы больше, чем
// реально гарантировано).
func TestAnalyzeNullableNarrowingDoesNotSurviveAfterWhile(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная {
    переменная а: ?Целое = Ничто
    а = 5
    пока а != Ничто {
        а = Ничто
    }
    вывести а + 1
}
`)
	if err == nil || !strings.Contains(err.Error(), "не поддерживается") {
		t.Fatalf("ожидалась ошибка компиляции при использовании \"а\" без сужения после \"пока\", получено: %v", err)
	}
}

// TestAnalyzeNullableNarrowingAppliesToCallValueExpr - вызов ЗНАЧЕНИЯ типа
// "Вызываемый" (не обычной функции по имени) должен разворачивать доказанно
// непустой nullable-аргумент так же, как обычный вызов (checkCallArgs) -
// раньше analyzeCallValueExpr использовал analyzeExpr напрямую вместо
// coerceExpr и терял narrowing (isAssignable отклонял ?Строка на месте
// Строка), хотя тот же аргумент у обычного вызова функции по имени уже
// разворачивался - найдено при написании examples/проекты/http_сервер
// (middleware, вызывающее параметр "следующий" с уже проверенным на Ничто
// именем пользователя).
func TestAnalyzeNullableNarrowingAppliesToCallValueExpr(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная {
    переменная ф: Вызываемый(Строка): Целое = &длина
    переменная а: ?Строка = Ничто
    а = "привет"
    если а != Ничто {
        вывести ф(а)
    }
}
функция длина(текст: Строка): Целое { вернуть 0 }
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

func TestAnalyzeNullCheckOnNonNullableRejected(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная {
    переменная а = 5
    если а == Ничто {
        вывести 0
    }
}
`)
	if err == nil || !strings.Contains(err.Error(), "nullable-типа") {
		t.Fatalf("ожидалась ошибка сравнения не-nullable значения с \"Ничто\", получено: %v", err)
	}
}

func TestAnalyzeNullCoalesceUnwraps(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная {
    переменная а: ?Целое = Ничто
    переменная б = а ?? 0
    вывести б + 1
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

func TestAnalyzeNullCoalesceAssignRequiresNullable(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная {
    переменная а = 5
    а ?= 10
}
`)
	if err == nil || !strings.Contains(err.Error(), "?=") {
		t.Fatalf("ожидалась ошибка \"?=\" на не-nullable переменной, получено: %v", err)
	}
}

func TestAnalyzeExplicitCastUnwrapsNullable(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная {
    переменная а: ?Целое = Ничто
    вывести (Целое) а
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

// TestAnalyzeExplicitCastToOtherTypeUnwrapsNarrowedNullable - НАЙДЕННОЕ И
// ИСПРАВЛЕННОЕ ограничение (docs/ROADMAP.md, Шаг 41/44): "(Целое) а" (а -
// ?Строка, уже сужена "!= Ничто") - каст сразу В ДРУГОЙ тип, не просто
// разворот "?Строка" в "Строка" (тот уже работал безусловно, см.
// TestAnalyzeExplicitCastUnwrapsNullable выше, даже БЕЗ сужения - рантайм-
// паника на "Ничто"). Раньше analyzeCastExpr не звал maybeUnwrapNarrowed
// вовсе - "to" (Целое) не совпадал с внутренним типом "from" (Строка)
// буквально, allowedCasts не знает про "?Строка" - "нельзя привести
// ?Строка к Целое".
func TestAnalyzeExplicitCastToOtherTypeUnwrapsNarrowedNullable(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная {
    переменная а: ?Строка = Ничто
    а = "42"
    если а != Ничто {
        вывести (Целое) а
    }
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

// TestAnalyzeExplicitCastUnwrapsNullableOpaqueType - явный каст разворачивает
// "?Сокет" (nullable-обёртка непрозрачного KindSocket, возвращаемая
// "принять") тем же путём, что и TestAnalyzeExplicitCastUnwrapsNullable
// выше для примитива - до этой правки "(Сокет)" не входил в
// parser.primitiveCastTypes, синтаксис вообще не разбирался как каст.
func TestAnalyzeExplicitCastUnwrapsNullableOpaqueType(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная {
    переменная слушатель = сетевой_узел("tcp", "", 18080)
    переменная соединение = принять(слушатель)
    переменная развёрнутое = (Сокет) соединение
    закрыть_сокет(развёрнутое)
    закрыть_сокет(слушатель)
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

// TestAnalyzeNullableNarrowedAfterEarlyReturnOnEqualNichto - НАЙДЕННОЕ И
// ИСПРАВЛЕННОЕ ограничение (docs/ROADMAP.md, Шаг 41/44): "если а == Ничто {
// вернуть }" (ранний выход, БЕЗ явного "иначе") должно доказывать
// непустоту "а" для ОСТАЛЬНОГО тела функции, ровно как доказало бы "если а
// == Ничто {...} иначе {...использовать а...}" - Then расходится
// (blockDiverges), значит единственный путь ДОСТИЧЬ кода после "если" -
// тот, где условие было ложно.
func TestAnalyzeNullableNarrowedAfterEarlyReturnOnEqualNichto(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная {
    переменная а: ?Целое = Ничто
    а = 5
    если а == Ничто {
        вернуть
    }
    вывести а + 1
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

// TestAnalyzeNullableNotNarrowedAfterNonDivergingIf - контрольный тест: без
// гарантированного расхождения ("вывести", не "вернуть"/"выбросить" внутри
// Then) сужение по-прежнему НЕ применяется дальше по коду - код после
// "если" достижим и ветвью, где "а" остался Ничто.
func TestAnalyzeNullableNotNarrowedAfterNonDivergingIf(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная {
    переменная а: ?Целое = Ничто
    а = 5
    если а == Ничто {
        вывести 0
    }
    вывести а + 1
}
`)
	if err == nil || !strings.Contains(err.Error(), "?Целое") {
		t.Fatalf("ожидалась ошибка использования ?Целое без сужения, получено: %v", err)
	}
}

// TestAnalyzeSerializeJSONUnwrapsNarrowedNullable - НАЙДЕННОЕ И
// ИСПРАВЛЕННОЕ ограничение (docs/ROADMAP.md, Шаг 41/44): "сериализовать
// JSON x" (x - ?Точка, уже сужена "!= Ничто") раньше не разворачивала
// nullable-обёртку сама (structCompatible не принимает "?Тип" вовсе), хотя
// употребление того же значения любым другим способом уже разворачивало
// его.
func TestAnalyzeSerializeJSONUnwrapsNarrowedNullable(t *testing.T) {
	_, err := AnalyzeSource(t, `
сущность Точка {
    имеет:
        x: Целое
}
функция главная {
    переменная т: ?Точка = Ничто
    т = новая Точка()
    если т != Ничто {
        вывести сериализовать JSON т
    }
}
`)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
}

func TestAnalyzeNullableListElementRejected(t *testing.T) {
	_, err := AnalyzeSource(t, `
функция главная {
    переменная а: Список<?Целое> = Ничто
}
`)
	if err == nil || !strings.Contains(err.Error(), "nullable-типы") {
		t.Fatalf("ожидалась ошибка компиляции для \"Список<?Целое>\", получено: %v", err)
	}
}
