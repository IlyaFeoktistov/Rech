# Грамматика Rech (EBNF)

Формальная грамматика строгого (context-free) синтаксиса языка - того, что
компилятор принимает прямо сейчас, без пометок "планируется". Семантика
каждой конструкции, полные примеры и построчные пояснения -
[docs/SPEC.md](SPEC.md); учебное изложение с введением - [docs/MANUAL.md](MANUAL.md).

Естественный (падежный) синтаксис - `оформить заказ клиенту` вместо
`оформить(заказ, клиент)` - здесь намеренно не описан: это не отдельная
ветка context-free грамматики, а семантическое сопоставление словоформы с
параметром по падежу поверх уже разобранного `call_expr`/`method_call_expr`
ниже. Правила падежного сопоставления и согласования - [docs/MORPHOLOGY.md](MORPHOLOGY.md).

```
program        := { sep } { ( function | entity | enum | statement ) { sep } }
function       := "функция" IDENT [ type_params ] [ "(" [ params ] ")" ] [ ":" type_name ] { sep } block
params         := param { "," param }
param          := IDENT ":" type_name
type_name      := [ "?" ] IDENT [ "<" type_name { "," type_name } ">" ] { "[" "]" }
                                             (* "Тип[]"/"Тип[][]" - сахар над "Список<Тип>"/"Список<Список<Тип>>" *)
type_params    := "<" type_param { "," type_param } ">"
type_param     := IDENT [ "как" type_name { "|" type_name } ]
entity         := ( "сущность" | "класс" ) IDENT [ type_params ] { sep } "{" { sep } [ states_section ] { sep } [ fields_section ] { sep } { method { sep } } "}"
                                             (* type_params несовместим с "наследует"/"реализует" *)
states_section := "состояние" ":" IDENT { IDENT }      (* одна строка *)
fields_section := "имеет" ":" field { sep field }
field          := IDENT ":" type_name
method         := IDENT "(" [ params ] ")" [ ":" type_name ] { sep } block
block          := "{" { sep } [ statement { sep statement } [ sep ] ] "}"
sep            := ( newline | ";" ) { newline | ";" }
enum           := "перечисление" IDENT { sep } "{" { sep } enum_member { enum_sep enum_member } { sep } "}"
enum_member    := IDENT [ "=" [ "-" ] INT ]
enum_sep       := ( "," | sep ) { "," | sep }        (* запятая и/или перенос строки, оба допустимы *)

statement      := var_decl | const_decl | assign | field_assign | null_coalesce_assign | index_assign
                | become_stmt | print_stmt | show_stmt | assert_stmt | if_stmt | while_stmt | for_stmt
                | break_stmt | continue_stmt
                | return_stmt | call_stmt | method_call_stmt | incdec_stmt | throw_stmt | try_stmt | await_stmt
throw_stmt     := "выбросить" expr
try_stmt       := "попытаться" block { catch_clause } [ "окончательно" block ]
                (* хотя бы один catch_clause ИЛИ "окончательно" обязателен *)
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

expr           := ternary_expr
ternary_expr   := coalesce_expr [ "?" ternary_expr ":" ternary_expr ]   (* правоассоциативно, самый низкий приоритет *)
coalesce_expr  := or_expr { "??" or_expr }   (* null-coalescing *)
or_expr        := and_expr { ("или" | "||") and_expr }
and_expr       := not_expr { ("и" | "&&") not_expr }
not_expr       := "не" not_expr | comparison
comparison     := bitor_expr [ ("==" | "!=" | "<" | ">" | "<=" | ">=") bitor_expr ]
                | bitor_expr ("==" | "!=") "Ничто" | "Ничто" ("==" | "!=") bitor_expr
                | bitor_expr "относится" "к" IDENT   (* IsExpr, IDENT - имя типа как написано, без склонения *)
bitor_expr     := bitxor_expr { "|" bitxor_expr }        (* только Целое *)
bitxor_expr    := bitand_expr { "^" bitand_expr }        (* только Целое *)
bitand_expr    := shift_expr { "&" shift_expr }          (* только Целое *)
shift_expr     := additive { ("<<" | ">>") additive }    (* только Целое *)
additive       := multiplicative { ("+" | "-") multiplicative }
multiplicative := unary { ("*" | "/" | "%") unary }
unary          := "-" unary | "~" unary | "!" unary | incdec_prefix | postfix   (* "~" - только Целое; "!x" - сахар над "x == Ничто", только nullable *)
incdec_prefix  := ("++" | "--") IDENT
postfix        := primary { ("." IDENT [ "(" [ expr { "," expr } ] ")" ]) | ("[" expr "]") }
                                             (* "[" expr "]" - безопасная индексация, только чтение (ListIndexExpr/DictIndexExpr) *)
primary        := INT | FLOAT | STRING | "истина" | "ложь" | "Ничто" | ( IDENT [ "++" | "--" ] ) | "этот"
                | call_expr | new_expr | list_literal | "(" expr ")" | launch_expr | await_stmt
list_literal   := "[" [ expr { "," expr } ] "]"
launch_expr    := "запустить" call_expr   (* только обычная функция программы - не встроенная/внешняя/метод *)
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
