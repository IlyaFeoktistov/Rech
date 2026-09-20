package interp

// optionsProjectValue/setOptionsProjectValue - "опции_проекта()"/
// "установить_опции_проекта(текст)" (internal/builtins) - см. package doc
// Interp.optionsText - интерпретатор хранит то же значение, что и cbackend
// (internal/backend/cbackend/runtime/options), только в обычном Go-поле
// вместо C-глобали, RWMutex вместо CRITICAL_SECTION/pthread_mutex_t.
func (in *Interp) optionsProjectValue() Value {
	in.optionsMu.RLock()
	defer in.optionsMu.RUnlock()
	return stringValue(in.optionsText)
}

func (in *Interp) setOptionsProjectValue(text string) {
	in.optionsMu.Lock()
	defer in.optionsMu.Unlock()
	in.optionsText = text
}
