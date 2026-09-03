package cbackend

import (
	"fmt"

	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

// cryptoHelpers - "хеш_sha256"/"подпись_hmac_sha256"/
// "закодировать_base64url"/"декодировать_base64url" (internal/builtins.All,
// docs/ROADMAP.md - "Нет крипто-хеш-функции"). Собственная реализация
// SHA-256 (FIPS 180-4), не биндинг к OpenSSL/libcrypto - тот же принцип,
// что и у randomHelpers/датHelpers (не тянуть внешнюю библиотеку туда, где
// хватает ~100 строк стандартного алгоритма, не привязанного к платформе).
// HMAC-SHA256 (FIPS 198-1) построен поверх него - основа подписи токена
// (алгоритм HS256 в JWT). "rech_bytes_to_hex" - общий хвост обеих
// хеш-функций: результат - HEX, не сырые байты (Rech "Строка" в cbackend -
// "const char *" без отдельно хранимой длины, сырой 32-байтовый дайджест
// мог бы содержать 0x00 внутри и потерять хвост при любой операции,
// использующей strlen). Buffer выделяется через malloc, не GC_MALLOC - тем
// же принципом, что и stringOpsHelpers/randomHelpers: программа, вызвавшая
// только эти функции, не должна тянуть за собой Boehm GC (<gc.h>).
var cryptoHelpers = fmt.Sprintf(`static const uint32_t rech_sha256_k[64] = {
    0x428a2f98,0x71374491,0xb5c0fbcf,0xe9b5dba5,0x3956c25b,0x59f111f1,0x923f82a4,0xab1c5ed5,
    0xd807aa98,0x12835b01,0x243185be,0x550c7dc3,0x72be5d74,0x80deb1fe,0x9bdc06a7,0xc19bf174,
    0xe49b69c1,0xefbe4786,0x0fc19dc6,0x240ca1cc,0x2de92c6f,0x4a7484aa,0x5cb0a9dc,0x76f988da,
    0x983e5152,0xa831c66d,0xb00327c8,0xbf597fc7,0xc6e00bf3,0xd5a79147,0x06ca6351,0x14292967,
    0x27b70a85,0x2e1b2138,0x4d2c6dfc,0x53380d13,0x650a7354,0x766a0abb,0x81c2c92e,0x92722c85,
    0xa2bfe8a1,0xa81a664b,0xc24b8b70,0xc76c51a3,0xd192e819,0xd6990624,0xf40e3585,0x106aa070,
    0x19a4c116,0x1e376c08,0x2748774c,0x34b0bcb5,0x391c0cb3,0x4ed8aa4a,0x5b9cca4f,0x682e6ff3,
    0x748f82ee,0x78a5636f,0x84c87814,0x8cc70208,0x90befffa,0xa4506ceb,0xbef9a3f7,0xc67178f2,
};

typedef struct {
    uint32_t state[8];
    uint64_t bitlen;
    unsigned char buffer[64];
    size_t buflen;
} rech_sha256_ctx;

#define RECH_SHA256_ROTR(x, n) (((x) >> (n)) | ((x) << (32 - (n))))

static void rech_sha256_transform(rech_sha256_ctx *ctx, const unsigned char *data) {
    uint32_t m[64];
    for (int i = 0; i < 16; i++) {
        m[i] = ((uint32_t)data[i * 4] << 24) | ((uint32_t)data[i * 4 + 1] << 16) |
               ((uint32_t)data[i * 4 + 2] << 8) | (uint32_t)data[i * 4 + 3];
    }
    for (int i = 16; i < 64; i++) {
        uint32_t s0 = RECH_SHA256_ROTR(m[i - 15], 7) ^ RECH_SHA256_ROTR(m[i - 15], 18) ^ (m[i - 15] >> 3);
        uint32_t s1 = RECH_SHA256_ROTR(m[i - 2], 17) ^ RECH_SHA256_ROTR(m[i - 2], 19) ^ (m[i - 2] >> 10);
        m[i] = m[i - 16] + s0 + m[i - 7] + s1;
    }
    uint32_t a = ctx->state[0], b = ctx->state[1], c = ctx->state[2], d = ctx->state[3];
    uint32_t e = ctx->state[4], f = ctx->state[5], g = ctx->state[6], h = ctx->state[7];
    for (int i = 0; i < 64; i++) {
        uint32_t s1 = RECH_SHA256_ROTR(e, 6) ^ RECH_SHA256_ROTR(e, 11) ^ RECH_SHA256_ROTR(e, 25);
        uint32_t ch = (e & f) ^ ((~e) & g);
        uint32_t t1 = h + s1 + ch + rech_sha256_k[i] + m[i];
        uint32_t s0 = RECH_SHA256_ROTR(a, 2) ^ RECH_SHA256_ROTR(a, 13) ^ RECH_SHA256_ROTR(a, 22);
        uint32_t maj = (a & b) ^ (a & c) ^ (b & c);
        uint32_t t2 = s0 + maj;
        h = g; g = f; f = e; e = d + t1;
        d = c; c = b; b = a; a = t1 + t2;
    }
    ctx->state[0] += a; ctx->state[1] += b; ctx->state[2] += c; ctx->state[3] += d;
    ctx->state[4] += e; ctx->state[5] += f; ctx->state[6] += g; ctx->state[7] += h;
}

static void rech_sha256_init(rech_sha256_ctx *ctx) {
    ctx->bitlen = 0;
    ctx->buflen = 0;
    ctx->state[0] = 0x6a09e667; ctx->state[1] = 0xbb67ae85;
    ctx->state[2] = 0x3c6ef372; ctx->state[3] = 0xa54ff53a;
    ctx->state[4] = 0x510e527f; ctx->state[5] = 0x9b05688c;
    ctx->state[6] = 0x1f83d9ab; ctx->state[7] = 0x5be0cd19;
}

static void rech_sha256_update(rech_sha256_ctx *ctx, const unsigned char *data, size_t len) {
    for (size_t i = 0; i < len; i++) {
        ctx->buffer[ctx->buflen++] = data[i];
        if (ctx->buflen == 64) {
            rech_sha256_transform(ctx, ctx->buffer);
            ctx->bitlen += 512;
            ctx->buflen = 0;
        }
    }
}

static void rech_sha256_final(rech_sha256_ctx *ctx, unsigned char out[32]) {
    size_t i = ctx->buflen;
    ctx->bitlen += (uint64_t)ctx->buflen * 8;
    ctx->buffer[i++] = 0x80;
    if (i > 56) {
        while (i < 64) {
            ctx->buffer[i++] = 0;
        }
        rech_sha256_transform(ctx, ctx->buffer);
        i = 0;
    }
    while (i < 56) {
        ctx->buffer[i++] = 0;
    }
    for (int j = 7; j >= 0; j--) {
        ctx->buffer[i++] = (unsigned char)((ctx->bitlen >> (j * 8)) & 0xff);
    }
    rech_sha256_transform(ctx, ctx->buffer);
    for (int j = 0; j < 8; j++) {
        out[j * 4] = (unsigned char)((ctx->state[j] >> 24) & 0xff);
        out[j * 4 + 1] = (unsigned char)((ctx->state[j] >> 16) & 0xff);
        out[j * 4 + 2] = (unsigned char)((ctx->state[j] >> 8) & 0xff);
        out[j * 4 + 3] = (unsigned char)(ctx->state[j] & 0xff);
    }
}

static char *rech_bytes_to_hex(const unsigned char *data, size_t len) {
    static const char hexd[] = "0123456789abcdef";
    char *out = malloc(len * 2 + 1);
    for (size_t i = 0; i < len; i++) {
        out[i * 2] = hexd[(data[i] >> 4) & 0xf];
        out[i * 2 + 1] = hexd[data[i] & 0xf];
    }
    out[len * 2] = '\0';
    return out;
}

static char *rech_sha256_hex(const char *s) {
    rech_sha256_ctx ctx;
    rech_sha256_init(&ctx);
    rech_sha256_update(&ctx, (const unsigned char *)s, strlen(s));
    unsigned char digest[32];
    rech_sha256_final(&ctx, digest);
    return rech_bytes_to_hex(digest, 32);
}

// rech_hmac_sha256_hex - HMAC-SHA256 (FIPS 198-1), тот же алгоритм подписи,
// что и у HS256 в JWT. Ключ длиннее блока (64 байта) сначала сжимается
// самим SHA-256, короче - дополняется нулями справа.
static char *rech_hmac_sha256_hex(const char *data, const char *key) {
    unsigned char k[64];
    memset(k, 0, sizeof(k));
    size_t keylen = strlen(key);
    if (keylen > 64) {
        rech_sha256_ctx kctx;
        rech_sha256_init(&kctx);
        rech_sha256_update(&kctx, (const unsigned char *)key, keylen);
        rech_sha256_final(&kctx, k);
    } else {
        memcpy(k, key, keylen);
    }
    unsigned char ipad[64], opad[64];
    for (int i = 0; i < 64; i++) {
        ipad[i] = (unsigned char)(k[i] ^ 0x36);
        opad[i] = (unsigned char)(k[i] ^ 0x5c);
    }
    rech_sha256_ctx ictx;
    rech_sha256_init(&ictx);
    rech_sha256_update(&ictx, ipad, 64);
    rech_sha256_update(&ictx, (const unsigned char *)data, strlen(data));
    unsigned char inner[32];
    rech_sha256_final(&ictx, inner);

    rech_sha256_ctx octx;
    rech_sha256_init(&octx);
    rech_sha256_update(&octx, opad, 64);
    rech_sha256_update(&octx, inner, 32);
    unsigned char outer[32];
    rech_sha256_final(&octx, outer);
    return rech_bytes_to_hex(outer, 32);
}

// rech_base64url_encode/rech_base64url_decode - RFC 4648 §5, БЕЗ
// "="-паддинга (тот же алфавит, что несут части JWT) - только для текста
// без встроенных нулевых байт (та же оговорка, что у HEX-выбора выше:
// "Строка" в cbackend - "const char *", strlen решает длину входа).
static char *rech_base64url_encode(const char *s) {
    static const char tbl[] = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_";
    size_t len = strlen(s);
    const unsigned char *data = (const unsigned char *)s;
    char *out = malloc(((len + 2) / 3) * 4 + 1);
    size_t oi = 0, i;
    for (i = 0; i + 3 <= len; i += 3) {
        uint32_t n = ((uint32_t)data[i] << 16) | ((uint32_t)data[i + 1] << 8) | (uint32_t)data[i + 2];
        out[oi++] = tbl[(n >> 18) & 0x3f];
        out[oi++] = tbl[(n >> 12) & 0x3f];
        out[oi++] = tbl[(n >> 6) & 0x3f];
        out[oi++] = tbl[n & 0x3f];
    }
    size_t rem = len - i;
    if (rem == 1) {
        uint32_t n = (uint32_t)data[i] << 16;
        out[oi++] = tbl[(n >> 18) & 0x3f];
        out[oi++] = tbl[(n >> 12) & 0x3f];
    } else if (rem == 2) {
        uint32_t n = ((uint32_t)data[i] << 16) | ((uint32_t)data[i + 1] << 8);
        out[oi++] = tbl[(n >> 18) & 0x3f];
        out[oi++] = tbl[(n >> 12) & 0x3f];
        out[oi++] = tbl[(n >> 6) & 0x3f];
    }
    out[oi] = '\0';
    return out;
}

static int rech_base64url_val(char c) {
    if (c >= 'A' && c <= 'Z') return c - 'A';
    if (c >= 'a' && c <= 'z') return c - 'a' + 26;
    if (c >= '0' && c <= '9') return c - '0' + 52;
    if (c == '-') return 62;
    if (c == '_') return 63;
    return -1;
}

static char *rech_base64url_decode(const char *s) {
    size_t len = strlen(s);
    char *out = malloc(len + 1);
    size_t oi = 0;
    int vals[4], vi = 0;
    for (size_t i = 0; i < len; i++) {
        int v = rech_base64url_val(s[i]);
        if (v < 0) {
            rech_panic("%[1]s");
        }
        vals[vi++] = v;
        if (vi == 4) {
            out[oi++] = (char)((vals[0] << 2) | (vals[1] >> 4));
            out[oi++] = (char)(((vals[1] & 0xf) << 4) | (vals[2] >> 2));
            out[oi++] = (char)(((vals[2] & 0x3) << 6) | vals[3]);
            vi = 0;
        }
    }
    if (vi == 1) {
        rech_panic("%[1]s");
    } else if (vi == 2) {
        out[oi++] = (char)((vals[0] << 2) | (vals[1] >> 4));
    } else if (vi == 3) {
        out[oi++] = (char)((vals[0] << 2) | (vals[1] >> 4));
        out[oi++] = (char)(((vals[1] & 0xf) << 4) | (vals[2] >> 2));
    }
    out[oi] = '\0';
    return out;
}
`, rtmsg.Base64URLInvalidInput)
