package interp

import (
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"

	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

// sha256Hex/hmacSHA256Hex/base64URLEncode/base64URLDecode - "хеш_sha256"/
// "подпись_hmac_sha256"/"закодировать_base64url"/"декодировать_base64url"
// (internal/builtins.All) - стандартная библиотека Go вместо ручной
// реализации (в отличие от cbackend.cryptoHelpers, там нет готового
// crypto.h), результат должен побайтово совпадать между интерпретатором и
// скомпилированной программой на одинаковом входе - см. package doc
// crypto.go в cbackend.
func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// sha1Hex/md5Hex - "хеш_sha1"/"хеш_md5" (docs/ROADMAP.md, "Другие
// криптографические примитивы"). Оба алгоритма криптографически сломаны
// (коллизии) - не для паролей/подписей (для этого уже есть "хеш_sha256"/
// "подпись_hmac_sha256"), а для совместимости с внешними форматами/
// протоколами, которые их требуют "как есть" (Git-объекты - SHA-1,
// контрольные суммы файлов/ETag - оба).
func sha1Hex(s string) string {
	sum := sha1.Sum([]byte(s))
	return hex.EncodeToString(sum[:])
}

func md5Hex(s string) string {
	sum := md5.Sum([]byte(s))
	return hex.EncodeToString(sum[:])
}

func hmacSHA256Hex(data, key string) string {
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(data))
	return hex.EncodeToString(mac.Sum(nil))
}

func base64URLEncode(s string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(s))
}

func base64URLDecode(s string) string {
	out, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		panicRuntime("%s", rtmsg.Base64URLInvalidInput)
	}
	return string(out)
}
