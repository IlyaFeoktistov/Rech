// Package version хранит версию компилятора rech.
//
// Version - переменная, а не константа: при релизной сборке она
// переопределяется флагом линковщика, чтобы не редактировать код руками
// перед каждым тегом. Пример:
//
//	go build -ldflags "-X github.com/IlyaFeoktistov/Rech/internal/version.Version=$(git describe --tags --always)" ./cmd/rech
//
// Версия самого Go-модуля (для "go get .../Rech@v0.1.0") определяется
// git-тегом на коммите, а не этим файлом - go.mod версии не хранит.
package version

var Version = "0.1.0"
