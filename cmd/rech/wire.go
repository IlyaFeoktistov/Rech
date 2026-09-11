//go:build wireinject

// Файл-источник для google/wire. Он не компилируется в обычную сборку
// (тег wireinject), а используется генератором `wire` для создания
// wire_gen.go. Перегенерировать после изменения набора провайдеров:
//
//	go run github.com/google/wire/cmd/wire ./cmd/rech
package main

import (
	"github.com/google/wire"

	"github.com/IlyaFeoktistov/Rech/internal/compiler"
	"github.com/IlyaFeoktistov/Rech/internal/toolchain"
)

// InitializeCompiler собирает *compiler.Compiler из стандартных провайдеров
// v0.1: Frontend (лексер+парсер), Backend (транспиляция в C) и Toolchain
// (системный cc/clang/gcc). Когда появятся альтернативные backend'ы или
// toolchain'ы, здесь появятся другие наборы провайдеров.
func InitializeCompiler() *compiler.Compiler {
	wire.Build(
		compiler.NewFrontend,
		compiler.NewBackend,
		toolchain.NewCC,
		compiler.NewCompiler,
	)
	return nil
}
