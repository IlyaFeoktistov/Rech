package tzdata

import (
	"embed"
	"encoding/binary"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"sync"
)

// zoneinfoFS - копия IANA tzdata (формат TZif, RFC 8536), вшитая в сам
// компилятор: программа, использующая часовые пояса, получает её внутрь
// своего бинарника (см. Blob) и не зависит ни от установки rech, ни от
// системной /usr/share/zoneinfo (на Windows такой нет вовсе).
//
//go:embed zoneinfo
var zoneinfoFS embed.FS

// tzifHeaderLen - заголовок TZif: "TZif", версия, 15 байт резерва, шесть
// 32-битных счётчиков.
const tzifHeaderLen = 44

var (
	blobOnce sync.Once
	blobC    string
)

// Blob - C-объявления вшитой базы поясов: rech_tz_blob (данные всех поясов
// подряд) и rech_tz_index (имя пояса -> смещение/длина в rech_tz_blob,
// отсортирован по strcmp для двоичного поиска). Строится один раз на
// процесс компилятора.
func Blob() string {
	blobOnce.Do(func() { blobC = buildBlobC(zoneinfoFS) })
	return blobC
}

// zoneEntry - пояс во вшитой базе.
type zoneEntry struct {
	name     string
	off, len int
}

func buildBlobC(fsys fs.FS) string {
	var names []string
	files := map[string][]byte{}
	_ = fs.WalkDir(fsys, "zoneinfo", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(fsys, path)
		if err != nil {
			return err
		}
		name := strings.TrimPrefix(path, "zoneinfo/")
		names = append(names, name)
		files[name] = slimTZif(data)
		return nil
	})
	sort.Strings(names)

	var blob []byte
	offsets := map[string]int{}
	entries := make([]zoneEntry, 0, len(names))
	for _, name := range names {
		data := files[name]
		off, seen := offsets[string(data)]
		if !seen {
			off = len(blob)
			offsets[string(data)] = off
			blob = append(blob, data...)
		}
		entries = append(entries, zoneEntry{name: name, off: off, len: len(data)})
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "static const unsigned char rech_tz_blob[%d] =\n", len(blob)+1)
	writeCBytes(&sb, blob)
	sb.WriteString(";\n\n")
	sb.WriteString("typedef struct {\n    const char *name;\n    unsigned int off;\n    unsigned int len;\n} rech_tz_entry;\n\n")
	sb.WriteString("static const rech_tz_entry rech_tz_index[] = {\n")
	for _, e := range entries {
		fmt.Fprintf(&sb, "    {\"%s\", %du, %du},\n", e.name, e.off, e.len)
	}
	sb.WriteString("};\n\n")
	fmt.Fprintf(&sb, "static const int rech_tz_index_len = %d;\n\n", len(entries))
	return sb.String()
}

// slimTZif - файл TZif версии 2+ без 32-битного блока V1: его счётчики
// обнуляются, данные выбрасываются, 64-битный блок и POSIX-хвост остаются
// как есть - читатель (rech_tz_load) всё равно разбирает только второй
// блок. Файл версии 1 или повреждённый возвращается без изменений.
func slimTZif(data []byte) []byte {
	if len(data) < tzifHeaderLen || string(data[:4]) != "TZif" || data[4] == 0 {
		return data
	}
	count := func(i int) int { return int(binary.BigEndian.Uint32(data[20+4*i:])) }
	isutcnt, isstdcnt, leapcnt, timecnt, typecnt, charcnt := count(0), count(1), count(2), count(3), count(4), count(5)
	v1Len := timecnt*4 + timecnt + typecnt*6 + charcnt + leapcnt*8 + isstdcnt + isutcnt
	v2 := tzifHeaderLen + v1Len
	if v2+tzifHeaderLen > len(data) || string(data[v2:v2+4]) != "TZif" {
		return data
	}
	out := make([]byte, 0, tzifHeaderLen+len(data)-v2)
	out = append(out, data[:20]...)
	out = append(out, make([]byte, 24)...)
	return append(out, data[v2:]...)
}

// writeCBytes - data как строковый литерал C, по строке на 96 байт:
// печатные символы как есть, остальные - трёхзначной восьмеричной
// последовательностью (фиксированная длина - следующая цифра не
// продолжает её, в отличие от "\x").
func writeCBytes(sb *strings.Builder, data []byte) {
	const perLine = 96
	for start := 0; start < len(data); start += perLine {
		end := min(start+perLine, len(data))
		sb.WriteString("    \"")
		for _, b := range data[start:end] {
			if b >= 0x20 && b < 0x7f && b != '"' && b != '\\' && b != '?' {
				sb.WriteByte(b)
			} else {
				fmt.Fprintf(sb, "\\%03o", b)
			}
		}
		sb.WriteString("\"\n")
	}
	if len(data) == 0 {
		sb.WriteString("    \"\"\n")
	}
}
