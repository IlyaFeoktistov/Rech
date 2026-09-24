// "разобрать PROTOBUF <выражение> как <Тип>"/"сериализовать PROTOBUF
// <выражение>" (internal/ir.ParseExpr/SerializeExpr, Format == "PROTOBUF") -
// ручная реализация двоичного wire-формата Protocol Buffers (varint/
// fixed32/fixed64/length-delimited), тем же общим устройством, что и JSON
// (json_extract.go/json_serialize.go) - для каждого типа из prog.
// ProtobufParseTargets/ProtobufSerializeTargets (и рекурсивно для любого
// достижимого из него типа поля/элемента Список<T>) генерируется своя
// функция разбора/сериализации. См. package doc internal/semantic/
// protobuf.go для схемы (теги "protobuf:<номер>") и internal/interp/
// protobuf.go для эквивалентной реализации на Go.
//
// В отличие от JSON примитивы НЕ получают собственных generated-функций -
// они инлайнятся прямо в тело сообщения через общие рантайм-хелперы
// (rech_pb_write_int/rech_pb_read_varint/...), параметризованные номером
// поля как C-литералом (известен уже на этапе кодогенерации из тега) - у
// protobuf, в отличие от JSON, есть разница между "как записано в
// СООБЩЕНИИ" (тег+значение) и "как записано в PACKED-repeated" (голое
// значение без тега) - единая рекурсивная функция на тип, как у JSON,
// не покрыла бы обе формы без разрастания сигнатуры, поэтому только
// СООБЩЕНИЯ (сущность/структура) получают собственную generated-функцию
// (rech_pb_serialize_<T>/rech_pb_parse_<T>), примитивы - нет.
//
// Собственные разбор/сериализация НЕ бросают исключение на несовпадение
// номера поля/wire-типа с ожидаемым (в отличие от JSON) - protobuf по
// спецификации молча пропускает такие данные (rtmsg.ProtobufParseError,
// см. её package doc) - "ОшибкаРазбора" (через rech_struct_throw, тот же
// канал, что и у JSON) бросается ТОЛЬКО когда байты физически обрываются
// раньше конца ожидаемого значения.
//
// Предположение о хосте (не проверяется и не документировано отдельно в
// docs/SPEC.md - тот же уровень поддержки, что и у остального cbackend):
// little-endian - fixed32/fixed64 читаются/пишутся прямым memcpy без
// перестановки байт (протокол сам всегда little-endian, вне зависимости от
// хоста) - верно для всех реалистичных целей сборки этого компилятора
// (x86/x86-64/ARM в обычном режиме), big-endian хосты не поддержаны нигде
// в cbackend и здесь не исключение.
package cbackend

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
	"github.com/IlyaFeoktistov/Rech/internal/structtag"
)

// pbFieldNumber - см. internal/interp/protobuf.go, тот же принцип: номер
// поля уже проверен internal/semantic/protobuf.go для любого поля,
// достижимого из места вызова - ошибка здесь невозможна.
func pbFieldNumber(f ir.Field) int {
	n, _ := strconv.Atoi(structtag.Parse(f.Tag).Get("protobuf"))
	return n
}

func pbSerializeFuncName(entity string) string { return "rech_pb_serialize_" + transliterate(entity) }
func pbParseFuncName(entity string) string     { return "rech_pb_parse_" + transliterate(entity) }
func pbZeroFuncName(entity string) string      { return "rech_pb_zero_" + transliterate(entity) }
func pbToBytesWrapperName(entity string) string {
	return "rech_pb_serialize_to_bytes_" + transliterate(entity)
}

// ================= Сериализация (сериализовать PROTOBUF) =================

type pbSerGen struct {
	ctx       *irGenCtx
	generics  map[string]ir.GenericInstantiation
	generated map[string]bool
	code      strings.Builder
}

// protobufSerializeRuntimeCode - см. package doc, вызывается через
// resolveRuntimeCode (ir_generator.go) только когда prog.
// ProtobufSerializeTargets непуст. targets - ВСЕГДА сущности/структуры
// (protobufTopLevelMessage, internal/semantic/protobuf.go гарантирует, что
// верхний уровень "сериализовать PROTOBUF" - только message, не примитив/
// список).
func protobufSerializeRuntimeCode(ctx *irGenCtx, prog *ir.Program, targets []ir.Type) (string, error) {
	generics := make(map[string]ir.GenericInstantiation, len(prog.Generics))
	for _, g := range prog.Generics {
		generics[g.Name] = g
	}
	g := &pbSerGen{ctx: ctx, generics: generics, generated: map[string]bool{}}
	for _, t := range targets {
		if err := g.ensure(t); err != nil {
			return "", err
		}
	}
	var sb strings.Builder
	sb.WriteString(pbEncodeRuntimeCode)
	sb.WriteString(g.code.String())
	wrapped := map[string]bool{}
	for _, t := range targets {
		if wrapped[t.Entity] {
			continue
		}
		wrapped[t.Entity] = true
		code, err := pbToBytesWrapperCode(ctx, t)
		if err != nil {
			return "", err
		}
		sb.WriteString(code)
	}
	return sb.String(), nil
}

