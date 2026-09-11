# Rech (Речь)

Язык программирования, в котором русская грамматика, морфология и
словообразование становятся частью синтаксиса и семантики.

Сейчас в репозитории - v0.1: минимальный, но реально работающий
пайплайн `Rech -> AST -> C -> нативный бинарник`. Руководство по языку
(введение, философия, весь синтаксис по порядку) -
[docs/MANUAL.md](docs/MANUAL.md); формальная спецификация того, что
компилятор принимает прямо сейчас - [docs/SPEC.md](docs/SPEC.md); черновик
синтаксиса для того, что запланировано, - [docs/GRAMMAR.md](docs/GRAMMAR.md);
дизайн морфологического движка - [docs/MORPHOLOGY.md](docs/MORPHOLOGY.md).

Компилятор встраивает словарные данные OpenCorpora (через gomorphy) -
атрибуция и условия лицензии CC BY-SA 4.0 - [NOTICE.md](NOTICE.md).

## Пример

`examples/основы/привет.rech`:

```
функция главная
{
    вывести "Привет, мир!"
}
```

Больше примеров, по темам, - в подпапках `examples/` (`основы/`,
`функции/`, `сущности/`).

## Установка

Самый быстрый способ получить рабочую команду `rech` в `PATH`:

- **Linux/macOS** - собирает `rech` из исходников этого репозитория (нужен
  Go), проверяет C-компилятор и ВСЕ опциональные библиотеки (Boehm GC,
  клиенты БД, OpenSSL, nghttp2, libpng, libjpeg) и подсказывает команду
  установки для каждой отсутствующей, добавляет каталог с бинарником в
  `PATH`:
  ```sh
  sh installer/unix/install.sh                # в ~/.local/bin
  sh installer/unix/install.sh --prefix=/usr/local/bin   # системно (нужен sudo мимо самого скрипта)
  ```
