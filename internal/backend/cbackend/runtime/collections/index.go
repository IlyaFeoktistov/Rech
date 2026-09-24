package collections

import "fmt"

// ListIndexSafeGetCode - "получить_безопасно" для Список<T> (внутреннее
// имя, вызывается только сгенерированным кодом безопасной индексации "[]",
// ir.ListIndexExpr) - возвращает "?T" вместо паники при выходе за границу.
// Отдельная функция, не часть listCollectionCCode - генерируется ПОЗЖЕ, уже
// когда C-typedef обёртки nullableStructC точно готов (nullablesCode
// эмитится после genericsCode, но эта функция - после ОБОИХ, см. package
// doc generateSource, ensureIndexSafeGetHelpers). structC/nullableStructC -
// уже вычисленные вызывающей стороной (mangleType - внутренняя,
// неэкспортированная инфраструктура cbackend, этот пакет от неё не
// зависит). elemNullable - элемент сам "?T" ("Список<?Целое>"): он уже
// nullableStructC и возвращается как есть, без повторной обёртки.
func ListIndexSafeGetCode(structC, nullableStructC string, elemNullable bool) string {
	fn := structC + "_get_safe"
	return fmt.Sprintf(`static %[1]s %[2]s(%[3]s *this_, long long index) {
    if (index < 0 || index >= this_->len) {
        return %[1]s_empty();
    }
    return %[4]s;
}

`, nullableStructC, fn, structC, safeGetResult(nullableStructC, "this_->data[index]", elemNullable))
}

// safeGetResult - найденный элемент как результат "[]": обёрнутый в "?T",
// либо как есть, если он уже "?T".
func safeGetResult(nullableStructC, elem string, elemNullable bool) string {
	if elemNullable {
		return elem
	}
	return nullableStructC + "_wrap(" + elem + ")"
}

// DictIndexSafeGetCode - тот же принцип, что ListIndexSafeGetCode, но для
// Словарь<К, В> - переиспользует уже сгенерированную dictCollectionCCode
// функцию поиска слота по ключу (structC+"_find", та же, что использует
// ".получить"/".содержит") вместо дублирования логики хеш-таблицы.
func DictIndexSafeGetCode(structC, keyC, nullableStructC string, elemNullable bool) string {
	findFn := structC + "_find"
	fn := structC + "_get_safe"
	return fmt.Sprintf(`static %[1]s %[2]s(%[3]s *this_, %[4]s key) {
    long long idx = %[5]s(this_, key);
    if (idx < 0) {
        return %[1]s_empty();
    }
    return %[6]s;
}

`, nullableStructC, fn, structC, keyC, findFn, safeGetResult(nullableStructC, "this_->slots[idx].value", elemNullable))
}