// ensure генерирует rech_pb_serialize_<T> для t (сущность/структура,
// "сообщение") и рекурсивно для любого достижимого из неё типа
// вложенного сообщения (поле или элемент Список<T>) - примитивы/
// Список<Байт> инлайнятся на месте (fieldSerializeStmt), собственной
// функции не получают.
func (g *pbSerGen) ensure(t ir.Type) error {
	if t.Kind != ir.KindEntity || t.Entity == byteListType {
		return nil
	}
	if g.generated[t.Entity] {
		return nil
	}
	g.generated[t.Entity] = true
	if gi, ok := g.generics[t.Entity]; ok && gi.Template == "Список" {
		return g.ensure(gi.ElemType)
	}
	ent, ok := g.ctx.entities[t.Entity]
	if !ok {
		return fmt.Errorf("генерация C из IR: \"сериализовать PROTOBUF\" для неизвестного типа %q", t.Entity)
	}
	for _, f := range ent.Fields {
		if err := g.ensure(f.Type); err != nil {
			return err
		}
	}
	code, err := g.messageSerializerCode(ent)
	if err != nil {
		return err
	}
	g.code.WriteString(code)
	return nil
}

func (g *pbSerGen) messageSerializerCode(ent *ir.Entity) (string, error) {
	valueC := fromIRType(ir.EntityType(ent.Name)).cType(g.ctx)
	access := "v."
	if g.ctx.entityIsRef(ent.Name) {
		access = "v->"
	}
	var body strings.Builder
	for _, f := range ent.Fields {
		stmt, err := g.fieldSerializeStmt(access+mangleField(f.Name), f.Type, pbFieldNumber(f))
		if err != nil {
			return "", err
		}
		body.WriteString(stmt)
	}
	return fmt.Sprintf("static void %s(%s v, rech_pb_sb *sb) {\n%s}\n\n", pbSerializeFuncName(ent.Name), valueC, body.String()), nil
}

// fieldSerializeStmt - C-код записи ОДНОГО singular-поля (не элемента
// Список<T> - см. repeatedFieldStmt) - proto3 implicit presence: значение
// "по умолчанию" (0/""/ложь/пустой Список<Байт>) целиком пропускается,
// вложенное сообщение кодируется всегда (нет понятия "не присвоено" у
// поля-сущности в этой модели - оно всегда сконструировано).
func (g *pbSerGen) fieldSerializeStmt(exprC string, t ir.Type, num int) (string, error) {
	switch t.Kind {
	case ir.KindInt:
		return fmt.Sprintf("    if (%s != 0) { rech_pb_write_int(sb, %d, %s); }\n", exprC, num, exprC), nil
	case ir.KindBool:
		return fmt.Sprintf("    if (%s) { rech_pb_write_bool(sb, %d, %s); }\n", exprC, num, exprC), nil
	case ir.KindFloat32:
		return fmt.Sprintf("    if (%s != 0.0f) { rech_pb_write_float32(sb, %d, %s); }\n", exprC, num, exprC), nil
	case ir.KindFloat:
		return fmt.Sprintf("    if (%s != 0.0) { rech_pb_write_float64(sb, %d, %s); }\n", exprC, num, exprC), nil
	case ir.KindString:
		return fmt.Sprintf("    if (%s[0] != '\\0') { rech_pb_write_string(sb, %d, %s); }\n", exprC, num, exprC), nil
	case ir.KindEntity:
		if t.Entity == byteListType {
			return fmt.Sprintf("    if (%s->len > 0) { rech_pb_write_bytes(sb, %d, %s->data, %s->len); }\n", exprC, num, exprC, exprC), nil
		}
		if gi, ok := g.generics[t.Entity]; ok && gi.Template == "Список" {
			return g.repeatedFieldStmt(exprC, gi, num)
		}
		if err := g.ensure(t); err != nil {
			return "", err
		}
		return fmt.Sprintf(`    {
        rech_pb_sb tmp;
        rech_pb_sb_init(&tmp);
        %s(%s, &tmp);
        rech_pb_write_bytes(sb, %d, tmp.buf, tmp.len);
        free(tmp.buf);
    }
`, pbSerializeFuncName(t.Entity), exprC, num), nil
	default:
		return "", fmt.Errorf("генерация C из IR: \"сериализовать PROTOBUF\" для неподдержанного типа %s", t)
	}
}

