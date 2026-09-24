package strutil

import (
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/IlyaFeoktistov/Rech/internal/textutf8"
)

// TestTextCodeMatchesGo - C-версия правила (TextCode) даёт ровно то же,
// что internal/textutf8 (interp), на наборе случаев и на случайных байтах.
func TestTextCodeMatchesGo(t *testing.T) {
	cc, err := exec.LookPath("cc")
	if err != nil {
		t.Skip("нет системного компилятора C")
	}
	inputs := []string{"привет", "a\x00b", "\xd0", "\xd0\x41", "\xc0\xaf", "\xed\xa0\x80", "\xf4\x90\x80\x80", "ok\xe2\x82", "\uFFFD", "ab\xe2\x82", "\xf0\x9f\x98", "ab\xd0"}
	rng := rand.New(rand.NewSource(1))
	for range 300 {
		b := make([]byte, rng.Intn(12))
		for i := range b {
			b[i] = byte(rng.Intn(256))
		}
		inputs = append(inputs, string(b))
	}
	dir := t.TempDir()
	var src strings.Builder
	src.WriteString("#include <stdio.h>\n#include <stdlib.h>\n#include <string.h>\n" + TextCode)
	src.WriteString("static void dump(const char *s) { for (; *s; s++) printf(\"%02x\", (unsigned char)*s); printf(\"\\n\"); }\nint main(void) {\n")
	for _, in := range inputs {
		var lit strings.Builder
		for i := 0; i < len(in); i++ {
			lit.WriteString("\\x" + strconv.FormatInt(int64(in[i])|0x100, 16)[1:])
		}
		n := strconv.Itoa(len(in))
		src.WriteString("    { static const unsigned char in[] = \"" + lit.String() + "\"; char *b = malloc(" + n + " + 1); memcpy(b, in, " + n + "); dump(rech_text_take(b, " + n + ")); printf(\"%d\\n\", (int)rech_utf8_incomplete_tail(in, " + n + ")); }\n")
	}
	src.WriteString("    return 0;\n}\n")
	cPath := filepath.Join(dir, "t.c")
	if err := os.WriteFile(cPath, []byte(src.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, "t")
	if out, err := exec.Command(cc, cPath, "-o", bin).CombinedOutput(); err != nil {
		t.Fatalf("cc: %v\n%s", err, out)
	}
	out, err := exec.Command(bin).Output()
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for i, in := range inputs {
		wantHex := ""
		for _, c := range []byte(textutf8.Fix(in)) {
			wantHex += strconv.FormatInt(int64(c)|0x100, 16)[1:]
		}
		if got := strings.TrimSpace(lines[2*i]); got != wantHex {
			t.Errorf("%q: C дал %s, Go - %s", in, got, wantHex)
		}
		if got, want := lines[2*i+1], strconv.Itoa(textutf8.IncompleteTail([]byte(in))); got != want {
			t.Errorf("%q: хвост C %s, Go %s", in, got, want)
		}
	}
}
