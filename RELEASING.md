# Как готовить изменения к выпуску

Этот файл - про то, как оформить готовое изменение так, чтобы оно попало в
`main` через PR, и что происходит дальше, вплоть до самого релиза.

Как менять сам компилятор (что трогать, частые ловушки, как тестировать
руками) - [DEVELOPMENT.md](DEVELOPMENT.md).

## Ветки и PR

`main` защищена - прямой `push` в неё запрещён. Единственный путь -
отдельная ветка + PR:

```sh
git checkout -b моя-фича
# ... правки ...
git push -u origin моя-фича
# затем - Pull Request в веб-интерфейсе GitHub, main <- моя-фича
```

PR принимается, только когда зелёный CI (`.github/workflows/ci.yml`) -
`go build`/`go vet`/`go test` на Linux/macOS/Windows и смоук-сборка ВСЕХ
`examples/*.rech` настоящим `rech build` (`check_examples.sh`) хотя бы на
Linux. Это тот же самый набор, что `make verify` гоняет локально - см.
DEVELOPMENT.md, "Как тестировать".

## Локальные git-хуки

Необязательно, но экономит цикл "запушил -> CI упало -> чинишь" - гоняют
ровно то же самое, только раньше, на своей машине:

```sh
make hooks-install   # один раз на клон - git config core.hooksPath .githooks
```

- **pre-commit** (`.githooks/pre-commit`) - `gofmt` на застейдженные
  `.go`-файлы + `go build`/`go vet`. Быстро (секунды), на каждый `commit`.
- **pre-push** (`.githooks/pre-push`) - `make verify` целиком (`go vet`/
  `go test`/`check_examples.sh`). Медленнее, но только на `push`, не на
  каждый `commit`.

## Что должно быть готово в самом PR

- `make verify` зелёный (или доверьтесь pre-push хуку/CI - смысл тот же).
- Новый синтаксис/builtin языка - обновлены `examples/*.rech` (см.
  DEVELOPMENT.md - новая фича без примера не считается завершённой) и, если
  меняется грамматика подсветки, - отдельный PR в
  [`rech-vscode`](https://github.com/IlyaFeoktistov/rech-vscode) (другой
  репозиторий, не забыть - см. DEVELOPMENT.md, "Расширение для редактора").
- Пользовательская документация языка (`docs/MANUAL.md`/`docs/SPEC.md`/
  `docs/REFERENCE.md`) обновлена, если меняется то, что видит программист
  НА Rech (не сам компилятор).
- `dist/`, `rech-setup.exe`, `installer/windows/rech.exe` в PR не трогаем -
  это сгенерированные артефакты установщиков (gitignored), их собирает
  `release.yml` при выпуске, а не сам PR.

## Как выпустить релиз

1. Смёрджить готовые PR в `main`.
2. Решить номер версии (semver - патч для багфиксов, минор для новых фич
   без breaking changes, мажор для breaking changes) и вписать его в
   `internal/version/version.go` (`Version = "X.Y.Z"`) - единственная точка
   правды, её же читают `installer/linux/deb/build.sh` и `installer/windows/
   build.sh` для номера пакета и для `-ldflags` в самом бинарнике.
3. Закоммитить бамп версии тем же путём (ветка + PR - см. "Ветки и PR"
   выше), смёрджить.
4. На `main`: `git tag vX.Y.Z && git push origin vX.Y.Z`.
5. Тег запускает `.github/workflows/release.yml` - собирает
   `dist/rech_X.Y.Z+git<sha>_amd64.deb` и `rech-setup.exe` (тот же путь,
   что `make installers` локально) и публикует их **черновиком** GitHub
   Release (не публичным - снять "draft" вручную, когда пора раздавать).
6. `arm64`-`.deb` пока не автоматизирован - руками, `ARCH=arm64 make deb`.
7. macOS Homebrew formula (`installer/macos/rech.rb`) ничего собирать не
   просит - `brew install --HEAD installer/macos/rech.rb` сама соберёт из
   тега/HEAD, как только репозиторий станет публичным. `release.yml`
   попутно смоук-тестирует саму formula на `macos-latest` (best-effort, не
   блокирует релиз, если упадёт - formula всё ещё непроверена вживую, см.
   README.md).
8. rech-vscode (отдельный репозиторий) - бампнуть версию его
   `package.json`, `npx @vscode/vsce package` -> `.vsix`, дальше на сайт/в
   Marketplace/Open VSX - это вне этого репозитория и вне release.yml.

Сайт для скачивания собранных артефактов (`.deb`/`.exe`/`.vsix`) -
отдельный, закрытый ресурс, публикация туда всегда ручная. Черновик GitHub
Release (шаг 5 выше) - временная площадка для самих файлов, пока сайта нет
либо вместо него.
