package tzdata

import (
	"io/fs"
	"strings"
	"testing"
	"time"
)

// TestSlimTZifPreservesOffsets - ужатый файл (без блока V1) даёт те же
// смещения, что исходный, у каждого вшитого пояса - от 1900 до 2100 года
// (после последнего перехода - по POSIX-хвосту).
func TestSlimTZifPreservesOffsets(t *testing.T) {
	err := fs.WalkDir(zoneinfoFS, "zoneinfo", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		orig, err := fs.ReadFile(zoneinfoFS, path)
		if err != nil {
			return err
		}
		name := strings.TrimPrefix(path, "zoneinfo/")
		slim := slimTZif(orig)
		if orig[4] != 0 && len(slim) >= len(orig) {
			t.Errorf("%s: файл версии 2+ не ужат (%d -> %d байт)", name, len(orig), len(slim))
		}
		want, err := time.LoadLocationFromTZData(name, orig)
		if err != nil {
			t.Errorf("%s: исходный файл не разбирается: %v", name, err)
			return nil
		}
		got, err := time.LoadLocationFromTZData(name, slim)
		if err != nil {
			t.Errorf("%s: ужатый файл не разбирается: %v", name, err)
			return nil
		}
		for ts := time.Date(1900, 1, 1, 0, 0, 0, 0, time.UTC); ts.Year() < 2100; ts = ts.Add(29 * 24 * time.Hour) {
			_, wantOff := ts.In(want).Zone()
			_, gotOff := ts.In(got).Zone()
			if wantOff != gotOff {
				t.Errorf("%s: %s - смещение %d, ожидалось %d", name, ts.Format(time.RFC3339), gotOff, wantOff)
				return nil
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestBlobIndexSortedAndDeduplicated - индекс отсортирован по байтам (тем
// же порядком, что strcmp в rech_tz_load), содержит каждый вшитый пояс,
// одинаковые файлы хранятся в rech_tz_blob один раз.
func TestBlobIndexSortedAndDeduplicated(t *testing.T) {
	c := Blob()
	var names []string
	offsets := map[string]string{}
	for _, line := range strings.Split(c, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "{\"") {
			continue
		}
		fields := strings.Split(strings.Trim(line, "{},"), ", ")
		name := strings.Trim(fields[0], "\"")
		names = append(names, name)
		offsets[name] = fields[1]
	}
	files := 0
	_ = fs.WalkDir(zoneinfoFS, "zoneinfo", func(_ string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			files++
		}
		return err
	})
	if len(names) != files {
		t.Fatalf("в индексе %d поясов, вшито файлов %d", len(names), files)
	}
	for i := 1; i < len(names); i++ {
		if names[i-1] >= names[i] {
			t.Fatalf("индекс не отсортирован: %q перед %q", names[i-1], names[i])
		}
	}
	if offsets["Europe/Helsinki"] == "" || offsets["Europe/Helsinki"] != offsets["Europe/Mariehamn"] {
		t.Fatalf("Europe/Helsinki и Europe/Mariehamn (одинаковые файлы) должны ссылаться на одно смещение, получено %q и %q",
			offsets["Europe/Helsinki"], offsets["Europe/Mariehamn"])
	}
}

// TestWriteCBytesEscapesFixedWidth - непечатные байты и "\\", "\"", "?" -
// трёхзначные восьмеричные последовательности: цифра после них не
// продолжает escape.
func TestWriteCBytesEscapesFixedWidth(t *testing.T) {
	var sb strings.Builder
	writeCBytes(&sb, []byte{0, '1', '"', '\\', '?', 'a', 0xff})
	if got, want := sb.String(), "    \"\\0001\\042\\134\\077a\\377\"\n"; got != want {
		t.Fatalf("получено %q, ожидалось %q", got, want)
	}
}
