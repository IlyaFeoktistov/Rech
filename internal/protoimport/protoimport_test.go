package protoimport

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeProto(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestImportBasicMessage(t *testing.T) {
	dir := t.TempDir()
	path := writeProto(t, dir, "a.proto", `
syntax = "proto3";

message Address {
  string street = 1;
  int32 house = 2;
}
`)
	out, err := Import(path)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if !strings.Contains(out, "структура Address") {
		t.Errorf("ожидалась структура Address, получено:\n%s", out)
	}
	if !strings.Contains(out, "street: Строка `protobuf:1`") {
		t.Errorf("ожидалось поле street с protobuf:1, получено:\n%s", out)
	}
	if !strings.Contains(out, "house: Целое `protobuf:2`") {
		t.Errorf("ожидалось поле house с protobuf:2, получено:\n%s", out)
	}
}

func TestImportRepeatedAndNestedMessage(t *testing.T) {
	dir := t.TempDir()
	path := writeProto(t, dir, "a.proto", `
syntax = "proto3";

message User {
  string name = 1;
  repeated string tags = 2;
  Address address = 3;

  message Address {
    string street = 1;
  }
}
`)
	out, err := Import(path)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if !strings.Contains(out, "tags: Список<Строка> `protobuf:2`") {
		t.Errorf("ожидался repeated tags, получено:\n%s", out)
	}
	if !strings.Contains(out, "структура User_Address") {
		t.Errorf("ожидалось приплюснутое имя User_Address, получено:\n%s", out)
	}
	if !strings.Contains(out, "address: User_Address `protobuf:3`") {
		t.Errorf("ожидалась ссылка на вложенный тип, получено:\n%s", out)
	}
}

func TestImportEnumFieldBecomesIntWithComment(t *testing.T) {
	dir := t.TempDir()
	path := writeProto(t, dir, "a.proto", `
syntax = "proto3";

enum Role {
  ROLE_UNKNOWN = 0;
  ROLE_ADMIN = 1;
}

message User {
  Role role = 1;
}
`)
	out, err := Import(path)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if !strings.Contains(out, "перечисление Role") {
		t.Errorf("ожидалось перечисление Role, получено:\n%s", out)
	}
	if !strings.Contains(out, "role: Целое `protobuf:1`") {
		t.Errorf("ожидалось поле role типа Целое, получено:\n%s", out)
	}
}

func TestImportLocalImportMerged(t *testing.T) {
	dir := t.TempDir()
	writeProto(t, dir, "common.proto", `
syntax = "proto3";
message Address {
  string street = 1;
}
`)
	path := writeProto(t, dir, "main.proto", `
syntax = "proto3";
import "common.proto";
message User {
  Address address = 1;
}
`)
	out, err := Import(path)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if !strings.Contains(out, "структура Address") || !strings.Contains(out, "структура User") {
		t.Errorf("ожидались оба сообщения, получено:\n%s", out)
	}
}

func TestImportRejectsOneof(t *testing.T) {
	dir := t.TempDir()
	path := writeProto(t, dir, "a.proto", `
syntax = "proto3";
message M { oneof x { string a = 1; int32 b = 2; } }
`)
	if _, err := Import(path); err == nil || !strings.Contains(err.Error(), "oneof") {
		t.Fatalf("ожидалась ошибка про oneof, получено: %v", err)
	}
}

func TestImportRejectsMap(t *testing.T) {
	dir := t.TempDir()
	path := writeProto(t, dir, "a.proto", `
syntax = "proto3";
message M { map<string, int32> m = 1; }
`)
	if _, err := Import(path); err == nil || !strings.Contains(err.Error(), "map<") {
		t.Fatalf("ожидалась ошибка про map, получено: %v", err)
	}
}

func TestImportRejectsUnsupportedWireTypes(t *testing.T) {
	for _, typ := range []string{"sint32", "sint64", "fixed32", "fixed64", "sfixed32", "sfixed64"} {
		dir := t.TempDir()
		path := writeProto(t, dir, "a.proto", `
syntax = "proto3";
message M { `+typ+` x = 1; }
`)
		if _, err := Import(path); err == nil || !strings.Contains(err.Error(), typ) {
			t.Errorf("%s: ожидалась ошибка о неподдержанном типе, получено: %v", typ, err)
		}
	}
}

func TestImportRejectsWellKnownImport(t *testing.T) {
	dir := t.TempDir()
	path := writeProto(t, dir, "a.proto", `
syntax = "proto3";
import "google/protobuf/timestamp.proto";
message M { int32 x = 1; }
`)
	if _, err := Import(path); err == nil || !strings.Contains(err.Error(), "well-known") {
		t.Fatalf("ожидалась ошибка про well-known import, получено: %v", err)
	}
}

func TestImportRejectsProto2(t *testing.T) {
	dir := t.TempDir()
	path := writeProto(t, dir, "a.proto", `
syntax = "proto2";
message M { required int32 x = 1; }
`)
	if _, err := Import(path); err == nil || !strings.Contains(err.Error(), "proto3") {
		t.Fatalf("ожидалась ошибка про proto2/proto3, получено: %v", err)
	}
}

func TestImportRejectsDuplicateFieldNumberIsAllowedAcrossMessages(t *testing.T) {
	// Номера полей уникальны ТОЛЬКО внутри одного сообщения, не глобально -
	// это не ошибка.
	dir := t.TempDir()
	path := writeProto(t, dir, "a.proto", `
syntax = "proto3";
message A { string x = 1; }
message B { string y = 1; }
`)
	if _, err := Import(path); err != nil {
		t.Fatalf("Import: %v", err)
	}
}
