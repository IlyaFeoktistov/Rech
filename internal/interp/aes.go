package interp

import (
	"crypto/aes"
	"crypto/cipher"
	cryptorand "crypto/rand"
	"crypto/sha256"
	"encoding/hex"

	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

// aesEncryptHex/aesDecryptHex - "зашифровать"/"расшифровать"
// (internal/builtins.All) - AES-256-GCM (аутентифицированное шифрование)
// через стандартную библиотеку Go, тем же принципом, что и у остальных
// crypto.go функций (в cbackend - через OpenSSL EVP, не собственная
// реализация, см. package doc internal/backend/cbackend/aes.go - для
// АУТЕНТИФИЦИРОВАННОГО шифрования, в отличие от голых хеш-функций,
// собственный код - риск для безопасности).
//
// "ключ" - строка любой длины, не обязательно 32 байта - её SHA-256
// используется как настоящий 256-битный ключ AES (тот же приём, что и у
// HMAC-ключа длиннее блока в hmacSHA256Hex).
//
// Формат вывода "зашифровать" - HEX(nonce(12 байт) || шифротекст ||
// тег(16 байт)) - Seal уже дописывает тег в конец шифротекста сам (Go
// cipher.AEAD), nonce приписывается спереди явно - "расшифровать"
// принимает эту строку целиком, без отдельных параметров для nonce/тега.
// Байтовая раскладка ДОЛЖНА совпадать с cbackend.rech_aes_encrypt_hex -
// зашифрованное одним backend'ом должно расшифровываться другим.
func aesKeyFromPassphrase(passphrase string) []byte {
	key := sha256.Sum256([]byte(passphrase))
	return key[:]
}

func aesEncryptHex(plaintext, passphrase string) string {
	block, err := aes.NewCipher(aesKeyFromPassphrase(passphrase))
	if err != nil {
		panicRuntime("%s", err.Error())
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		panicRuntime("%s", err.Error())
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := cryptorand.Read(nonce); err != nil {
		panicRuntime("%s", rtmsg.AESRandomFailed)
	}
	sealed := gcm.Seal(nil, nonce, []byte(plaintext), nil)
	return hex.EncodeToString(append(nonce, sealed...))
}

func aesDecryptHex(ciphertextHex, passphrase string) string {
	raw, err := hex.DecodeString(ciphertextHex)
	if err != nil {
		panicRuntime("%s", rtmsg.AESCiphertextInvalid)
	}
	block, err := aes.NewCipher(aesKeyFromPassphrase(passphrase))
	if err != nil {
		panicRuntime("%s", err.Error())
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		panicRuntime("%s", err.Error())
	}
	nonceSize := gcm.NonceSize()
	if len(raw) < nonceSize+gcm.Overhead() {
		panicRuntime("%s", rtmsg.AESCiphertextInvalid)
	}
	nonce, sealed := raw[:nonceSize], raw[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, sealed, nil)
	if err != nil {
		panicRuntime("%s", rtmsg.AESAuthenticationFailed)
	}
	return string(plaintext)
}
