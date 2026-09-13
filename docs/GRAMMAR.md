# Грамматика Rech (EBNF)

Формальная грамматика строгого (context-free) синтаксиса языка - того, что
компилятор принимает прямо сейчас, без пометок "планируется". Семантика
каждой конструкции, полные примеры и построчные пояснения -
[docs/SPEC.md](SPEC.md); учебное изложение с введением - [docs/MANUAL.md](MANUAL.md).

Естественный (падежный) синтаксис - `оформить заказ клиенту` вместо
`оформить(заказ, клиент)` - здесь намеренно не описан: это не отдельная
ветка context-free грамматики, а семантическое сопоставление словоформы с
параметром по падежу поверх уже разобранного `call_expr`/`method_call_expr`
ниже (то же для естественного вызова метода - раздел 6 - и деепричастного
вызова `gerund_stmt` ниже). Правила падежного сопоставления и согласования -
[docs/MORPHOLOGY.md](MORPHOLOGY.md). Модификаторы (`приватный`/`статический`/
...) описаны здесь одним общим `modifier`, хотя не любое сочетание и не в
любой позиции на самом деле допустимо (например, `статический`/`абстрактный`
- только у члена сущности, не у объявления верхнего уровня файла;
`защищённый` - только у поля/метода сущности) - конкретные ограничения и
согласование по роду/числу - docs/SPEC.md, разделы 6 и 10.

```
program        := { sep } { top_decl { sep } }
top_decl       := function | entity | interface_decl | extern_decl | enum | import_decl
                | platform_block | statement
                                             (* "константа"/"перечисление" верхнего уровня файла - те же
                                                const_decl/enum, что и внутри функции - место в файле только
                                                определяет область видимости (раздел 10), не грамматику *)
sep            := ( newline | ";" ) { newline | ";" }
block          := "{" { sep } [ statement { sep statement } [ sep ] ] "}"

modifier       := visibility_mod | "внутренний" | "внутренняя" | "внутреннее"
                | "статический" | "статическая" | "статическое" | "статические"
                | "абстрактный" | "абстрактная" | "абстрактное" | "абстрактные"
                | "асинхронный" | "асинхронная" | "асинхронное" | "асинхронные"
                | "экспортировать"
visibility_mod := "публичный" | "публичная" | "публичное" | "публичные"
                | "приватный" | "приватная" | "приватное" | "приватные"
                | "защищённый" | "защищённая" | "защищённое" | "защищённые"
TAG            := "`" <произвольный текст без "`"> "`"
                                             (* раздел 6, "Тег поля" - у поля/метода/самой сущности;
                                                содержимое (ключ:значение, ...) - не грамматика Rech *)

function       := { modifier } "функция" IDENT [ type_params ] [ "(" [ params ] ")" ] [ ":" type_name ] { sep } block
params         := param { "," param }
param          := IDENT ":" type_name [ "=" expr ]
type_name      := [ "?" ] ( IDENT [ "<" type_name { "," type_name } ">" ] | "*" type_name | callable_type ) { "[" "]" }
                                             (* "Тип[]"/"Тип[][]" - сахар над "Список<Тип>"/"Список<Список<Тип>>";
                                                "*Тип" - сахар над "Указатель<Тип>" (раздел 22);
                                                "Тип<Аргумент, ...>" - один и более аргументов (встроенная
                                                коллекция, "Указатель<T>" или пользовательский generic-тип,
                                                раздел 7/16) *)
callable_type  := "Вызываемый" [ "<" IDENT { "," IDENT } ">" ] "(" [ type_name { "," type_name } ] ")" [ ":" type_name ]
                                             (* "функция как значение" (раздел 17) - валиден везде, где
                                                ожидается type_name; "<...>" перед скобками - параметры типа
                                                самого значения, той же грамматикой, что у generic-функции *)
type_params    := "<" type_param { "," type_param } ">"
type_param     := IDENT [ "как" type_name { "|" type_name } ]

entity         := { modifier } ( "сущность" | "класс" | "структура" ) IDENT [ type_params ]
                   [ "наследует" IDENT { "," IDENT } ] [ "реализует" IDENT { "," IDENT } ] [ TAG ] { sep }
                   "{" { sep } [ states_section ] { sep } [ fields_section ] { sep } { member { sep } } "}"
                                             (* type_params несовместим с "наследует"/"реализует";
                                                "структура" несовместима с "наследует"/"реализует"/абстрактный
                                                модификатор; "реализует" - только сущность/класс (раздел 6) *)
