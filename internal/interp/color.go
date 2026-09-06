package interp

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/IlyaFeoktistov/Rech/internal/ir"
	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

// hexToColor - "rgbцвет_из_hex" - "#RRGGBB"/"RRGGBB" ("#" необязателен) в
// сущность "RGBЦвет" - байт-в-байт то же правило, что и rech_hex_to_color
// (internal/backend/cbackend/color.go).
func hexToColor(hex string) Value {
	s := strings.TrimPrefix(hex, "#")
	if len(s) != 6 {
		panicRuntime("%s", rtmsg.HexColorInvalidFormat)
	}
	r, errR := strconv.ParseUint(s[0:2], 16, 8)
	g, errG := strconv.ParseUint(s[2:4], 16, 8)
	b, errB := strconv.ParseUint(s[4:6], 16, 8)
	if errR != nil || errG != nil || errB != nil {
		panicRuntime("%s", rtmsg.HexColorInvalidFormat)
	}
	fields := map[string]Value{
		"красный": byteValue(int64(r)), "зелёный": byteValue(int64(g)), "синий": byteValue(int64(b)),
	}
	return Value{Type: ir.EntityType("RGBЦвет"), Obj: &entityObj{typeName: "RGBЦвет", fields: fields}}
}

// colorToHex - "цвет_в_hex", обратное hexToColor.
func colorToHex(c Value) string {
	f := c.Obj.fields
	return fmt.Sprintf("#%02X%02X%02X", f["красный"].I, f["зелёный"].I, f["синий"].I)
}