// pbRawWriterFor - имя рантайм-хелпера, пишущего ГОЛОЕ (без тега) значение
// packable-скаляра - используется только внутри packed-payload
// (repeatedFieldStmt).
var pbRawWriterFor = map[ir.TypeKind]string{
	ir.KindInt:     "rech_pb_raw_int",
	ir.KindBool:    "rech_pb_raw_bool",
	ir.KindFloat32: "rech_pb_raw_float32",
	ir.KindFloat:   "rech_pb_raw_float64",
}

// repeatedFieldStmt - C-код записи Список<T>-поля: packed (одно
// length-delimited поле - длина + конкатенация ГОЛЫХ значений без тегов)
// для packable-скаляров (Целое/Логическое/Дробное/Действительное - proto3-
// умолчание для repeated-скаляров), unpacked (тег+значение НА КАЖДЫЙ
// элемент) для Строка/Список<Байт>/вложенного сообщения - элементы
// кодируются ВСЕГДА, включая нулевые (значима длина списка, не значение
// элемента, в отличие от singular-поля).
func (g *pbSerGen) repeatedFieldStmt(listExprC string, gi ir.GenericInstantiation, num int) (string, error) {
	elem := gi.ElemType
	listC := mangleType(gi.Name)
	if raw, ok := pbRawWriterFor[elem.Kind]; ok {
		return fmt.Sprintf(`    if (%[1]s->len > 0) {
        rech_pb_sb tmp;
        rech_pb_sb_init(&tmp);
        for (long long i = 0; i < %[1]s->len; i++) {
            %[2]s(&tmp, %[1]s->data[i]);
        }
        rech_pb_write_bytes(sb, %[3]d, tmp.buf, tmp.len);
        free(tmp.buf);
    }
`, listExprC, raw, num), nil
	}
	switch elem.Kind {
	case ir.KindString:
		return fmt.Sprintf(`    for (long long i = 0; i < %[1]s->len; i++) {
        rech_pb_write_string(sb, %[2]d, %[1]s->data[i]);
    }
`, listExprC, num), nil
	case ir.KindEntity:
		if elem.Entity == byteListType {
			return fmt.Sprintf(`    for (long long i = 0; i < %[1]s->len; i++) {
        %[2]s *e = %[1]s->data[i];
        rech_pb_write_bytes(sb, %[3]d, e->data, e->len);
    }
`, listExprC, mangleType(byteListType), num), nil
		}
		if err := g.ensure(elem); err != nil {
			return "", err
		}
		return fmt.Sprintf(`    for (long long i = 0; i < %[1]s->len; i++) {
        rech_pb_sb tmp;
        rech_pb_sb_init(&tmp);
        %[2]s(%[1]s->data[i], &tmp);
        rech_pb_write_bytes(sb, %[3]d, tmp.buf, tmp.len);
        free(tmp.buf);
    }
`, listExprC, pbSerializeFuncName(elem.Entity), num), nil
	default:
		return "", fmt.Errorf("генерация C из IR: \"сериализовать PROTOBUF\" - %s<%s> не поддержан", listC, elem)
	}
}

// pbToBytesWrapperCode - "rech_pb_serialize_to_bytes_<T>(v) -> Список<Байт>*" -
// только для targets (типы, реально встретившиеся как ВЕРХНИЙ уровень
// "сериализовать PROTOBUF ..."), тем же принципом, что и
// jsonToStringWrapperCode (json_serialize.go), но результат - Список<Байт>,
// не C-строка.
func pbToBytesWrapperCode(ctx *irGenCtx, t ir.Type) (string, error) {
	valueC := fromIRType(t).cType(ctx)
	listC := mangleType(byteListType)
	return fmt.Sprintf(`static %[1]s *%[2]s(%[3]s v) {
    rech_pb_sb sb;
    rech_pb_sb_init(&sb);
    %[4]s(v, &sb);
    %[1]s *out = rech_bytes_wrap(sb.buf, sb.len);
    free(sb.buf);
    return out;
}

`, listC, pbToBytesWrapperName(t.Entity), valueC, pbSerializeFuncName(t.Entity)), nil
}