- **Windows** - `installer/windows/setup.nsi` (нужны Go и
  [NSIS](https://nsis.sourceforge.io/) - `winget install NSIS.NSIS`):
  ```powershell
  powershell -File installer\windows\build.ps1   # соберёт rech.exe и rech-setup.exe
  .\rech-setup.exe                               # обычный установщик, без прав admin
  ```
  Устанавливает в `%LOCALAPPDATA%\Rech\bin`, добавляет его в `PATH`
  пользователя, предупреждает (не прерывая установку) про отсутствующий
  C-компилятор и про ту же полную библиотеку опциональных зависимостей,
  что и Unix-версия (пробует скомпилировать/слинковать крошечную программу
  уже найденным компилятором). Базовая часть (C-компилятор, PATH,
  деинсталлятор) собрана и проверена вручную на настоящем Windows; пробы
  библиотек (Boehm GC/OpenSSL/nghttp2/libpng/libjpeg/клиенты БД) - НЕТ,
  только скомпилированы через `makensis` и проверены логикой, реального
  Windows под рукой не было.

Ни один из установщиков не тянет за собой C-компилятор/внешние библиотеки
сам - только проверяет их и подсказывает, что поставить (см. "Зависимости
для сборки" ниже) - это внешние зависимости самой ОС, отдельные от Rech.

Собрать и установить вручную (то же самое, что делают скрипты выше, но
руками - см. "Установка бинарника `rech` глобально" ниже) или через `go
run ./cmd/rech ...` без установки вовсе - тоже можно, см. дальше.

## Зависимости для сборки

| Что | Зачем | Нужно всегда? |
| --- | --- | --- |
| [Go](https://go.dev/dl/) 1.25+ | собрать сам компилятор `rech` (этот репозиторий - на Go) | да, один раз при установке/пересборке `rech` |
| C-компилятор (`cc`/`clang`/`gcc` в `PATH`, либо `$CC`) | `rech build`/`rech run` транспилирует Rech в C и вызывает системный компилятор для финальной сборки бинарника | да, для КАЖДОЙ сборки Rech-программы |
| Boehm GC (`libgc`) | сборщик мусора для сгенерированного C - нужен любой программе хотя бы с одной ссылочной `сущность`/`класс` (раздел "Сущности", `docs/SPEC.md`) | нет - только программе с `сущность`/`класс`; `структура` и программы совсем без сущностей не требуют |
| `git` в `PATH` | `rech install`/`rech build`/`rech run` скачивают внешние зависимости проекта (`.rechproj`, секция "зависимости:") через `git clone` | нет - только у проекта с непустой секцией "зависимости:" |
| Клиентские библиотеки БД (`libsqlite3`/`libpq`/`libmysqlclient`, все три сразу) | единый `БД` ("подключиться_к_бд"/"запросить"/"выполнить"/"закрыть_бд", `docs/SPEC.md`, "БД") - какая из трёх реально понадобится, решает схема строки подключения в рантайме, компилятор линкует все три безусловно | нет - только программе, использующей `БД` |
| OpenSSL (`libssl`/`libcrypto`) | TLS (`слушать_tcp_защищённо`, HTTPS-клиент) и симметричное шифрование (`зашифровать`/`расшифровать`, AES-256-GCM), а также асимметричная подпись (`сгенерировать_приватный_ключ`/`подписать`/`проверить_подпись`) | нет - только программе, использующей одну из этих функций |
| nghttp2 | HTTP/2- и gRPC-транспорт (`http2_*`/`grpc_*`/`слушать_grpcs`) | нет - только программе, использующей одну из этих функций |
| libpng | `декодировать_изображение`/`закодировать_изображение` с `ФорматИзображения.PNG` | нет - только программе, работающей с PNG |
| libjpeg | `декодировать_изображение`/`закодировать_изображение` с `ФорматИзображения.JPEG` | нет - только программе, работающей с JPEG |

Сам компилятор `rech` (Go-бинарник) не требует Boehm GC - только
программы, которые ОН собирает через `cc`/`clang`/`gcc`. Установка `libgc`
по ОС/пакетному менеджеру:

- Debian/Ubuntu - `apt install libgc-dev`.
- Fedora - `dnf install gc-devel`.
- macOS (Homebrew) - `brew install bdw-gc`.
- Windows с vcpkg - `vcpkg install bdwgc` (не проверялось напрямую - vcpkg
  не был установлен на тестовой машине).
- **Windows с MinGW-w64 (проверено, например WinLibs)** - vcpkg не нужен,
  собрать `bdwgc` из исходников через уже имеющиеся `cmake`/`mingw32-make`
  из состава дистрибутива:
  ```
  curl -LO https://github.com/ivmai/bdwgc/releases/download/v8.2.6/gc-8.2.6.tar.gz
  tar xzf gc-8.2.6.tar.gz && cd gc-8.2.6 && mkdir build && cd build
  cmake -G "MinGW Makefiles" -DCMAKE_BUILD_TYPE=Release -Denable_cplusplus=OFF -DBUILD_SHARED_LIBS=OFF ..
  mingw32-make
  ```
  Затем скопировать `include/*.h` (и `include/extra/`) в `<mingw64>/include`,
  `build/libgc.a` в `<mingw64>/lib` - тот же `<mingw64>`, где лежит сам
  `gcc.exe` (стандартные пути поиска компилятора, без правки `CC`/флагов).

Установка клиентских библиотек БД по ОС/пакетному менеджеру (нужны только
их `-dev`-пакеты - именно они дают линковщику `-lsqlite3`/`-lpq`/
`-lmysqlclient`, версия самой клиентской библиотеки не обязана совпадать с
версией СУБД, к которой подключается программа):

- Debian/Ubuntu - `apt install libsqlite3-dev libpq-dev libmysqlclient-dev`
  (или `default-libmysqlclient-dev`, если пакет с точным именем недоступен).
- Fedora - `dnf install sqlite-devel libpq-devel mysql-devel` (или
  `community-mysql-devel`/`mariadb-connector-c-devel` - см. свой репозиторий).
- macOS (Homebrew) - `brew install sqlite postgresql mysql-client`.

Установка OpenSSL/nghttp2/libpng/libjpeg по ОС/пакетному менеджеру (тоже
нужны `-dev`-пакеты, не только рантайм-библиотеки):

- Debian/Ubuntu - `apt install libssl-dev libnghttp2-dev libpng-dev libjpeg-dev`.
- Fedora - `dnf install openssl-devel libnghttp2-devel libpng-devel libjpeg-turbo-devel`.
- macOS (Homebrew) - `brew install openssl nghttp2 libpng jpeg`.
- Windows с vcpkg - `vcpkg install openssl nghttp2 libpng libjpeg-turbo` (не
  проверялось напрямую, тем же принципом, что и `bdwgc` выше).
- Windows с MinGW-w64 через MSYS2 (не проверялось напрямую) -
  `pacman -S mingw-w64-x86_64-openssl mingw-w64-x86_64-libnghttp2 mingw-w64-x86_64-libpng mingw-w64-x86_64-libjpeg-turbo`.

## Сборка и запуск

Без установки - собрать и сразу запустить пример через `go run`:

```sh
go run ./cmd/rech run examples/основы/привет.rech
```

Вывод:

```
Привет, мир!
```

Отдельно собрать бинарник без запуска:

```sh
go run ./cmd/rech build examples/основы/привет.rech
```

Компилятор соберёт бинарник `examples/основы/привет`. Промежуточный
`.c`-файл по умолчанию удаляется после успешной сборки - добавьте
`--keep-c`, если хотите посмотреть, во что компилируется программа:

```sh
go run ./cmd/rech build --keep-c examples/основы/привет.rech
```

### Файл проекта (`.rechproj`)

`rech init [имя]` создаёт заготовку проекта - `исходники/главный.rech` и
манифест `<имя>.rechproj` (имя по умолчанию - имя текущего каталога; пути в
манифесте - на русском, как и остальной язык):

```sh
rech init hello
```

```
проект:
  имя: hello
  версия: 0.1.0

файлы:
  исходники: исходники/**/*.rech
  главный: исходники/главный.rech
  выход: сборка

сборка:
  режим: отладка
  платформа: текущая
```

`rech build`/`rech run` без пути ищут единственный `*.rechproj` в текущем
каталоге, либо принимают путь к нему явно (`rech build hello.rechproj`) -
собранный бинарник кладётся в каталог из поля `выход`, под именем `имя`.
`режим: отладка` соответствует флагу `--keep-c`, `режим: релиз` - его
отсутствию; `платформа` пока поддерживает только значение `текущая`
(кросс-компиляция не реализована). Поле `исходники` - для будущих шагов и
внешних инструментов вроде подсветки в VS Code, сама сборка сейчас идёт от
`главный` через `использовать` (docs/SPEC.md, раздел 10).

### Отладка (`rech debug`)

`rech debug` - сервер [Debug Adapter Protocol](https://microsoft.github.io/debug-adapter-protocol/)
по stdin/stdout: точки останова, continue/шаг через/шаг внутрь/шаг наружу,
стек вызовов, просмотр локальных переменных/полей сущностей, вычисление
произвольных выражений в остановленном кадре. Исполняет программу через
интерпретатор (`internal/interp`), не через `gdb`/`lldb` на скомпилированной
программе - без DWARF/`#line`-директив, одинаково работает на всех
платформах. Не предназначен для запуска вручную из терминала - им
пользуется расширение [`rech-vscode`](https://github.com/IlyaFeoktistov/rech-vscode)
(кнопка "Rech: отладить" или `launch.json`).

### Установка бинарника `rech` глобально (вручную)

`go run ./cmd/rech ...` каждый раз пересобирает компилятор из исходников -
удобно при разработке самого компилятора, но не для повседневного `rech
build`/`run` из любого каталога. Установщики из раздела "Установка" выше
делают то же самое автоматически - этот раздел - то же самое руками, если
установщик по какой-то причине не подходит. Чтобы получить обычную команду
`rech` в `PATH`:

```sh
go build -o rech ./cmd/rech
```

Дальше переместите получившийся бинарник `rech` (или `rech.exe` на Windows) в
каталог, уже входящий в `PATH`, либо добавьте каталог с ним в `PATH`:

```sh
# Linux/macOS - вариант "положить в системный каталог" (нужны права root):
sudo mv rech /usr/local/bin/rech

# либо вариант "без sudo" - положить в личный каталог и один раз добавить
# его в PATH (в ~/.bashrc/~/.zshrc):
mkdir -p ~/bin && mv rech ~/bin/
echo 'export PATH="$HOME/bin:$PATH"' >> ~/.bashrc   # или ~/.zshrc
```

```powershell
# Windows (PowerShell) - положить рядом с другими пользовательскими
# инструментами и один раз добавить каталог в PATH:
Move-Item rech.exe "$HOME\bin\rech.exe"
[Environment]::SetEnvironmentVariable("Path", "$env:Path;$HOME\bin", "User")
```

После этого (открыв новый терминал) `rech` работает из любого каталога:

```sh
rech version
rech build examples/основы/привет.rech
```

Имена `.rech`-файлов - только строчные буквы и `_` (`имя_файла.rech`, без
заглавных букв) и обязательно в форме Unicode NFC - подробности и мотивация
в [docs/SPEC.md](docs/SPEC.md#1-кодировка-и-имена-файлов). Если репозиторий
только что клонирован, разово выполните `git config core.quotepath false`
(без `--global`) - иначе `git status` печатает кириллические пути
восьмеричными escape-кодами.

## Документация как сайт (MkDocs)

`docs/MANUAL.md`/`docs/SPEC.md`/`docs/GRAMMAR.md`/`docs/MORPHOLOGY.md`
читаются и как обычный markdown на GitHub, но их же можно собрать в
навигируемый сайт (боковое меню по главам, поиск, тёмная/светлая тема) -
[MkDocs](https://www.mkdocs.org/) с темой
[Material](https://squidfunk.github.io/mkdocs-material/).

Нужен Python 3 - зависимость только у этого сайта, самого компилятора
`rech` не касается. Одноразовая настройка (виртуальное окружение, чтобы не
трогать системный Python):

```sh
python3 -m venv .venv-docs
.venv-docs/bin/pip install -r requirements-docs.txt
```

Дальше, из корня репозитория:

```sh
# Локальный live-просмотр с автообновлением при правке .md (http://127.0.0.1:8000):
.venv-docs/bin/mkdocs serve

# Собрать статический сайт в site/ (можно открыть site/index.html
# напрямую или выложить на любой статический хостинг):
.venv-docs/bin/mkdocs build
```

`site/` и `.venv-docs/` - в `.gitignore`, не коммитятся.

## Структура проекта

Подробный гид "как вносить изменения в компилятор" (что в каком порядке
трогать, частые ловушки, как тестировать) - [DEVELOPMENT.md](DEVELOPMENT.md).
Здесь - только карта каталогов.

Пайплайн компиляции (`cmd/rech` -> `internal/lexer` -> `internal/morph` ->
`internal/parser`/`internal/ast` -> `internal/module` -> `internal/semantic`/
`internal/ir` -> `internal/backend/cbackend` **и** `internal/interp` -
оба должны давать одинаковый наблюдаемый результат на любой программе):

```
cmd/rech/                  - CLI (build/run/test/debug/lsp/install/init/version)
internal/lexer/            - лексер (кириллица в идентификаторах и ключевых словах)
internal/morph/            - морфологический движок (склонение ключевых слов, OpenCorpora/gomorphy)
internal/parser/           - парсер (токены -> AST)
internal/ast/               - синтаксическое дерево, не зависящее от backend'а
internal/module/           - "использовать ... из" - связывание многофайловых проектов в один AST
internal/semantic/         - типизация/видимость/проверки, сборка internal/ir.Program
internal/ir/                - промежуточное представление, общее для обоих backend'ов
internal/backend/cbackend/ - генерация C из IR (единственный путь к нативному бинарнику)
internal/interp/           - дерево-обходящий интерпретатор (rech test/rech debug/rech lsp)
internal/builtins/         - реестр встроенных функций (имена/сигнатуры, общий для семантики/обоих backend'ов)
internal/rtmsg/            - тексты рантайм-сообщений/ошибок, общие между cbackend и interp
internal/dbscheme/         - разбор схемы строки подключения "БД" (sqlite/postgres/mysql)
internal/structtag/        - разбор тегов полей (`` `json:...` ``)
internal/compiler/         - интерфейсы Frontend/Backend/Toolchain, связывание пайплайна
internal/toolchain/        - вызов системного cc/clang/gcc, условная линковка библиотек
internal/project/          - разбор манифеста `.rechproj`
internal/pkgfetch/         - скачивание внешних зависимостей проекта (секция "зависимости:", `rech install`)
internal/testrunner/       - `rech test`
internal/format/           - канонический отступ исходника (`rech fmt`)
internal/protoimport/      - импорт ".proto"-контракта в Rech-сущности с тегами `protobuf:N` (`rech proto`)
internal/lsp/              - language server (`rech lsp`) - автодополнение/hover/definition
internal/dap/               - типы Debug Adapter Protocol
internal/debugadapter/     - сервер DAP (`rech debug`) поверх internal/interp
internal/rpcframe/         - общий транспортный кадр LSP/DAP (Content-Length + JSON)
internal/debug/            - дамп AST/IR для отладки самого компилятора
internal/version/          - версия компилятора (см. `rech version`)
installer/unix/            - install.sh (Linux/macOS)
installer/windows/         - setup.nsi/build.ps1 (Windows, без прав admin)
examples/                  - примеры программ на Rech, по тематическим подпапкам
docs/MANUAL.md             - руководство по языку для пользователя (учебник)
docs/SPEC.md               - формальная спецификация того, что реализовано прямо сейчас
docs/GRAMMAR.md            - черновик синтаксиса для того, что запланировано
docs/MORPHOLOGY.md         - дизайн морфологического движка
```

## Архитектура

```
RECH -> Lexer -> Morphology -> Parser -> AST -> Module linking -> Semantic analysis -> IR
                                                                                          |
                                                                Rech-программа целиком (все "использовать")
                                                                                          |
                                                                   +----------------------+----------------------+
                                                                   |                                             |
                                                              C backend                                   Interpreter
                                                          (internal/backend/cbackend)                  (internal/interp)
                                                                   |                                             |
                                                              GCC/Clang/MSVC                    rech test / rech debug (DAP) / rech lsp
                                                                   |
                                                            Native executable
```

AST и IR не зависят от C - C backend первый (и единственный путь к
нативному бинарнику), но не единственный ПОТРЕБИТЕЛЬ IR: интерпретатор -
второй, полноправный потребитель того же IR, не урезанная копия
компилятора - он приводит в действие `rech test`, отладчик (`rech debug`,
DAP) и language server (`rech lsp`). Оба backend'а обязаны давать
идентичный наблюдаемый результат на любой программе - это стандартное
требование к каждой новой фиче языка (подробнее - [DEVELOPMENT.md](DEVELOPMENT.md)).
