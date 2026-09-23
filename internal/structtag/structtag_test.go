package structtag

import "testing"

func TestParseMultipleKeys(t *testing.T) {
	tag := Parse("json:name, sql:user_name")
	if got := tag.Get("json"); got != "name" {
		t.Fatalf("json: ожидалось %q, получено %q", "name", got)
	}
	if got := tag.Get("sql"); got != "user_name" {
		t.Fatalf("sql: ожидалось %q, получено %q", "user_name", got)
	}
}

func TestParseEmpty(t *testing.T) {
	if tag := Parse(""); tag != nil {
		t.Fatalf("пустой тег должен разбираться в nil, получено %v", tag)
	}
}

func TestParseUnknownKeyLookup(t *testing.T) {
	tag := Parse("json:name")
	if got := tag.Get("sql"); got != "" {
		t.Fatalf("отсутствующий ключ должен вернуть \"\", получено %q", got)
	}
}

func TestXMLName(t *testing.T) {
	tests := []struct {
		raw, field string
		name       string
		attr       bool
	}{
		{"", "имя", "имя", false},
		{"json:name", "имя", "имя", false},
		{"xml:title", "имя", "title", false},
		{"xml:@id", "ид", "id", true},
		{"xml:@", "валюта", "валюта", true},
		{"json:x, xml:@y", "поле", "y", true},
	}
	for _, tt := range tests {
		name, attr := XMLName(tt.raw, tt.field)
		if name != tt.name || attr != tt.attr {
			t.Errorf("XMLName(%q, %q) = (%q, %v), ожидалось (%q, %v)", tt.raw, tt.field, name, attr, tt.name, tt.attr)
		}
	}
}