// generateIRSerializeExprPROTOBUF - см. generateIRSerializeExpr (json_serialize.go).
func generateIRSerializeExprPROTOBUF(ctx *irGenCtx, t ir.Type, valueCode string) (string, valueType, error) {
	ctx.usesByteListWrap = true
	code := fmt.Sprintf("%s(%s)", pbToBytesWrapperName(t.Entity), valueCode)
	return code, fromIRType(ir.EntityType(byteListType)), nil
}

// pbEncodeRuntimeCode - растущий байтовый буфер (rech_pb_sb, malloc/realloc -
// тот же принцип, что и rech_json_sb, json_serialize.go) и рантайм-хелперы
// записи: varint/тег, "тегированные" writer'ы (rech_pb_write_int/.../
// rech_pb_write_bytes - всегда пишут тег+значение, пропуск нулевого
// singular-значения решается генерируемым кодом снаружи, не здесь) и
// "голые" writer'ы (rech_pb_raw_* - без тега, только для packed-payload
// внутри repeated-поля).
const pbEncodeRuntimeCode = `typedef struct { unsigned char *buf; long long len; long long cap; } rech_pb_sb;

static void rech_pb_sb_init(rech_pb_sb *sb) {
    sb->cap = 64;
    sb->len = 0;
    sb->buf = malloc((size_t)sb->cap);
}

static void rech_pb_sb_ensure(rech_pb_sb *sb, long long extra) {
    if (sb->len + extra > sb->cap) {
        while (sb->cap < sb->len + extra) {
            sb->cap *= 2;
        }
        sb->buf = realloc(sb->buf, (size_t)sb->cap);
    }
}

static void rech_pb_sb_byte(rech_pb_sb *sb, unsigned char b) {
    rech_pb_sb_ensure(sb, 1);
    sb->buf[sb->len++] = b;
}

static void rech_pb_sb_bytes(rech_pb_sb *sb, const unsigned char *data, long long n) {
    if (n <= 0) return;
    rech_pb_sb_ensure(sb, n);
    memcpy(sb->buf + sb->len, data, (size_t)n);
    sb->len += n;
}

static void rech_pb_varint(rech_pb_sb *sb, unsigned long long v) {
    while (v >= 0x80) {
        rech_pb_sb_byte(sb, (unsigned char)(v | 0x80));
        v >>= 7;
    }
    rech_pb_sb_byte(sb, (unsigned char)v);
}

static void rech_pb_tag(rech_pb_sb *sb, long long num, unsigned int wiretype) {
    rech_pb_varint(sb, ((unsigned long long)num << 3) | (unsigned long long)wiretype);
}

static void rech_pb_write_int(rech_pb_sb *sb, long long num, long long v) {
    rech_pb_tag(sb, num, 0);
    rech_pb_varint(sb, (unsigned long long)v);
}

static void rech_pb_write_bool(rech_pb_sb *sb, long long num, bool v) {
    rech_pb_tag(sb, num, 0);
    rech_pb_varint(sb, v ? 1 : 0);
}

static void rech_pb_write_float32(rech_pb_sb *sb, long long num, float v) {
    rech_pb_tag(sb, num, 5);
    unsigned int bits;
    memcpy(&bits, &v, 4);
    rech_pb_sb_bytes(sb, (const unsigned char *)&bits, 4);
}

static void rech_pb_write_float64(rech_pb_sb *sb, long long num, double v) {
    rech_pb_tag(sb, num, 1);
    unsigned long long bits;
    memcpy(&bits, &v, 8);
    rech_pb_sb_bytes(sb, (const unsigned char *)&bits, 8);
}

static void rech_pb_write_bytes(rech_pb_sb *sb, long long num, const unsigned char *data, long long len) {
    rech_pb_tag(sb, num, 2);
    rech_pb_varint(sb, (unsigned long long)len);
    rech_pb_sb_bytes(sb, data, len);
}

static void rech_pb_write_string(rech_pb_sb *sb, long long num, const char *s) {
    rech_pb_write_bytes(sb, num, (const unsigned char *)s, (long long)strlen(s));
}

static void rech_pb_raw_int(rech_pb_sb *sb, long long v) {
    rech_pb_varint(sb, (unsigned long long)v);
}

static void rech_pb_raw_bool(rech_pb_sb *sb, bool v) {
    rech_pb_varint(sb, v ? 1 : 0);
}

static void rech_pb_raw_float32(rech_pb_sb *sb, float v) {
    unsigned int bits;
    memcpy(&bits, &v, 4);
    rech_pb_sb_bytes(sb, (const unsigned char *)&bits, 4);
}

static void rech_pb_raw_float64(rech_pb_sb *sb, double v) {
    unsigned long long bits;
    memcpy(&bits, &v, 8);
    rech_pb_sb_bytes(sb, (const unsigned char *)&bits, 8);
}

`