states_section := "состояние" ":" IDENT { IDENT }      (* одна строка *)
fields_section := "имеет" ":" field { sep field }
                                             (* поле принадлежит секции по ЗНАЧИМОМУ ОТСТУПУ (строже, чем
                                                само слово "имеет"), не по границе-токену - раздел 6 *)
field          := { modifier } IDENT ":" type_name [ "=" expr ] [ TAG ]
member         := field | method | constructor | operator_method | const_decl
method         := { modifier } IDENT "(" [ params ] ")" [ ":" ( type_name | "этот" ) ] [ TAG ] ( { sep } block | sep )
                                             (* без block - тело отсутствует, валидно только у метода
                                                абстрактной сущности (раздел "Наследование и интерфейсы");
                                                ":этот" - ковариантный тип результата (то же место) *)
constructor    := "конструктор" "(" [ params ] ")" { sep } block
operator_method := "статический" visibility_mod "оператор" op_symbol "(" param "," param ")" ":" type_name { sep } block
op_symbol      := "==" | "!=" | "<=" | ">=" | "+" | "-" | "*" | "/" | "%" | "<" | ">"
                                             (* перегрузка операторов, раздел 26 - ровно пара параметров *)
interface_decl := "интерфейс" IDENT [ "наследует" IDENT { "," IDENT } ] { sep } "{" { sep } { interface_method { sep } } "}"
interface_method := IDENT "(" [ params ] ")" [ ":" ( type_name | "этот" ) ]

extern_decl    := extern_func | extern_entity | extern_struct
extern_func    := "внешняя" "функция" IDENT [ "как" IDENT ] "(" [ params ] ")" [ ":" type_name ]
extern_entity  := "внешняя" "сущность" IDENT [ "как" IDENT ]
extern_struct  := "внешняя" "структура" IDENT [ "как" IDENT ] { sep } "{" { sep } fields_section { sep } "}"
                                             (* внешняя функция/структура/сущность - раздел 12 (FFI) *)

import_decl    := "использовать" import_item { "," import_item } "из" STRING
import_item    := IDENT [ "как" IDENT ]
                                             (* раздел 10 - "как" относится к КОНКРЕТНОМУ имени списка,
                                                не ко всему списку сразу *)

platform_block := "#" "если" platform_cond { sep } "{" { sep } { top_decl { sep } } "}"
                   { "#" "иначе" "если" platform_cond { sep } "{" { sep } { top_decl { sep } } "}" }
                   [ "#" "иначе" { sep } "{" { sep } { top_decl { sep } } "}" ]
platform_cond  := "ОС" "." ( "Linux" | "Windows" | "MacOS" )
                                             (* раздел 27 - тело каждой руки - только ОБЪЯВЛЕНИЯ (те же
                                                альтернативы top_decl, кроме statement/platform_block самого
                                                себя), не обычные инструкции; решается ДО семантического
                                                анализа - непройденная рука не проверяется вовсе *)

enum           := "перечисление" IDENT { sep } "{" { sep } enum_member { enum_sep enum_member } { sep } "}"
enum_member    := IDENT [ "=" [ "-" ] INT ]
enum_sep       := ( "," | sep ) { "," | sep }        (* запятая и/или перенос строки, оба допустимы *)

statement      := var_decl | const_decl | assign | field_assign | null_coalesce_assign | index_assign
                | become_stmt | print_stmt | show_stmt | assert_stmt | if_stmt | while_stmt | for_stmt
                | switch_stmt | break_stmt | continue_stmt
                | return_stmt | call_stmt | method_call_stmt | incdec_stmt | throw_stmt | try_stmt | await_stmt
                | gerund_stmt
throw_stmt     := "выбросить" expr
try_stmt       := "попытаться" block { catch_clause } [ "окончательно" block ]
                (* хотя бы один catch_clause ИЛИ "окончательно" обязателен; catch_clause можно повторять
                   подряд (раздел 14) - пробуются по порядку, первое совпадение отрабатывает *)
catch_clause   := "поймать" "(" IDENT ":" type_name { "|" type_name } ")" block
                (* "Паника" - зарезервированное псевдо-имя, допустимо в этом списке
                   как единственный элемент или в сочетании с обычными типами через "|" *)
