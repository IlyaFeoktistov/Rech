package interp

// dictPairs - хранилище "Словарь<К, В>" с порядком вставки ключей (тот
// же порядок, что у cbackend - collections.DictCollectionCode): цикл
// "для к, з в словаре", показать, отладчик обходят записи в этом порядке.
// Value как ключ Go-map сравним напрямую (==) для примитивных типов ключа
// Словаря (Целое/Строка/Байт/Символ/Логическое, см. internal/semantic.
// dictKeyKinds) - указательные поля Value (Obj/File/Listener/Conn/Task/
// Callable) у них всегда nil, значит сравнение всей структуры корректно
// отражает равенство самого значения ключа.
type dictPairs struct {
	pos  map[Value]int
	keys []Value
	vals []Value
}

func newDictPairs() *dictPairs {
	return &dictPairs{pos: map[Value]int{}}
}

func (d *dictPairs) get(key Value) (Value, bool) {
	if d == nil {
		return Value{}, false
	}
	i, ok := d.pos[key]
	if !ok {
		return Value{}, false
	}
	return d.vals[i], true
}

func (d *dictPairs) has(key Value) bool {
	_, ok := d.get(key)
	return ok
}

// set вставляет новый ключ в конец порядка или перезаписывает значение
// существующего, не меняя его места.
func (d *dictPairs) set(key, val Value) {
	if i, ok := d.pos[key]; ok {
		d.vals[i] = val
		return
	}
	d.pos[key] = len(d.keys)
	d.keys = append(d.keys, key)
	d.vals = append(d.vals, val)
}

// del удаляет ключ, сохраняя порядок остальных (сдвиг - O(длина)).
func (d *dictPairs) del(key Value) {
	i, ok := d.pos[key]
	if !ok {
		return
	}
	delete(d.pos, key)
	d.keys = append(d.keys[:i], d.keys[i+1:]...)
	d.vals = append(d.vals[:i], d.vals[i+1:]...)
	for j := i; j < len(d.keys); j++ {
		d.pos[d.keys[j]] = j
	}
}

func (d *dictPairs) size() int {
	if d == nil {
		return 0
	}
	return len(d.keys)
}