// ================= Разбор (разобрать PROTOBUF) =================

type pbDecGen struct {
	ctx       *irGenCtx
	generics  map[string]ir.GenericInstantiation
	generated map[string]bool
	code      strings.Builder
}

// protobufParseRuntimeCode - см. package doc, вызывается через
// resolveRuntimeCode только когда prog.ProtobufParseTargets непуст. targets
// - всегда сущности/структуры (protobufTopLevelMessage).
func protobufParseRuntimeCode(ctx *irGenCtx, prog *ir.Program, targets []ir.Type) (string, error) {
	generics := make(map[string]ir.GenericInstantiation, len(prog.Generics))
	for _, g := range prog.Generics {
		generics[g.Name] = g
	}
	g := &pbDecGen{ctx: ctx, generics: generics, generated: map[string]bool{}}
	var sb strings.Builder
	fmt.Fprintf(&sb, "#define RECH_PB_PARSE_ERR \"%s\"\n", escapeCString(rtmsg.ProtobufParseError))
	sb.WriteString(pbDecodeRuntimeCode)
	for _, t := range targets {
		if err := g.ensure(t); err != nil {
			return "", err
		}
	}
	sb.WriteString(g.code.String())
	return sb.String(), nil
}

func (g *pbDecGen) ensure(t ir.Type) error {
	if t.Kind != ir.KindEntity || t.Entity == byteListType {
		return nil
	}
	if g.generated[t.Entity] {
		return nil
	}
	g.generated[t.Entity] = true
	if gi, ok := g.generics[t.Entity]; ok && gi.Template == "Список" {
		return g.ensure(gi.ElemType)
	}
	ent, ok := g.ctx.entities[t.Entity]
	if !ok {
		return fmt.Errorf("генерация C из IR: \"разобрать PROTOBUF\" в неизвестный тип %q", t.Entity)
	}
	for _, f := range ent.Fields {
		if err := g.ensure(f.Type); err != nil {
			return err
		}
	}
	zeroCode, err := g.zeroValueFuncCode(ent)
	if err != nil {
		return err
	}
	g.code.WriteString(zeroCode)
	parseCode, err := g.messageParserCode(ent)
	if err != nil {
		return err
	}
	g.code.WriteString(parseCode)
	return nil
}

// zeroValueFuncCode - "rech_pb_zero_<T>(void) -> CType": значение T со
// всеми полями по нулевому умолчанию (proto3 implicit presence - поле,
// НИ РАЗУ не встретившееся на wire, обязано остаться нулевым) - у
// ссылочной сущности это просто её собственный "rech_new_X()" (уже
// корректно зануляет generic-коллекции/Строку через zeroFieldValueExpr,
// вызываемый её конструктором, см. package doc ir_generator.go,
// generateIREntityCtorInit); у структуры (значение, без конструктора)
// нужно построить компаунд-литерал вручную - переиспользует ТОТ ЖЕ
// zeroFieldValueExpr, что и настоящий конструктор сущности (не дублирует
// логику "какое нулевое значение у какого типа").
func (g *pbDecGen) zeroValueFuncCode(ent *ir.Entity) (string, error) {
	cType := fromIRType(ir.EntityType(ent.Name)).cType(g.ctx)
	if g.ctx.entityIsRef(ent.Name) {
		return fmt.Sprintf("static %s %s(void) {\n    return %s();\n}\n\n", cType, pbZeroFuncName(ent.Name), mangleCtor(ent.Name)), nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "static %s %s(void) {\n", cType, pbZeroFuncName(ent.Name))
	fmt.Fprintf(&b, "    %s v = {0};\n", cType)
	for _, f := range ent.Fields {
		expr := zeroFieldValueExpr(g.ctx, f.Type)
		if expr == nil {
			continue
		}
		code, _, err := generateIRExpr(g.ctx, expr)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&b, "    v.%s = %s;\n", mangleField(f.Name), code)
	}
	b.WriteString("    return v;\n}\n\n")
	return b.String(), nil
}

