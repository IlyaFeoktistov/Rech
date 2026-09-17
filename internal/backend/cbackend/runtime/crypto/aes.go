package crypto

import (
	"fmt"

	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

// AESHelpers - "зашифровать"/"расшифровать" (internal/builtins.All) -
// AES-256-GCM через OpenSSL EVP (<openssl/evp.h>, RAND_bytes из
// <openssl/rand.h> для nonce) - В ОТЛИЧИЕ от Helpers (crypto.go,
// собственная реализация SHA-256/SHA-1/MD5/SHA-512): аутентифицированное
// шифрование с нуля - риск для безопасности, тот же довод, что у tls.go
// про собственный TLS-стек. OpenSSL уже слинкован ради TLS ("слушать_tcp_
// защищённо") - не новая внешняя зависимость (-lcrypto - internal/
// toolchain/cc.go, по маркеру "#include <openssl/evp.h>").
//
// Ключ AES - SHA-256 переданного пароля (rech_sha256_ctx/_init/_update/
// _final из Helpers, crypto.go - ВСЕГДА эмитится вместе с этим
// файлом, см. builtins_shared.go: usesAES тянет usesCrypto=true), не сам
// пароль напрямую - строка любой длины превращается в настоящий 256-битный
// ключ, тем же приёмом, что и у HMAC-ключа длиннее блока.
//
// Формат вывода "зашифровать" - HEX(nonce(12 байт) || шифротекст ||
// тег(16 байт)) - "расшифровать" принимает эту строку целиком. Байтовая
// раскладка ДОЛЖНА совпадать с interp.aesEncryptHex (internal/interp/
// aes.go) - зашифрованное одним backend'ом должно расшифровываться
// другим. "rech_hex_decode" (обратная к rech_bytes_to_hex) - в crypto.go,
// не здесь - нужна и "расшифровать" тут, и "проверить_подпись" (signing.go).
var AESHelpers = fmt.Sprintf(`static void rech_aes_key_from_passphrase(const char *passphrase, unsigned char key[32]) {
    rech_sha256_ctx kctx;
    rech_sha256_init(&kctx);
    rech_sha256_update(&kctx, (const unsigned char *)passphrase, strlen(passphrase));
    rech_sha256_final(&kctx, key);
}

static char *rech_aes_encrypt_hex(const char *plaintext, const char *passphrase) {
    unsigned char key[32];
    rech_aes_key_from_passphrase(passphrase, key);

    unsigned char nonce[12];
    if (RAND_bytes(nonce, sizeof(nonce)) != 1) {
        rech_panic("%[1]s");
    }

    size_t plen = strlen(plaintext);
    unsigned char *ciphertext = malloc(plen > 0 ? plen : 1);
    unsigned char tag[16];
    int outlen = 0, tmplen = 0;

    EVP_CIPHER_CTX *ctx = EVP_CIPHER_CTX_new();
    EVP_EncryptInit_ex(ctx, EVP_aes_256_gcm(), NULL, NULL, NULL);
    EVP_CIPHER_CTX_ctrl(ctx, EVP_CTRL_GCM_SET_IVLEN, (int)sizeof(nonce), NULL);
    EVP_EncryptInit_ex(ctx, NULL, NULL, key, nonce);
    EVP_EncryptUpdate(ctx, ciphertext, &outlen, (const unsigned char *)plaintext, (int)plen);
    EVP_EncryptFinal_ex(ctx, ciphertext + outlen, &tmplen);
    outlen += tmplen;
    EVP_CIPHER_CTX_ctrl(ctx, EVP_CTRL_GCM_GET_TAG, (int)sizeof(tag), tag);
    EVP_CIPHER_CTX_free(ctx);

    size_t totallen = sizeof(nonce) + (size_t)outlen + sizeof(tag);
    unsigned char *combined = malloc(totallen);
    memcpy(combined, nonce, sizeof(nonce));
    memcpy(combined + sizeof(nonce), ciphertext, (size_t)outlen);
    memcpy(combined + sizeof(nonce) + (size_t)outlen, tag, sizeof(tag));
    free(ciphertext);

    char *result = rech_bytes_to_hex(combined, totallen);
    free(combined);
    return result;
}

static char *rech_aes_decrypt_hex(const char *ciphertext_hex, const char *passphrase) {
    size_t rawlen = 0;
    unsigned char *raw = rech_hex_decode(ciphertext_hex, &rawlen);
    if (raw == NULL || rawlen < 12 + 16) {
        if (raw != NULL) {
            free(raw);
        }
        rech_panic("%[2]s");
    }

    unsigned char *nonce = raw;
    unsigned char *ciphertext = raw + 12;
    size_t ctlen = rawlen - 12 - 16;
    unsigned char *tag = raw + 12 + ctlen;

    unsigned char key[32];
    rech_aes_key_from_passphrase(passphrase, key);

    unsigned char *plaintext = malloc(ctlen + 1);
    int outlen = 0, tmplen = 0;

    EVP_CIPHER_CTX *ctx = EVP_CIPHER_CTX_new();
    EVP_DecryptInit_ex(ctx, EVP_aes_256_gcm(), NULL, NULL, NULL);
    EVP_CIPHER_CTX_ctrl(ctx, EVP_CTRL_GCM_SET_IVLEN, 12, NULL);
    EVP_DecryptInit_ex(ctx, NULL, NULL, key, nonce);
    EVP_DecryptUpdate(ctx, plaintext, &outlen, ciphertext, (int)ctlen);
    EVP_CIPHER_CTX_ctrl(ctx, EVP_CTRL_GCM_SET_TAG, 16, tag);
    int ok = EVP_DecryptFinal_ex(ctx, plaintext + outlen, &tmplen);
    EVP_CIPHER_CTX_free(ctx);
    free(raw);

    if (ok <= 0) {
        free(plaintext);
        rech_panic("%[3]s");
    }
    outlen += tmplen;
    plaintext[outlen] = '\0';
    return (char *)plaintext;
}
`, rtmsg.AESRandomFailed, rtmsg.AESCiphertextInvalid, rtmsg.AESAuthenticationFailed)
