# Rech (Речь)

Язык программирования, в котором русская грамматика, морфология и
словообразование становятся частью синтаксиса и семантики.

Сейчас в репозитории - v0.1: минимальный, но реально работающий
пайплайн `Rech -> AST -> C -> нативный бинарник`. Полный план развития - в
[docs/ROADMAP.md](docs/ROADMAP.md).

## Пример

`examples/hello.rech`:

```
функция главная
{
    вывести "Привет, мир!"
}
```

## Сборка и запуск

Нужен установленный системный C-компилятор (`cc`, `clang` или `gcc` в `PATH`,
либо переменная окружения `CC`).

```sh
go run ./cmd/rech run examples/hello.rech
```

Вывод:

```
Привет, мир!
```

Отдельно собрать бинарник без запуска:

```sh
go run ./cmd/rech build examples/hello.rech
```

Компилятор сгенерирует `examples/hello.c` и соберёт `examples/hello`.

## Структура проекта

```
cmd/rech/                  - CLI (build/run), сборка зависимостей через google/wire
internal/lexer/            - лексер (кириллица в идентификаторах и ключевых словах)
internal/ast/              - синтаксическое дерево, не зависящее от backend'а
internal/parser/           - парсер v0.1
internal/backend/cbackend/ - генерация C из AST
internal/compiler/         - интерфейсы Frontend/Backend/Toolchain и их связывание
internal/toolchain/        - вызов системного cc/clang/gcc
examples/                  - примеры программ на Rech
docs/ROADMAP.md            - план развития языка по шагам
```

## Архитектура

```
RECH -> Lexer -> Morphology -> Parser -> AST -> Semantic analysis -> IR
                                                                       |
                                                        +--------------+--------------+
                                                        |                             |
                                                    C backend                   Interpreter (позже VM)
                                                        |
                                                   GCC/Clang
                                                        |
                                                Native executable
```

AST и будущий IR не зависят от C - C backend первый, но не единственный.
