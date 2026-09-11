.PHONY: build vet test check fmt fmt-write verify install docs docs-serve clean

# Собрать компилятор в ./rech (тот же путь, что и в README.md/DEVELOPMENT.md) -
# gitignored, не коммитится.
build:
	go build -o rech ./cmd/rech

vet:
	go vet ./...

test:
	go test ./...

# Смоук-сборка КАЖДОГО examples/*.rech настоящим "rech build" (не только
# юнит-тесты) - см. package doc check_examples.sh/DEVELOPMENT.md, "Как
# тестировать".
check:
	./check_examples.sh

fmt:
	gofmt -l .

fmt-write:
	gofmt -w .

# Полный набор проверок перед коммитом - тот же порядок, что и в
# DEVELOPMENT.md, "Как тестировать".
verify: vet test check

# Только Linux/macOS (installer/unix/install.sh, POSIX sh) - Windows
# собирается отдельно через installer/windows/build.ps1 (PowerShell, не
# make).
install:
	sh installer/unix/install.sh

# Виртуальное окружение создаётся при первой сборке, если его ещё нет -
# те же команды, что и в README.md ("Документация как сайт (MkDocs)"),
# просто без ручного одноразового шага.
.venv-docs/bin/mkdocs: requirements-docs.txt
	python3 -m venv .venv-docs
	.venv-docs/bin/pip install -r requirements-docs.txt

docs: .venv-docs/bin/mkdocs
	.venv-docs/bin/mkdocs build

docs-serve: .venv-docs/bin/mkdocs
	.venv-docs/bin/mkdocs serve

clean:
	rm -f rech
	rm -rf site
