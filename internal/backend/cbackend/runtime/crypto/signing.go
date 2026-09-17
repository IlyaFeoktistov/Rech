package crypto

import (
	"fmt"

	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

// SigningHelpers - "сгенерировать_приватный_ключ"/"получить_публичный_
// ключ"/"подписать"/"проверить_подпись" (internal/builtins.All) -
// асимметричная криптография (RSA/Ed25519) через OpenSSL EVP (EVP_PKEY/
// PEM_write_bio_*/EVP_DigestSign/EVP_DigestVerify) - ТОТ ЖЕ довод, что и у
// AESHelpers: настоящая криптография с нуля - риск для безопасности, не
// место для "хватает ~100 строк своего кода" (в отличие от Helpers -
// голых хеш-функций). Использует те же OpenSSL-заголовки, что и AES
// (<openssl/evp.h>) плюс <openssl/pem.h>/<openssl/bio.h> для PEM.
//
// PEM - PKCS#8 (приватный ключ, "PRIVATE KEY")/PKIX (публичный,
// "PUBLIC KEY", он же "SubjectPublicKeyInfo") - тот же формат, что выводит
// Go `encoding/x509`/`encoding/pem` в interp (см. package doc
// internal/interp/signing.go) - ключ, сгенерированный одним backend'ом,
// читается другим, и совместимый с "openssl pkey"/"openssl dgst -sign" и
// подобными настоящими инструментами (то, ради чего выбран именно PEM, а
// не более простой в реализации HEX).
//
// RSA - 2048 бит (rech_rsa_key_bits ниже), не параметр builtin'а - тот же
// принцип минимализма, что и у единственного режима "зашифровать"
// (AES-256-GCM): не заводить абстракцию под гипотетический выбор размера
// ключа, которого никто не просил.
var SigningHelpers = fmt.Sprintf(`static char *rech_bio_to_string(BIO *bio) {
    char *data;
    long len = BIO_get_mem_data(bio, &data);
    char *out = malloc((size_t)len + 1);
    memcpy(out, data, (size_t)len);
    out[len] = '\0';
    return out;
}

static char *rech_generate_private_key_pem(int algorithm) {
    EVP_PKEY *pkey = NULL;
    EVP_PKEY_CTX *pctx = NULL;
    if (algorithm == 0) {
        pctx = EVP_PKEY_CTX_new_id(EVP_PKEY_RSA, NULL);
        if (pctx == NULL || EVP_PKEY_keygen_init(pctx) <= 0 ||
            EVP_PKEY_CTX_set_rsa_keygen_bits(pctx, 2048) <= 0 ||
            EVP_PKEY_keygen(pctx, &pkey) <= 0) {
            rech_panic("%[1]s");
        }
    } else if (algorithm == 1) {
        pctx = EVP_PKEY_CTX_new_id(EVP_PKEY_ED25519, NULL);
        if (pctx == NULL || EVP_PKEY_keygen_init(pctx) <= 0 ||
            EVP_PKEY_keygen(pctx, &pkey) <= 0) {
            rech_panic("%[1]s");
        }
    } else {
        rech_panic("%[2]s");
    }
    EVP_PKEY_CTX_free(pctx);

    BIO *bio = BIO_new(BIO_s_mem());
    if (PEM_write_bio_PrivateKey(bio, pkey, NULL, NULL, 0, NULL, NULL) <= 0) {
        rech_panic("%[1]s");
    }
    char *out = rech_bio_to_string(bio);
    BIO_free(bio);
    EVP_PKEY_free(pkey);
    return out;
}

static EVP_PKEY *rech_read_private_key_pem(const char *private_key_pem) {
    BIO *bio = BIO_new_mem_buf(private_key_pem, -1);
    EVP_PKEY *pkey = PEM_read_bio_PrivateKey(bio, NULL, NULL, NULL);
    BIO_free(bio);
    if (pkey == NULL) {
        rech_panic("%[3]s");
    }
    return pkey;
}

static char *rech_get_public_key_pem(const char *private_key_pem) {
    EVP_PKEY *pkey = rech_read_private_key_pem(private_key_pem);
    BIO *bio = BIO_new(BIO_s_mem());
    if (PEM_write_bio_PUBKEY(bio, pkey) <= 0) {
        EVP_PKEY_free(pkey);
        rech_panic("%[1]s");
    }
    char *out = rech_bio_to_string(bio);
    BIO_free(bio);
    EVP_PKEY_free(pkey);
    return out;
}

static char *rech_sign_hex(const char *data, const char *private_key_pem) {
    EVP_PKEY *pkey = rech_read_private_key_pem(private_key_pem);
    const EVP_MD *md = (EVP_PKEY_id(pkey) == EVP_PKEY_ED25519) ? NULL : EVP_sha256();

    EVP_MD_CTX *mdctx = EVP_MD_CTX_new();
    size_t datalen = strlen(data);
    size_t siglen = 0;
    if (EVP_DigestSignInit(mdctx, NULL, md, NULL, pkey) <= 0 ||
        EVP_DigestSign(mdctx, NULL, &siglen, (const unsigned char *)data, datalen) <= 0) {
        rech_panic("%[1]s");
    }
    unsigned char *sig = malloc(siglen);
    if (EVP_DigestSign(mdctx, sig, &siglen, (const unsigned char *)data, datalen) <= 0) {
        rech_panic("%[1]s");
    }
    EVP_MD_CTX_free(mdctx);
    EVP_PKEY_free(pkey);

    char *out = rech_bytes_to_hex(sig, siglen);
    free(sig);
    return out;
}

static bool rech_verify_signature_hex(const char *data, const char *signature_hex, const char *public_key_pem) {
    BIO *bio = BIO_new_mem_buf(public_key_pem, -1);
    EVP_PKEY *pkey = PEM_read_bio_PUBKEY(bio, NULL, NULL, NULL);
    BIO_free(bio);
    if (pkey == NULL) {
        rech_panic("%[4]s");
    }

    size_t siglen = 0;
    unsigned char *sig = rech_hex_decode(signature_hex, &siglen);
    if (sig == NULL) {
        EVP_PKEY_free(pkey);
        rech_panic("%[5]s");
    }

    const EVP_MD *md = (EVP_PKEY_id(pkey) == EVP_PKEY_ED25519) ? NULL : EVP_sha256();
    EVP_MD_CTX *mdctx = EVP_MD_CTX_new();
    int ok = 0;
    if (EVP_DigestVerifyInit(mdctx, NULL, md, NULL, pkey) > 0) {
        ok = EVP_DigestVerify(mdctx, sig, siglen, (const unsigned char *)data, strlen(data));
    }
    EVP_MD_CTX_free(mdctx);
    EVP_PKEY_free(pkey);
    free(sig);
    return ok == 1;
}
`, rtmsg.SigningKeyGenFailed, rtmsg.UnknownSignatureAlgorithm, rtmsg.SigningInvalidPrivateKey,
	rtmsg.SigningInvalidPublicKey, rtmsg.SigningInvalidSignatureHex)
