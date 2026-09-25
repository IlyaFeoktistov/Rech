package interp

import (
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha3"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"

	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

// sha256Hex/hmacSHA256Hex/base64URLEncode/base64URLDecode - реализация
// builtin'ов "хеш"/"подпись_hmac_sha256"/"закодировать_base64url"/
// "декодировать_base64url" (internal/builtins.All) - стандартная
// библиотека Go вместо ручной реализации (в отличие от
// cbackend.cryptoHelpers, там нет готового crypto.h), результат должен
// побайтово совпадать между интерпретатором и скомпилированной программой
// на одинаковом входе - см. package doc crypto.go в cbackend.
func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// sha1Hex/md5Hex - алгоритмы SHA1/MD5 перечисления "АлгоритмХеша"
// (internal/semantic/types.go, StandardEnums). Оба криптографически
// сломаны (коллизии) - не для паролей/подписей (для этого - SHA256/
// SHA512 того же перечисления), а для совместимости с внешними
// форматами/протоколами, которые их требуют "как есть" (Git-объекты -
// SHA-1, контрольные суммы файлов/ETag - оба).
func sha1Hex(s string) string {
	sum := sha1.Sum([]byte(s))
	return hex.EncodeToString(sum[:])
}

func md5Hex(s string) string {
	sum := md5.Sum([]byte(s))
	return hex.EncodeToString(sum[:])
}

// sha512Hex - алгоритм SHA512, тем же принципом, что и SHA256 (не сломан,
// годится для тех же случаев - паролей/подписей/контрольных сумм - просто
// более длинный дайджест).
func sha512Hex(s string) string {
	sum := sha512.Sum512([]byte(s))
	return hex.EncodeToString(sum[:])
}

// sha3256Hex - алгоритм SHA3_256 (FIPS 202) - sponge-конструкция на базе
// перестановки Keccak-f[1600], принципиально другая, чем Меркла-Дамгора у
// SHA-256/512 выше - НЕ сломан, годится для новых паролей/подписей, как
// SHA256/SHA512 (не замена им, самостоятельный алгоритм).
func sha3256Hex(s string) string {
	sum := sha3.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// hashByAlgorithm - диспетчер builtin'а "хеш(алгоритм, данные)" по
// значению члена перечисления "АлгоритмХеша" (уже развёрнутого в Целое на
// этапе семантического анализа, docs/SPEC.md - "Перечисления": член
// перечисления - обычный компилируемый IntLiteral, рантайм не видит имени
// члена). Индексы должны буквально совпадать с internal/semantic/
// types.go, StandardEnums["АлгоритмХеша"] - это тот же контракт, что и у
// signalFromIndex в signals.go, но здесь всего одна точка определения
// (там - таблица индекс->макрос для двух backend'ов, здесь - таблица
// индекс->функция только в interp; в cbackend - свой rech_hash_hex с той
// же нумерацией, см. package doc crypto.go там).
func hashByAlgorithm(algorithm int, data string) string {
	switch algorithm {
	case 0:
		return sha256Hex(data)
	case 1:
		return sha1Hex(data)
	case 2:
		return md5Hex(data)
	case 3:
		return sha512Hex(data)
	case 4:
		return sha3256Hex(data)
	default:
		panicRuntime("%s", rtmsg.UnknownHashAlgorithm)
		return ""
	}
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
		throwRuntimeError(rtmsg.ErrorParse, "%s", rtmsg.Base64URLInvalidInput)
	}
	return string(out)
}
