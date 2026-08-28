package cbackend

import "fmt"

// listIndexSafeGetCCode - "получить_безопасно" для Список<T> (внутреннее
// имя, вызывается только сгенерированным кодом безопасной индексации "[]",
// ir.ListIndexExpr) - возвращает "?T" вместо паники при выходе за границу.
// Отдельная функция, не часть listCollectionCCode - генерируется ПОЗЖЕ, уже
// когда C-typedef обёртки nullableStructName точно готов (nullablesCode
// эмитится после genericsCode, но эта функция - после ОБОИХ, см. package
// doc generateSource, ensureIndexSafeGetHelpers).
func listIndexSafeGetCCode(name, nullableStructName string) string {
	structName := mangleType(name)
	fn := structName + "_get_safe"
	return fmt.Sprintf(`static %[1]s %[2]s(%[3]s *this_, long long index) {
    if (index < 0 || index >= this_->len) {
        return %[1]s_empty();
    }
    return %[1]s_wrap(this_->data[index]);
}

`, nullableStructName, fn, structName)
}

// dictIndexSafeGetCCode - тот же принцип, что listIndexSafeGetCCode, но
// для Словарь<К, В> - переиспользует уже сгенерированную dictCollectionCCode
// функцию поиска слота по ключу (mangleType(name)+"_find", та же, что
// использует ".получить"/".содержит") вместо дублирования логики
// хеш-таблицы.
func dictIndexSafeGetCCode(name, keyC, nullableStructName string) string {
	structName := mangleType(name)
	findFn := structName + "_find"
	fn := structName + "_get_safe"
	return fmt.Sprintf(`static %[1]s %[2]s(%[3]s *this_, %[4]s key) {
    long long idx = %[5]s(this_, key);
    if (idx < 0) {
        return %[1]s_empty();
    }
    return %[1]s_wrap(this_->slots[idx].value);
}

`, nullableStructName, fn, structName, keyC, findFn)
}
