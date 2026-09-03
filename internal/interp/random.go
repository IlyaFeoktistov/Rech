package interp

import (
	cryptorand "crypto/rand"
	"encoding/binary"

	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

// randomUint64/randomFloat/randomInt - реализация "случайное"/
// "случайное_целое" (internal/builtins.All) - crypto/rand, тот же
// криптографически стойкий источник, что и arc4random в cbackend (см.
// package doc random.go там) - не math/rand.
func randomUint64() uint64 {
	var buf [8]byte
	if _, err := cryptorand.Read(buf[:]); err != nil {
		panicRuntime("%s", err.Error())
	}
	return binary.LittleEndian.Uint64(buf[:])
}

func randomFloat() float64 {
	// 53 бита - вся мантисса float64, тем же принципом, что и у
	// стандартного math/rand.Float64.
	return float64(randomUint64()>>11) / float64(uint64(1)<<53)
}

func randomInt(mn, mx int64) int64 {
	if mx < mn {
		panicRuntime("%s", rtmsg.RandomIntInvalidRange)
	}
	rangeSize := uint64(mx-mn) + 1
	return mn + int64(randomUint64()%rangeSize)
}
