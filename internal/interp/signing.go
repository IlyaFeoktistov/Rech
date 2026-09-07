package interp

import (
	"crypto"
	"crypto/ed25519"
	cryptorand "crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"

	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

// rsaKeyBits - размер генерируемого RSA-ключа (не параметр builtin'а -
// тот же принцип минимализма, что и у "зашифровать" с единственным
// режимом AES-256-GCM: не заводить абстракцию под гипотетический выбор,
// которого никто не просил). 2048 бит - общепринятый современный минимум
// (NIST/CA/Browser Forum), не 1024 (устарел) и не 4096 (заметно медленнее
// генерировать/подписывать без реальной необходимости для примеров языка).
const rsaKeyBits = 2048

// generatePrivateKeyPEM/getPublicKeyPEM/signHex/verifySignatureHex -
// "сгенерировать_приватный_ключ"/"получить_публичный_ключ"/"подписать"/
// "проверить_подпись" (internal/builtins.All). PEM - PKCS#8 (приватный)/
// PKIX (публичный, "SubjectPublicKeyInfo") - тот же формат, что выводит
// OpenSSL EVP в cbackend (см. package doc internal/backend/cbackend/
// signing.go) - ключ, сгенерированный одним backend'ом, читается другим.
func generatePrivateKeyPEM(algorithm int) string {
	var der []byte
	var err error
	switch algorithm {
	case 0: // RSA
		var key *rsa.PrivateKey
		key, err = rsa.GenerateKey(cryptorand.Reader, rsaKeyBits)
		if err == nil {
			der, err = x509.MarshalPKCS8PrivateKey(key)
		}
	case 1: // Ed25519
		var priv ed25519.PrivateKey
		_, priv, err = ed25519.GenerateKey(cryptorand.Reader)
		if err == nil {
			der, err = x509.MarshalPKCS8PrivateKey(priv)
		}
	default:
		panicRuntime("%s", rtmsg.UnknownSignatureAlgorithm)
	}
	if err != nil {
		panicRuntime("%s", rtmsg.SigningKeyGenFailed)
	}
	block := &pem.Block{Type: "PRIVATE KEY", Bytes: der}
	return string(pem.EncodeToMemory(block))
}

func parsePrivateKeyPEM(privateKeyPEM string) crypto.Signer {
	block, _ := pem.Decode([]byte(privateKeyPEM))
	if block == nil {
		panicRuntime("%s", rtmsg.SigningInvalidPrivateKey)
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		panicRuntime("%s", rtmsg.SigningInvalidPrivateKey)
	}
	signer, ok := key.(crypto.Signer)
	if !ok {
		panicRuntime("%s", rtmsg.SigningInvalidPrivateKey)
	}
	return signer
}

func getPublicKeyPEM(privateKeyPEM string) string {
	signer := parsePrivateKeyPEM(privateKeyPEM)
	der, err := x509.MarshalPKIXPublicKey(signer.Public())
	if err != nil {
		panicRuntime("%s", rtmsg.SigningInvalidPrivateKey)
	}
	block := &pem.Block{Type: "PUBLIC KEY", Bytes: der}
	return string(pem.EncodeToMemory(block))
}

func signHex(data, privateKeyPEM string) string {
	signer := parsePrivateKeyPEM(privateKeyPEM)
	var sig []byte
	var err error
	switch key := signer.(type) {
	case ed25519.PrivateKey:
		// PureEdDSA - хеширует сообщение сам, как часть алгоритма -
		// передавать crypto.Hash(0) означает "подпиши сырое сообщение".
		sig, err = key.Sign(cryptorand.Reader, []byte(data), crypto.Hash(0))
	default:
		hashed := sha256.Sum256([]byte(data))
		sig, err = signer.Sign(cryptorand.Reader, hashed[:], crypto.SHA256)
	}
	if err != nil {
		panicRuntime("%s", rtmsg.SigningKeyGenFailed)
	}
	return hex.EncodeToString(sig)
}

func verifySignatureHex(data, signatureHex, publicKeyPEM string) bool {
	block, _ := pem.Decode([]byte(publicKeyPEM))
	if block == nil {
		panicRuntime("%s", rtmsg.SigningInvalidPublicKey)
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		panicRuntime("%s", rtmsg.SigningInvalidPublicKey)
	}
	sig, err := hex.DecodeString(signatureHex)
	if err != nil {
		panicRuntime("%s", rtmsg.SigningInvalidSignatureHex)
	}
	switch key := pub.(type) {
	case ed25519.PublicKey:
		return ed25519.Verify(key, []byte(data), sig)
	case *rsa.PublicKey:
		hashed := sha256.Sum256([]byte(data))
		return rsa.VerifyPKCS1v15(key, crypto.SHA256, hashed[:], sig) == nil
	default:
		panicRuntime("%s", rtmsg.SigningInvalidPublicKey)
		return false
	}
}
