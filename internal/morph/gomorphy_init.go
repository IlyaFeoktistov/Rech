package morph

import "github.com/IlyaFeoktistov/gomorphy"

// init загружает встроенный словарь gomorphy (OpenCorpora) один раз при
// старте. Изолировано в отдельном файле, чтобы весь остальной пакет morph
// зависел только от узкого интерфейса gomorphyAnalyzer (gomorphy_source.go),
// а не от конкретной библиотеки.
func init() {
	if a, err := gomorphy.Default(); err == nil {
		sharedGomorphy = a
		sharedLexicon = a
	}
	// err игнорируется намеренно: если словарь не загрузился, sharedGomorphy
	// остаётся nil, lookupGenderViaGomorphy тихо ничего не находит - это не
	// более резкая деградация, чем для слова, которого просто нет в OpenCorpora.
}
