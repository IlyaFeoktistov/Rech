.PHONY: build vet test check fmt fmt-write verify install deb windows-installer installers hooks-install docs docs-serve docs-image docs-image-push clean

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

# Только Linux/macOS (installer/unix/install.sh, POSIX sh) - собирает rech
# из исходников на ЭТОЙ машине и ПРОВЕРЯЕТ (не ставит) внешние библиотеки.
# Для настоящего пакета, который сам их ставит, - см. "deb" ниже.
install:
	sh installer/unix/install.sh

# Настоящий .deb (не только собирает rech, но и через Depends заставляет
# apt/dpkg самому поставить компилятор и ВСЕ опциональные библиотеки) -
# installer/linux/deb/build.sh, ARCH=/VERSION= переопределяемы тем же
# путём, что и переменные окружения самого build.sh.
deb:
	sh installer/linux/deb/build.sh

# Кросс-сборка rech-setup.exe С этой (Linux/macOS/WSL) машины - makensis
# кросс-платформенный, Windows под рукой не нужен. installer/windows/
# build.ps1 остаётся для запуска НА настоящем Windows (PowerShell) - этот
# путь его не заменяет.
windows-installer:
	sh installer/windows/build.sh

# Полный набор установщиков одной командой - macOS Homebrew formula
# (installer/macos/rech.rb) не участвует - это статический файл, не
# собирается.
installers: deb windows-installer

# Локальные git-хуки (.githooks/pre-commit, .githooks/pre-push) - один раз
# на клон, см. RELEASING.md, "Локальные git-хуки".
hooks-install:
	git config core.hooksPath .githooks

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

# То же самое (mkdocs build), но упаковано в Docker-образ с nginx поверх -
# тем же тегом (коммит, не "latest" - см. kubernetes/README.md про
# развёртывание), что пушит .github/workflows/docs.yml и монтирует
# rechlang-site/kubernetes под "/docs" (см. их README).
DOCS_IMAGE := ghcr.io/ilyafeoktistov/rech-docs
DOCS_VERSION := $(shell git rev-parse HEAD)

docs-image:
	docker build -f docs.Dockerfile -t $(DOCS_IMAGE):$(DOCS_VERSION) .

docs-image-push: docs-image
	docker push $(DOCS_IMAGE):$(DOCS_VERSION)

clean:
	rm -f rech
	rm -rf site dist
	rm -f rech-setup.exe installer/windows/rech.exe