await_stmt     := "дождаться" postfix   (* значение отбрасывается - валидно, только если обёрнутая функция без результата *)
incdec_stmt    := incdec_prefix | IDENT ( "++" | "--" )   (* значение отбрасывается *)
var_decl       := "переменная" IDENT [ ":" type_name ] "=" expr
                | type_name IDENT "=" expr  (* сахар без "переменная" - тип пишется первым *)
const_decl     := "константа" IDENT "=" expr
assign         := IDENT assign_op expr
field_assign   := ( IDENT | "этот" ) "." IDENT assign_op expr
assign_op      := "=" | "+=" | "-=" | "*=" | "/=" | "%="
null_coalesce_assign := IDENT "?=" expr
index_assign   := IDENT "[" expr "]" "=" expr
                                             (* сахар над IDENT "." "установить" "(" expr "," expr ")" *)
become_stmt    := ( IDENT | "этот" ) "становится" IDENT
                                             (* IDENT - полная форма, творительный падеж *)
print_stmt     := "вывести" expr
show_stmt      := "показать" expr
assert_stmt    := "утверждать" expr [ "," expr ]  (* второй expr - необязательное сообщение,
                                        типа Строка, дописывается к стандартному тексту при провале *)
if_stmt        := "если" expr block [ "иначе" block ]
while_stmt     := "пока" expr block
switch_stmt    := "выбрать" expr { sep } "{" { sep } case_clause { sep case_clause }
                   [ sep "иначе" ":" { sep } statement { sep statement } ] { sep } "}"
case_clause    := "когда" case_value { "|" case_value } ":" { sep } statement { sep statement }
case_value     := expr | IDENT   (* голое имя типа сущности/интерфейса - сравнение "относится к"
                                    (раздел 6); переменная с тем же именем в области видимости побеждает *)
for_stmt       := "для" IDENT "от" expr "до" expr block
                | "для" IDENT "в" NOUN_PREP block       (* NOUN_PREP - см. SPEC.md, раздел 8 *)
                | "для" [ for_init ] ";" [ expr ] ";" [ stmt ] block
                                             (* общий цикл (как C-style for) - без скобок вокруг
                                                трёх секций (в языке их нигде больше нет вокруг
                                                условия), ";" - тот же разделитель инструкций, что
                                                и везде. Каждая секция независимо может быть пустой
                                                ("для ;; { ... }" - бесконечный цикл, выйти можно
                                                только через "прервать"/"вернуть"/паникой/исключением).
                                                Переменная цикла (for_init) всегда объявляется заново
                                                в области видимости цикла, не переприсваивает внешнюю
                                                с тем же именем. *)
for_init       := "переменная" IDENT "=" expr | IDENT "=" expr
break_stmt     := "прервать" [ INT ]  (* только внутри цикла (любого из четырёх видов выше) - INT,
                                        если есть, - глубина вложенности циклов, считая текущий
                                        (1 - ближайший, как и без аргумента); не может превышать
                                        реальную вложенность циклов в этой точке (ошибка компиляции) *)
continue_stmt  := "продолжить" [ INT ] (* та же глубина, что и у "прервать" выше; у общего "для"
                                        всё равно выполняет шаг ЦЕЛЕВОГО цикла перед повторной
                                        проверкой его условия, ровно как в C *)
return_stmt    := "вернуть" [ expr ]
call_stmt      := call_expr
call_expr      := IDENT "(" [ expr { "," expr } ] ")"
                | IDENT "<" type_name { "," type_name } ">" "(" [ expr { "," expr } ] ")"  (* только generic-функция *)
method_call_stmt := method_call_expr
gerund_stmt    := IDENT "(" [ expr { "," expr } ] ")" GERUND_BLOCK
                                             (* раздел 23 - IDENT сам должен быть распознан как
                                                деепричастная форма функции (морфология или тег
                                                `деепричастие:...`); GERUND_BLOCK - одна и более
                                                инструкций строго БОЛЬШИМ отступом, чем сам gerund_stmt
                                                (значимый отступ, не скобки - вне чистого CFG, детали и
                                                правило окончания блока - docs/SPEC.md); внутри блока
                                                неявно доступна переменная "результат" *)