// messageParserCode - "rech_pb_parse_<T>(rech_pbp *p) -> CType": читает
// теги из p до p->end (границу - весь буфер для верхнего уровня, срез
// длины length-delimited значения для вложенного сообщения - см. вызывающий
// код), диспетчеризуя по номеру поля через switch (номера полей - те же
// C-литералы, что и в messageSerializerCode). Незнакомый номер ИЛИ
// несовпадение wire-типа с ожидаемым по полю - тихий пропуск
// (rech_pb_skip) - см. package doc, protobuf не бросает исключение на
// это, только на физически оборванные байты.
func (g *pbDecGen) messageParserCode(ent *ir.Entity) (string, error) {
	cType := fromIRType(ir.EntityType(ent.Name)).cType(g.ctx)
	access := "obj."
	if g.ctx.entityIsRef(ent.Name) {
		access = "obj->"
	}
	var body strings.Builder
	fmt.Fprintf(&body, "static %s %s(rech_pbp *p) {\n", cType, pbParseFuncName(ent.Name))
	fmt.Fprintf(&body, "    %s obj = %s();\n", cType, pbZeroFuncName(ent.Name))
	body.WriteString("    while (p->cur < p->end) {\n")
	body.WriteString("        long long num; int wt;\n")
	body.WriteString("        rech_pb_read_tag(p, &num, &wt);\n")
	body.WriteString("        switch (num) {\n")
	for _, f := range ent.Fields {
		caseBody, err := g.fieldParseCaseCode(access+mangleField(f.Name), f.Type)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&body, "        case %d: {\n%s        } break;\n", pbFieldNumber(f), caseBody)
	}
	body.WriteString("        default:\n            rech_pb_skip(p, wt);\n            break;\n")
	body.WriteString("        }\n    }\n")
	body.WriteString("    return obj;\n}\n\n")
	return body.String(), nil
}

// fieldParseCaseCode - тело ОДНОГО "case <номер>:" внутри switch(num) -
// singular-поле (не элемент Список<T> - см. repeatedFieldParseCode).
// exprC - lvalue-выражение поля ("obj->имя"/"obj.имя").
func (g *pbDecGen) fieldParseCaseCode(exprC string, t ir.Type) (string, error) {
	switch t.Kind {
	case ir.KindInt:
		return fmt.Sprintf("            if (wt == 0) { %s = (long long)rech_pb_read_varint(p); } else { rech_pb_skip(p, wt); }\n", exprC), nil
	case ir.KindBool:
		return fmt.Sprintf("            if (wt == 0) { %s = rech_pb_read_varint(p) != 0; } else { rech_pb_skip(p, wt); }\n", exprC), nil
	case ir.KindFloat32:
		return fmt.Sprintf(`            if (wt == 5) {
                unsigned int bits = rech_pb_read_fixed32(p);
                memcpy(&%s, &bits, 4);
            } else {
                rech_pb_skip(p, wt);
            }
`, exprC), nil
	case ir.KindFloat:
		return fmt.Sprintf(`            if (wt == 1) {
                unsigned long long bits = rech_pb_read_fixed64(p);
                memcpy(&%s, &bits, 8);
            } else {
                rech_pb_skip(p, wt);
            }
`, exprC), nil
	case ir.KindString:
		return fmt.Sprintf(`            if (wt == 2) {
                long long n;
                const unsigned char *data = rech_pb_read_bytes(p, &n);
                char *s = GC_MALLOC((size_t)n + 1);
                memcpy(s, data, (size_t)n);
                s[n] = '\0';
                if (!rech_text_valid((const unsigned char *)s, (size_t)n)) {
                    s = rech_text_fixed((const unsigned char *)s, (size_t)n);
                }
                %s = s;
            } else {
                rech_pb_skip(p, wt);
            }
`, exprC), nil
	case ir.KindEntity:
		if t.Entity == byteListType {
			return fmt.Sprintf(`            if (wt == 2) {
                long long n;
                const unsigned char *data = rech_pb_read_bytes(p, &n);
                %s = rech_bytes_wrap(data, n);
            } else {
                rech_pb_skip(p, wt);
            }
`, exprC), nil
		}
		if gi, ok := g.generics[t.Entity]; ok && gi.Template == "Список" {
			return g.repeatedFieldParseCode(exprC, gi)
		}
		if err := g.ensure(t); err != nil {
			return "", err
		}
		return fmt.Sprintf(`            if (wt == 2) {
                long long n;
                const unsigned char *data = rech_pb_read_bytes(p, &n);
                rech_pbp sub;
                sub.cur = data;
                sub.end = data + n;
                %s = %s(&sub);
            } else {
                rech_pb_skip(p, wt);
            }
`, exprC, pbParseFuncName(t.Entity)), nil
	default:
		return "", fmt.Errorf("генерация C из IR: \"разобрать PROTOBUF\" в неподдержанный тип %s", t)
	}
}

