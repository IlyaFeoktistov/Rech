// "разобрать XML <выражение> как <Тип>" - симметрично json_extract.go, но
// для XML: см. package doc internal/interp/xml.go для отображения
// Rech-типа на XML (примитив - текст элемента; сущность - дочерние
// элементы по имени поля; Список<T> как поле - все дочерние элементы с
// именем поля без обёртки; Список<T> как цель разбора целиком - ВСЕ
// дочерние элементы корня, имя тега не проверяется). Без атрибутов, без
// пролога/DOCTYPE/комментариев - известные ограничения этого срезa.
//
// C-представление разобранного текста - "rech_xn" (дерево: текст +
// дочерние элементы с именами, в порядке документа), построенное ручным
// рекурсивным парсером (xmlParserCode). Несовпадение выбрасывает
// "ОшибкаРазбора" через общий rech_struct_throw (json_extract.go) - тот же
// канал, что и JSON.
package cbackend

import (
	"fmt"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

func mangleXMLExtractor(name string) string {
	return "rech_xml_extract_" + transliterate(name)
}

var xmlPrimitiveExtractors = map[ir.TypeKind]string{
	ir.KindInt:     "rech_xml_extract_int",
	ir.KindFloat32: "rech_xml_extract_float32",
	ir.KindFloat:   "rech_xml_extract_float64",
	ir.KindString:  "rech_xml_extract_string",
	ir.KindBool:    "rech_xml_extract_bool",
}

type xmlExtractorGen struct {
	ctx       *irGenCtx
	generics  map[string]ir.GenericInstantiation
	generated map[string]bool
	code      strings.Builder
}

// xmlRuntimeCode - см. jsonRuntimeCode (json_extract.go), тот же принцип
// для XML - вызывается, только когда targets непуст.
func xmlRuntimeCode(ctx *irGenCtx, prog *ir.Program, targets []ir.Type) (string, error) {
	generics := make(map[string]ir.GenericInstantiation, len(prog.Generics))
	for _, g := range prog.Generics {
		generics[g.Name] = g
	}
	g := &xmlExtractorGen{ctx: ctx, generics: generics, generated: map[string]bool{}}
	for _, t := range targets {
		if err := g.ensure(t); err != nil {
			return "", err
		}
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "#define RECH_XML_PARSE_ERR \"%s\"\n", escapeCString(rtmsg.XMLParseError))
	sb.WriteString(xmlParserCode)
	sb.WriteString(g.code.String())
	return sb.String(), nil
}

func (g *xmlExtractorGen) ensure(t ir.Type) error {
	key := jsonTypeKey(t) // тот же ключ дедупликации, что и у JSON - формат тут ни при чём, только имя типа
	if g.generated[key] {
		return nil
	}
	g.generated[key] = true

	if name, ok := xmlPrimitiveExtractors[t.Kind]; ok {
		g.code.WriteString(xmlPrimitiveExtractorCode(name, t))
		return nil
	}
	if t.Kind != ir.KindEntity {
		return fmt.Errorf("генерация C из IR: \"разобрать XML\" в неподдержанный тип %s", t)
	}
	if gi, ok := g.generics[t.Entity]; ok && gi.Template == "Список" {
		if err := g.ensure(gi.ElemType); err != nil {
			return err
		}
		g.code.WriteString(g.xmlListExtractorCode(gi))
		return nil
	}
	ent, ok := g.ctx.entities[t.Entity]
	if !ok {
		return fmt.Errorf("генерация C из IR: \"разобрать XML\" в неизвестный тип %q", t.Entity)
	}
	for _, f := range ent.Fields {
		if err := g.ensure(f.Type); err != nil {
			return err
		}
	}
	g.code.WriteString(g.xmlEntityExtractorCode(ent))
	return nil
}

func xmlPrimitiveExtractorCode(name string, t ir.Type) string {
	mismatch := fmt.Sprintf("rech_struct_throw(\"%s\");", escapeCString(rtmsg.XMLValueMismatch))
	switch t.Kind {
	case ir.KindInt:
		return fmt.Sprintf(`static long long %s(rech_xn v) {
    char *end;
    long long n = strtoll(v.text, &end, 10);
    if (end == v.text || *end != '\0') { %s }
    return n;
}

`, name, mismatch)
	case ir.KindFloat32:
		return fmt.Sprintf(`static float %s(rech_xn v) {
    char *end;
    double n = strtod(v.text, &end);
    if (end == v.text || *end != '\0') { %s }
    return (float)n;
}

`, name, mismatch)
	case ir.KindFloat:
		return fmt.Sprintf(`static double %s(rech_xn v) {
    char *end;
    double n = strtod(v.text, &end);
    if (end == v.text || *end != '\0') { %s }
    return n;
}

`, name, mismatch)
	case ir.KindString:
		return fmt.Sprintf(`static const char *%s(rech_xn v) {
    return v.text;
}

`, name)
	default: // ir.KindBool
		return fmt.Sprintf(`static bool %s(rech_xn v) {
    if (strcmp(v.text, "true") == 0) { return true; }
    if (strcmp(v.text, "false") == 0) { return false; }
    %s
    return false;
}

`, name, mismatch)
	}
}

// xmlListExtractorCode - Список<T> как ЦЕЛЬ РАЗБОРА целиком (не поле
// сущности - тот случай обрабатывает xmlEntityExtractorCode напрямую,
// through rech_xn_children_named) - берёт ВСЕ дочерние элементы v, имя
// тега не проверяется (см. package doc).
func (g *xmlExtractorGen) xmlListExtractorCode(gi ir.GenericInstantiation) string {
	listC := mangleType(gi.Name)
	ctor := mangleCtor(gi.Name)
	addFn := mangleMethod(gi.Name, "добавить")
	elemExtractor := xmlExtractorNameFor(gi.ElemType)
	return fmt.Sprintf(`static %[1]s *%[2]s(rech_xn v) {
    %[1]s *out = %[5]s();
    for (long long i = 0; i < v.child_count; i++) {
        %[3]s(out, %[4]s(v.children[i]));
    }
    return out;
}

`, listC, mangleXMLExtractor(gi.Name), addFn, elemExtractor, ctor)
}

func (g *xmlExtractorGen) xmlEntityExtractorCode(ent *ir.Entity) string {
	retC := fromIRType(ir.EntityType(ent.Name)).cType(g.ctx)

	var decls strings.Builder
	args := make([]string, len(ent.Fields))
	for i, f := range ent.Fields {
		fieldC := fromIRType(f.Type).cType(g.ctx)
		matchesVar := fmt.Sprintf("m%d", i)
		countVar := fmt.Sprintf("n%d", i)
		fmt.Fprintf(&decls, "    rech_xn *%s;\n", matchesVar)
		fmt.Fprintf(&decls, "    long long %s = rech_xn_children_named(&v, \"%s\", &%s);\n",
			countVar, escapeCString(f.Name), matchesVar)

		if gi, ok := g.generics[f.Type.Entity]; f.Type.Kind == ir.KindEntity && ok && gi.Template == "Список" {
			addFn := mangleMethod(gi.Name, "добавить")
			elemExtractor := xmlExtractorNameFor(gi.ElemType)
			fmt.Fprintf(&decls, "    %s f%d = %s();\n", fieldC, i, mangleCtor(gi.Name))
			fmt.Fprintf(&decls, "    for (long long j = 0; j < %s; j++) { %s(f%d, %s(%s[j])); }\n",
				countVar, addFn, i, elemExtractor, matchesVar)
			args[i] = fmt.Sprintf("f%d", i)
			continue
		}

		extractor := xmlExtractorNameFor(f.Type)
		missing := fmt.Sprintf("rech_struct_throw(\"%s\");", escapeCString(rtmsg.XMLFieldMissing(f.Name)))
		duplicated := fmt.Sprintf("rech_struct_throw(\"%s\");", escapeCString(rtmsg.XMLFieldDuplicated(f.Name)))
		fmt.Fprintf(&decls, "    if (%s == 0) { %s }\n", countVar, missing)
		fmt.Fprintf(&decls, "    if (%s > 1) { %s }\n", countVar, duplicated)
		fmt.Fprintf(&decls, "    %s f%d = %s(%s[0]);\n", fieldC, i, extractor, matchesVar)
		args[i] = fmt.Sprintf("f%d", i)
	}

	var construct string
	if g.ctx.entityIsRef(ent.Name) {
		// rech_new_X() без аргументов - тот же принцип, что и у JSON
		// (internal/backend/cbackend/json_extract.go, jsonEntityExtractorCode) -
		// собственный "конструктор(...)" у сущности-цели разбора запрещён
		// отдельной проверкой, поля присваиваются напрямую.
		var b strings.Builder
		fmt.Fprintf(&b, "%s p = %s();\n", retC, mangleCtor(ent.Name))
		for i, f := range ent.Fields {
			fmt.Fprintf(&b, "    p->%s = %s;\n", mangleField(f.Name), args[i])
		}
		b.WriteString("    return p;\n")
		construct = b.String()
	} else {
		parts := make([]string, len(ent.Fields))
		for i, f := range ent.Fields {
			parts[i] = fmt.Sprintf(".%s = %s", mangleField(f.Name), args[i])
		}
		initList := strings.Join(parts, ", ")
		if initList == "" {
			initList = "0"
		}
		construct = fmt.Sprintf("return (%s){%s};\n", entityCName(g.ctx, ent.Name), initList)
	}

	return fmt.Sprintf(`static %[1]s %[2]s(rech_xn v) {
%[3]s    %[4]s}

`, retC, mangleXMLExtractor(ent.Name), decls.String(), construct)
}

func xmlExtractorNameFor(t ir.Type) string {
	if name, ok := xmlPrimitiveExtractors[t.Kind]; ok {
		return name
	}
	return mangleXMLExtractor(t.Entity)
}

// xmlSbCode - растущий строковый буфер, общий для разбора (rech_xml_trim/
// escape-декодирование) и сериализации (xml_serialize.go) - эмитится один
// раз, когда нужен хотя бы один из двух (см. ir_generator.go), независимо
// от того, отдельный это тип rech_xml_sb от JSON-аналога (rech_json_sb) -
// программа может использовать только XML, без JSON, значит нельзя
// полагаться на чужой буфер.
const xmlSbCode = `typedef struct { char *buf; long long len; long long cap; } rech_xml_sb;

static void rech_xml_sb_init(rech_xml_sb *sb) {
    sb->cap = 64;
    sb->len = 0;
    sb->buf = malloc((size_t)sb->cap);
}

static void rech_xml_sb_putc(rech_xml_sb *sb, char c) {
    if (sb->len + 4 >= sb->cap) {
        long long ncap = sb->cap * 2;
        char *nb = malloc((size_t)ncap);
        memcpy(nb, sb->buf, (size_t)sb->len);
        sb->buf = nb;
        sb->cap = ncap;
    }
    sb->buf[sb->len++] = c;
}

static void rech_xml_sb_puts(rech_xml_sb *sb, const char *s) {
    for (; *s; s++) {
        rech_xml_sb_putc(sb, *s);
    }
}

static char *rech_xml_sb_finish(rech_xml_sb *sb) {
    sb->buf[sb->len] = '\0';
    return sb->buf;
}

static void rech_xml_sb_utf8(rech_xml_sb *sb, unsigned int cp) {
    if (cp <= 0x7F) {
        rech_xml_sb_putc(sb, (char)cp);
    } else if (cp <= 0x7FF) {
        rech_xml_sb_putc(sb, (char)(0xC0 | (cp >> 6)));
        rech_xml_sb_putc(sb, (char)(0x80 | (cp & 0x3F)));
    } else if (cp <= 0xFFFF) {
        rech_xml_sb_putc(sb, (char)(0xE0 | (cp >> 12)));
        rech_xml_sb_putc(sb, (char)(0x80 | ((cp >> 6) & 0x3F)));
        rech_xml_sb_putc(sb, (char)(0x80 | (cp & 0x3F)));
    } else {
        rech_xml_sb_putc(sb, (char)(0xF0 | (cp >> 18)));
        rech_xml_sb_putc(sb, (char)(0x80 | ((cp >> 12) & 0x3F)));
        rech_xml_sb_putc(sb, (char)(0x80 | ((cp >> 6) & 0x3F)));
        rech_xml_sb_putc(sb, (char)(0x80 | (cp & 0x3F)));
    }
}

`

// xmlParserCode - "rech_xn" (текст + дочерние элементы, каждый с именем
// тега, в порядке документа) и ручной рекурсивный XML-парсер: имя тега/
// атрибута (unicode-буквы, включая кириллицу, "_"/":"/"-"/"."), атрибуты
// синтаксически разбираются и отбрасываются (значений не несут в этом
// срезе), текст - с раскрытием "&amp;"/"&lt;"/"&gt;"/"&quot;"/"&apos;" и
// числовых ссылок "&#NNN;"/"&#xHHH;" - использует rech_xml_sb (xmlSbCode,
// эмитится раньше, см. ir_generator.go). Без пролога/DOCTYPE/комментариев.
const xmlParserCode = `typedef struct rech_xn {
    char *text;
    char **child_names;
    struct rech_xn *children;
    long long child_count;
    long long child_cap;
} rech_xn;

typedef struct { const char *cur; } rech_xmlp;

static void rech_struct_throw(const char *msg);

static void rech_xml_skip_ws(rech_xmlp *p) {
    while (*p->cur == ' ' || *p->cur == '\t' || *p->cur == '\n' || *p->cur == '\r') {
        p->cur++;
    }
}

static bool rech_xml_name_start(unsigned char c) {
    return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_' || c == ':' || c >= 0x80;
}

static bool rech_xml_name_char(unsigned char c) {
    return rech_xml_name_start(c) || (c >= '0' && c <= '9') || c == '-' || c == '.';
}

static char *rech_xml_parse_name(rech_xmlp *p) {
    const char *start = p->cur;
    if (!rech_xml_name_start((unsigned char)*p->cur)) rech_struct_throw(RECH_XML_PARSE_ERR);
    p->cur++;
    while (rech_xml_name_char((unsigned char)*p->cur)) {
        p->cur++;
    }
    long long len = (long long)(p->cur - start);
    char *out = malloc((size_t)len + 1);
    memcpy(out, start, (size_t)len);
    out[len] = '\0';
    return out;
}

static void rech_xml_skip_attr_value(rech_xmlp *p) {
    char q = *p->cur;
    if (q != '"' && q != '\'') rech_struct_throw(RECH_XML_PARSE_ERR);
    p->cur++;
    while (*p->cur && *p->cur != q) {
        p->cur++;
    }
    if (*p->cur != q) rech_struct_throw(RECH_XML_PARSE_ERR);
    p->cur++;
}

static void rech_xml_append_text_char(rech_xmlp *p, rech_xml_sb *sb) {
    if (*p->cur != '&') {
        rech_xml_sb_putc(sb, *p->cur);
        p->cur++;
        return;
    }
    const char *semi = strchr(p->cur, ';');
    if (!semi) rech_struct_throw(RECH_XML_PARSE_ERR);
    long long entlen = (long long)(semi - p->cur - 1);
    const char *ent = p->cur + 1;
    if (entlen == 3 && strncmp(ent, "amp", 3) == 0) {
        rech_xml_sb_putc(sb, '&');
    } else if (entlen == 2 && strncmp(ent, "lt", 2) == 0) {
        rech_xml_sb_putc(sb, '<');
    } else if (entlen == 2 && strncmp(ent, "gt", 2) == 0) {
        rech_xml_sb_putc(sb, '>');
    } else if (entlen == 4 && strncmp(ent, "quot", 4) == 0) {
        rech_xml_sb_putc(sb, '"');
    } else if (entlen == 4 && strncmp(ent, "apos", 4) == 0) {
        rech_xml_sb_putc(sb, '\'');
    } else if (entlen >= 2 && ent[0] == '#' && (ent[1] == 'x' || ent[1] == 'X')) {
        char buf[32];
        long long n = entlen - 2 < 31 ? entlen - 2 : 31;
        memcpy(buf, ent + 2, (size_t)n);
        buf[n] = '\0';
        rech_xml_sb_utf8(sb, (unsigned int)strtoul(buf, NULL, 16));
    } else if (entlen >= 1 && ent[0] == '#') {
        char buf[32];
        long long n = entlen - 1 < 31 ? entlen - 1 : 31;
        memcpy(buf, ent + 1, (size_t)n);
        buf[n] = '\0';
        rech_xml_sb_utf8(sb, (unsigned int)strtoul(buf, NULL, 10));
    } else {
        rech_struct_throw(RECH_XML_PARSE_ERR);
    }
    p->cur = semi + 1;
}

static char *rech_xml_trim(char *s) {
    long long len = (long long)strlen(s);
    long long start = 0;
    while (start < len && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r')) {
        start++;
    }
    long long end = len;
    while (end > start && (s[end - 1] == ' ' || s[end - 1] == '\t' || s[end - 1] == '\n' || s[end - 1] == '\r')) {
        end--;
    }
    long long newlen = end - start;
    char *out = malloc((size_t)newlen + 1);
    memcpy(out, s + start, (size_t)newlen);
    out[newlen] = '\0';
    return out;
}

static void rech_xn_add_child(rech_xn *n, char *name, rech_xn child) {
    if (n->child_count >= n->child_cap) {
        n->child_cap = n->child_cap == 0 ? 4 : n->child_cap * 2;
        n->child_names = realloc(n->child_names, sizeof(char *) * (size_t)n->child_cap);
        n->children = realloc(n->children, sizeof(rech_xn) * (size_t)n->child_cap);
    }
    n->child_names[n->child_count] = name;
    n->children[n->child_count] = child;
    n->child_count++;
}

static long long rech_xn_children_named(rech_xn *n, const char *name, rech_xn **out) {
    long long cap = 4, count = 0;
    rech_xn *matches = malloc(sizeof(rech_xn) * (size_t)cap);
    for (long long i = 0; i < n->child_count; i++) {
        if (strcmp(n->child_names[i], name) == 0) {
            if (count >= cap) {
                cap *= 2;
                matches = realloc(matches, sizeof(rech_xn) * (size_t)cap);
            }
            matches[count++] = n->children[i];
        }
    }
    *out = matches;
    return count;
}

static rech_xn rech_xml_parse_element(rech_xmlp *p, char **out_name) {
    if (*p->cur != '<') rech_struct_throw(RECH_XML_PARSE_ERR);
    p->cur++;
    char *name = rech_xml_parse_name(p);
    *out_name = name;
    for (;;) {
        rech_xml_skip_ws(p);
        if (*p->cur == '\0') rech_struct_throw(RECH_XML_PARSE_ERR);
        if (*p->cur == '/') {
            p->cur++;
            if (*p->cur != '>') rech_struct_throw(RECH_XML_PARSE_ERR);
            p->cur++;
            rech_xn empty;
            empty.text = malloc(1);
            empty.text[0] = '\0';
            empty.child_names = NULL;
            empty.children = NULL;
            empty.child_count = 0;
            empty.child_cap = 0;
            return empty;
        }
        if (*p->cur == '>') {
            p->cur++;
            break;
        }
        rech_xml_parse_name(p);
        rech_xml_skip_ws(p);
        if (*p->cur != '=') rech_struct_throw(RECH_XML_PARSE_ERR);
        p->cur++;
        rech_xml_skip_ws(p);
        rech_xml_skip_attr_value(p);
    }

    rech_xn node;
    node.text = NULL;
    node.child_names = NULL;
    node.children = NULL;
    node.child_count = 0;
    node.child_cap = 0;
    rech_xml_sb text_sb;
    rech_xml_sb_init(&text_sb);
    for (;;) {
        if (*p->cur == '\0') rech_struct_throw(RECH_XML_PARSE_ERR);
        if (*p->cur == '<') {
            if (p->cur[1] == '/') {
                p->cur += 2;
                char *close_name = rech_xml_parse_name(p);
                rech_xml_skip_ws(p);
                if (*p->cur != '>' || strcmp(close_name, name) != 0) rech_struct_throw(RECH_XML_PARSE_ERR);
                p->cur++;
                node.text = rech_xml_trim(rech_xml_sb_finish(&text_sb));
                return node;
            }
            char *child_name;
            rech_xn child = rech_xml_parse_element(p, &child_name);
            rech_xn_add_child(&node, child_name, child);
            continue;
        }
        rech_xml_append_text_char(p, &text_sb);
    }
}

static rech_xn rech_xml_parse(const char *text) {
    rech_xmlp p;
    p.cur = text;
    rech_xml_skip_ws(&p);
    char *name;
    rech_xn root = rech_xml_parse_element(&p, &name);
    rech_xml_skip_ws(&p);
    if (*p.cur != '\0') rech_struct_throw(RECH_XML_PARSE_ERR);
    return root;
}

`