expr           := ternary_expr
ternary_expr   := coalesce_expr [ "?" ternary_expr ":" ternary_expr ]   (* правоассоциативно, самый низкий приоритет *)
coalesce_expr  := or_expr { "??" or_expr }   (* null-coalescing *)
or_expr        := and_expr { ("или" | "||") and_expr }
and_expr       := not_expr { ("и" | "&&") not_expr }
not_expr       := "не" not_expr | comparison
comparison     := bitor_expr [ ("==" | "!=" | "<" | ">" | "<=" | ">=") bitor_expr ]
                | bitor_expr ("==" | "!=") "Ничто" | "Ничто" ("==" | "!=") bitor_expr
                | bitor_expr "относится" "к" IDENT   (* IsExpr, IDENT - имя типа как написано, без склонения *)
bitor_expr     := bitxor_expr { "|" bitxor_expr }        (* только Целое/Байт *)
bitxor_expr    := bitand_expr { "^" bitand_expr }        (* только Целое/Байт *)
bitand_expr    := shift_expr { "&" shift_expr }          (* только Целое/Байт *)
shift_expr     := additive { ("<<" | ">>") additive }    (* только Целое/Байт *)
additive       := multiplicative { ("+" | "-") multiplicative }
multiplicative := unary { ("*" | "/" | "%") unary }
unary          := "-" unary | "~" unary | "!" unary | "&" addressable | "*" unary | incdec_prefix | postfix
                                             (* "~" - только Целое/Байт; "!x" - сахар над "x == Ничто",
                                                только nullable; "&"/"*" - адрес/разыменование (раздел 22),
                                                тот же токен "&", что и у ссылки на функцию в primary ниже -
                                                неоднозначность разрешает семантика (объявленная функция
                                                побеждает); "*" здесь - разыменование, отдельно от "*" как
                                                бинарного умножения уровнем выше (позиция в грамматике,
                                                как и в C) *)
addressable    := IDENT | IDENT "." IDENT | IDENT "[" expr "]"
                                             (* ровно один уровень - раздел 22 *)
incdec_prefix  := ("++" | "--") IDENT
postfix        := primary { ("." IDENT [ "(" [ expr { "," expr } ] ")" ]) | ("[" expr "]") }
                                             (* "[" expr "]" - безопасная индексация, только чтение (ListIndexExpr/DictIndexExpr) *)
primary        := INT | FLOAT | STRING | TEMPLATE_STRING | "истина" | "ложь" | "Ничто" | ( IDENT [ "++" | "--" ] ) | "этот"
                | call_expr | new_expr | list_literal | "(" expr ")" | launch_expr | dispatch_expr | await_stmt
                | lambda_expr | "&" IDENT | parse_expr | serialize_expr
list_literal   := "[" [ expr { "," expr } ] "]"
launch_expr    := "запустить" call_expr   (* только обычная функция программы - не встроенная/внешняя/метод *)
dispatch_expr  := "поручить" call_expr    (* раздел 15 - только "асинхронная функция", спавн на планировщике
                                             корутин, немедленный возврат дескриптора задачи, без ожидания *)
lambda_expr    := "функция" "(" [ params ] ")" [ ":" type_name ] { sep } block
                                             (* без имени, в позиции выражения - раздел 17; замыкание
                                                копирует свободные переменные в момент создания *)
parse_expr     := "разобрать" format_kw expr "как" type_name
serialize_expr := "сериализовать" format_kw expr
format_kw      := "JSON" | "XML" | "YAML" | "PROTOBUF"
                                             (* раздел 18 - обе формы ВЫРАЖЕНИЯ (не инструкции):
                                                "вывести сериализовать JSON значение" *)
new_expr       := new_kw type_name "(" [ new_args ] ")"
new_kw         := "новый" | "новая" | "новое" | "новые"
new_args       := expr { "," expr }                          (* позиционная форма *)
                | IDENT ":" expr { "," IDENT ":" expr }       (* именованная форма *)
method_call_expr := postfix   (* postfix, оканчивающийся на ".имя(...)" *)
```

Постфиксный `.` (`field_access`) применяется к любому первичному выражению,
в том числе к результату `call_expr`/`new_expr`. Доступ к члену
перечисления (`Имя.Член`) синтаксически - тот же `postfix`, без отдельного
правила грамматики: различие видит только семантический анализ.

Программа должна содержать функцию `главная` - она становится точкой входа.
Другие функции разрешены и могут вызывать друг друга независимо от порядка
объявления в файле, включая взаимную рекурсию.

Инструкция на верхнем уровне (не внутри `function`/`entity`) допустима как
альтернатива объявлению `главная` - все такие инструкции собираются в тело
синтезированной `главная` (подробности и ограничения - [docs/SPEC.md](SPEC.md)).