// pbAddMethodCall - "receiver.добавить(значение)" как C-вызов (та же
// свободная функция-обёртка, что и у обычного generic-метода - см.
// mangleMethod).
func pbAddMethodCall(gi ir.GenericInstantiation, listExprC, valueC string) string {
	return fmt.Sprintf("%s(%s, %s)", mangleMethod(gi.Name, "добавить"), listExprC, valueC)
}

// repeatedFieldParseCode - тело "case <номер>:" для Список<T>-поля -
// принимает ОБЕ формы, которые допускает сама спецификация protobuf:
// packed (wt == 2 для packable-скаляра elemType - длина + конкатенация
// голых значений без тегов) и unpacked (wt совпадает с "родным" wire-типом
// elemType - одно значение). См. internal/interp/protobuf.go,
// pbDecodeRepeatedElement - та же логика на Go.
func (g *pbDecGen) repeatedFieldParseCode(listExprC string, gi ir.GenericInstantiation) (string, error) {
	elem := gi.ElemType
	switch elem.Kind {
	case ir.KindInt, ir.KindBool:
		conv := "(long long)v"
		if elem.Kind == ir.KindBool {
			conv = "v != 0"
		}
		return fmt.Sprintf(`            if (wt == 2) {
                long long n;
                const unsigned char *data = rech_pb_read_bytes(p, &n);
                rech_pbp sub;
                sub.cur = data;
                sub.end = data + n;
                while (sub.cur < sub.end) {
                    unsigned long long v = rech_pb_read_varint(&sub);
                    %s;
                }
            } else if (wt == 0) {
                unsigned long long v = rech_pb_read_varint(p);
                %s;
            } else {
                rech_pb_skip(p, wt);
            }
`, pbAddMethodCall(gi, listExprC, conv), pbAddMethodCall(gi, listExprC, conv)), nil
	case ir.KindFloat32:
		return fmt.Sprintf(`            if (wt == 2) {
                long long n;
                const unsigned char *data = rech_pb_read_bytes(p, &n);
                rech_pbp sub;
                sub.cur = data;
                sub.end = data + n;
                while (sub.end - sub.cur >= 4) {
                    unsigned int bits = rech_pb_read_fixed32(&sub);
                    float fv;
                    memcpy(&fv, &bits, 4);
                    %s;
                }
            } else if (wt == 5) {
                unsigned int bits = rech_pb_read_fixed32(p);
                float fv;
                memcpy(&fv, &bits, 4);
                %s;
            } else {
                rech_pb_skip(p, wt);
            }
`, pbAddMethodCall(gi, listExprC, "fv"), pbAddMethodCall(gi, listExprC, "fv")), nil
	case ir.KindFloat:
		return fmt.Sprintf(`            if (wt == 2) {
                long long n;
                const unsigned char *data = rech_pb_read_bytes(p, &n);
                rech_pbp sub;
                sub.cur = data;
                sub.end = data + n;
                while (sub.end - sub.cur >= 8) {
                    unsigned long long bits = rech_pb_read_fixed64(&sub);
                    double dv;
                    memcpy(&dv, &bits, 8);
                    %s;
                }
            } else if (wt == 1) {
                unsigned long long bits = rech_pb_read_fixed64(p);
                double dv;
                memcpy(&dv, &bits, 8);
                %s;
            } else {
                rech_pb_skip(p, wt);
            }
`, pbAddMethodCall(gi, listExprC, "dv"), pbAddMethodCall(gi, listExprC, "dv")), nil
	case ir.KindString:
		return fmt.Sprintf(`            if (wt == 2) {
                long long n;
                const unsigned char *data = rech_pb_read_bytes(p, &n);
                char *s = GC_MALLOC((size_t)n + 1);
                memcpy(s, data, (size_t)n);
                s[n] = '\0';
                if (!rech_text_valid((const unsigned char *)s, (size_t)n)) {
                    s = rech_text_fixed((const unsigned char *)s, (size_t)n);
                }
                %s;
            } else {
                rech_pb_skip(p, wt);
            }
`, pbAddMethodCall(gi, listExprC, "s")), nil
	case ir.KindEntity:
		if elem.Entity == byteListType {
			return fmt.Sprintf(`            if (wt == 2) {
                long long n;
                const unsigned char *data = rech_pb_read_bytes(p, &n);
                %s;
            } else {
                rech_pb_skip(p, wt);
            }
`, pbAddMethodCall(gi, listExprC, "rech_bytes_wrap(data, n)")), nil
		}
		if err := g.ensure(elem); err != nil {
			return "", err
		}
		return fmt.Sprintf(`            if (wt == 2) {
                long long n;
                const unsigned char *data = rech_pb_read_bytes(p, &n);
                rech_pbp sub;
                sub.cur = data;
                sub.end = data + n;
                %s;
            } else {
                rech_pb_skip(p, wt);
            }
`, pbAddMethodCall(gi, listExprC, pbParseFuncName(elem.Entity)+"(&sub)")), nil
	default:
		return "", fmt.Errorf("генерация C из IR: \"разобрать PROTOBUF\" - Список<%s> не поддержан", elem)
	}
}

// generateIRParseExprPROTOBUF - см. generateIRParseExpr (json_extract.go).
func generateIRParseExprPROTOBUF(ctx *irGenCtx, e *ir.ParseExpr, srcCode string) (string, valueType, error) {
	ctx.usesExceptions = true
	ctx.usesByteListWrap = true
	code := fmt.Sprintf("%s(&(rech_pbp){.cur = (%s)->data, .end = (%s)->data + (%s)->len})", pbParseFuncName(e.To.Entity), srcCode, srcCode, srcCode)
	return code, fromIRType(e.To), nil
}

// pbDecodeRuntimeCode - курсор (rech_pbp) и рантайм-хелперы чтения:
// varint/тег/fixed32/fixed64/length-delimited, rech_pb_skip - пропуск
// незнакомого/несовпадающего поля по его wire-типу (group, 3/4, - устаревшая
// форма, не поддержана). Все бросают "ОшибкаРазбора" через
// rech_struct_throw (уже эмитирован - см. structParseThrowHelperCode,
// ir_generator.go, условие включает prog.ProtobufParseTargets) ТОЛЬКО когда
// байты физически обрываются раньше конца ожидаемого значения - несовпадение
// wire-типа с ожидаемым по статическому типу поля (см. fieldParseCaseCode)
// - тихий пропуск, не ошибка (см. package doc).
const pbDecodeRuntimeCode = `typedef struct { const unsigned char *cur; const unsigned char *end; } rech_pbp;

static unsigned long long rech_pb_read_varint(rech_pbp *p) {
    unsigned long long v = 0;
    int shift = 0;
    while (p->cur < p->end) {
        unsigned char b = *p->cur++;
        v |= ((unsigned long long)(b & 0x7F)) << shift;
        if (!(b & 0x80)) {
            return v;
        }
        shift += 7;
        if (shift >= 70) {
            break;
        }
    }
    rech_struct_throw(RECH_PB_PARSE_ERR);
    return 0;
}

static unsigned int rech_pb_read_fixed32(rech_pbp *p) {
    if (p->end - p->cur < 4) {
        rech_struct_throw(RECH_PB_PARSE_ERR);
    }
    unsigned int v;
    memcpy(&v, p->cur, 4);
    p->cur += 4;
    return v;
}

static unsigned long long rech_pb_read_fixed64(rech_pbp *p) {
    if (p->end - p->cur < 8) {
        rech_struct_throw(RECH_PB_PARSE_ERR);
    }
    unsigned long long v;
    memcpy(&v, p->cur, 8);
    p->cur += 8;
    return v;
}

static void rech_pb_read_tag(rech_pbp *p, long long *num, int *wiretype) {
    unsigned long long v = rech_pb_read_varint(p);
    *num = (long long)(v >> 3);
    *wiretype = (int)(v & 0x7);
}

static const unsigned char *rech_pb_read_bytes(rech_pbp *p, long long *len_out) {
    unsigned long long len = rech_pb_read_varint(p);
    if ((unsigned long long)(p->end - p->cur) < len) {
        rech_struct_throw(RECH_PB_PARSE_ERR);
    }
    const unsigned char *start = p->cur;
    p->cur += (long long)len;
    *len_out = (long long)len;
    return start;
}

static void rech_pb_skip(rech_pbp *p, int wiretype) {
    switch (wiretype) {
        case 0:
            rech_pb_read_varint(p);
            break;
        case 1:
            rech_pb_read_fixed64(p);
            break;
        case 2: {
            long long n;
            rech_pb_read_bytes(p, &n);
            break;
        }
        case 5:
            rech_pb_read_fixed32(p);
            break;
        default:
            rech_struct_throw(RECH_PB_PARSE_ERR);
    }
}

`
