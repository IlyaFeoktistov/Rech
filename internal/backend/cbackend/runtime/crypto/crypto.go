package crypto

import (
	"fmt"

	"github.com/IlyaFeoktistov/Rech/internal/rtmsg"
)

// Helpers - "хеш(алгоритм, данные)"/"подпись_hmac_sha256"/
// "закодировать_base64url"/"декодировать_base64url" (internal/builtins.All,
// "Нет крипто-хеш-функции"). Собственная реализация
// SHA-256/SHA-1/MD5/SHA-512 (FIPS 180-4/RFC 1321), не биндинг к
// OpenSSL/libcrypto - тот же принцип, что и у RandomHelpers/датHelpers (не
// тянуть внешнюю библиотеку туда, где хватает ~100 строк стандартного
// алгоритма, не привязанного к платформе). "rech_hash_hex" - runtime-
// диспетчер по числу (член перечисления "АлгоритмХеша", уже развёрнутый
// семантическим анализом в IntLiteral - см. package doc hashByAlgorithm в
// internal/interp/crypto.go, ИНДЕКСЫ ДОЛЖНЫ СОВПАДАТЬ с internal/semantic/
// types.go, StandardEnums). HMAC-SHA256 (FIPS 198-1) построен поверх
// SHA-256 - основа подписи токена (алгоритм HS256 в JWT). "rech_bytes_
// to_hex" - общий хвост всех хеш-функций: результат - HEX, не сырые байты
// (Rech "Строка" в cbackend - "const char *" без отдельно хранимой длины,
// сырой дайджест мог бы содержать 0x00 внутри и потерять хвост при любой
// операции, использующей strlen). Buffer выделяется через malloc, не
// GC_MALLOC - тем же принципом, что и strutil.Helpers/RandomHelpers:
// программа, вызвавшая только эти функции, не должна тянуть за собой
// Boehm GC (<gc.h>).
var Helpers = fmt.Sprintf(`static const uint32_t rech_sha256_k[64] = {
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

// rech_sha1_ctx/rech_sha1_* - SHA-1 (FIPS 180-4), тот же принцип, что и
// SHA-256 выше (собственная реализация, не биндинг к libcrypto) - только
// 5 32-битных слов состояния и другие раундовые функции/константы.
// Криптографически сломан (известны коллизии) - не для новых
// паролей/подписей ("хеш_sha256"/"подпись_hmac_sha256" для этого), а для
// совместимости с форматами, которые требуют именно SHA-1 (объекты Git).
typedef struct {
    uint32_t state[5];
    uint64_t bitlen;
    unsigned char buffer[64];
    size_t buflen;
} rech_sha1_ctx;

#define RECH_ROTL32(x, n) (((x) << (n)) | ((x) >> (32 - (n))))

static void rech_sha1_transform(rech_sha1_ctx *ctx, const unsigned char *data) {
    uint32_t w[80];
    for (int i = 0; i < 16; i++) {
        w[i] = ((uint32_t)data[i * 4] << 24) | ((uint32_t)data[i * 4 + 1] << 16) |
               ((uint32_t)data[i * 4 + 2] << 8) | (uint32_t)data[i * 4 + 3];
    }
    for (int i = 16; i < 80; i++) {
        w[i] = RECH_ROTL32(w[i - 3] ^ w[i - 8] ^ w[i - 14] ^ w[i - 16], 1);
    }
    uint32_t a = ctx->state[0], b = ctx->state[1], c = ctx->state[2];
    uint32_t d = ctx->state[3], e = ctx->state[4];
    for (int i = 0; i < 80; i++) {
        uint32_t f, k;
        if (i < 20) {
            f = (b & c) | ((~b) & d);
            k = 0x5a827999u;
        } else if (i < 40) {
            f = b ^ c ^ d;
            k = 0x6ed9eba1u;
        } else if (i < 60) {
            f = (b & c) | (b & d) | (c & d);
            k = 0x8f1bbcdcu;
        } else {
            f = b ^ c ^ d;
            k = 0xca62c1d6u;
        }
        uint32_t temp = RECH_ROTL32(a, 5) + f + e + k + w[i];
        e = d; d = c; c = RECH_ROTL32(b, 30); b = a; a = temp;
    }
    ctx->state[0] += a; ctx->state[1] += b; ctx->state[2] += c;
    ctx->state[3] += d; ctx->state[4] += e;
}

static void rech_sha1_init(rech_sha1_ctx *ctx) {
    ctx->bitlen = 0;
    ctx->buflen = 0;
    ctx->state[0] = 0x67452301; ctx->state[1] = 0xefcdab89;
    ctx->state[2] = 0x98badcfe; ctx->state[3] = 0x10325476;
    ctx->state[4] = 0xc3d2e1f0;
}

static void rech_sha1_update(rech_sha1_ctx *ctx, const unsigned char *data, size_t len) {
    for (size_t i = 0; i < len; i++) {
        ctx->buffer[ctx->buflen++] = data[i];
        if (ctx->buflen == 64) {
            rech_sha1_transform(ctx, ctx->buffer);
            ctx->bitlen += 512;
            ctx->buflen = 0;
        }
    }
}

static void rech_sha1_final(rech_sha1_ctx *ctx, unsigned char out[20]) {
    size_t i = ctx->buflen;
    ctx->bitlen += (uint64_t)ctx->buflen * 8;
    ctx->buffer[i++] = 0x80;
    if (i > 56) {
        while (i < 64) {
            ctx->buffer[i++] = 0;
        }
        rech_sha1_transform(ctx, ctx->buffer);
        i = 0;
    }
    while (i < 56) {
        ctx->buffer[i++] = 0;
    }
    for (int j = 7; j >= 0; j--) {
        ctx->buffer[i++] = (unsigned char)((ctx->bitlen >> (j * 8)) & 0xff);
    }
    rech_sha1_transform(ctx, ctx->buffer);
    for (int j = 0; j < 5; j++) {
        out[j * 4] = (unsigned char)((ctx->state[j] >> 24) & 0xff);
        out[j * 4 + 1] = (unsigned char)((ctx->state[j] >> 16) & 0xff);
        out[j * 4 + 2] = (unsigned char)((ctx->state[j] >> 8) & 0xff);
        out[j * 4 + 3] = (unsigned char)(ctx->state[j] & 0xff);
    }
}

// rech_md5_ctx/rech_md5_* - MD5 (RFC 1321). В отличие от SHA-1/SHA-256
// (big-endian) слова блока и длина в паддинге - little-endian - обычное
// для MD5 расхождение, не опечатка.
static const uint32_t rech_md5_k[64] = {
    0xd76aa478,0xe8c7b756,0x242070db,0xc1bdceee,0xf57c0faf,0x4787c62a,0xa8304613,0xfd469501,
    0x698098d8,0x8b44f7af,0xffff5bb1,0x895cd7be,0x6b901122,0xfd987193,0xa679438e,0x49b40821,
    0xf61e2562,0xc040b340,0x265e5a51,0xe9b6c7aa,0xd62f105d,0x02441453,0xd8a1e681,0xe7d3fbc8,
    0x21e1cde6,0xc33707d6,0xf4d50d87,0x455a14ed,0xa9e3e905,0xfcefa3f8,0x676f02d9,0x8d2a4c8a,
    0xfffa3942,0x8771f681,0x6d9d6122,0xfde5380c,0xa4beea44,0x4bdecfa9,0xf6bb4b60,0xbebfbc70,
    0x289b7ec6,0xeaa127fa,0xd4ef3085,0x04881d05,0xd9d4d039,0xe6db99e5,0x1fa27cf8,0xc4ac5665,
    0xf4292244,0x432aff97,0xab9423a7,0xfc93a039,0x655b59c3,0x8f0ccc92,0xffeff47d,0x85845dd1,
    0x6fa87e4f,0xfe2ce6e0,0xa3014314,0x4e0811a1,0xf7537e82,0xbd3af235,0x2ad7d2bb,0xeb86d391,
};

static const int rech_md5_s[64] = {
    7,12,17,22, 7,12,17,22, 7,12,17,22, 7,12,17,22,
    5, 9,14,20, 5, 9,14,20, 5, 9,14,20, 5, 9,14,20,
    4,11,16,23, 4,11,16,23, 4,11,16,23, 4,11,16,23,
    6,10,15,21, 6,10,15,21, 6,10,15,21, 6,10,15,21,
};

typedef struct {
    uint32_t state[4];
    uint64_t bitlen;
    unsigned char buffer[64];
    size_t buflen;
} rech_md5_ctx;

static void rech_md5_transform(rech_md5_ctx *ctx, const unsigned char *data) {
    uint32_t m[16];
    for (int i = 0; i < 16; i++) {
        m[i] = (uint32_t)data[i * 4] | ((uint32_t)data[i * 4 + 1] << 8) |
               ((uint32_t)data[i * 4 + 2] << 16) | ((uint32_t)data[i * 4 + 3] << 24);
    }
    uint32_t a = ctx->state[0], b = ctx->state[1], c = ctx->state[2], d = ctx->state[3];
    for (int i = 0; i < 64; i++) {
        uint32_t f;
        int g;
        if (i < 16) {
            f = (b & c) | ((~b) & d);
            g = i;
        } else if (i < 32) {
            f = (d & b) | ((~d) & c);
            g = (5 * i + 1) %% 16;
        } else if (i < 48) {
            f = b ^ c ^ d;
            g = (3 * i + 5) %% 16;
        } else {
            f = c ^ (b | (~d));
            g = (7 * i) %% 16;
        }
        f = f + a + rech_md5_k[i] + m[g];
        a = d; d = c; c = b;
        b = b + RECH_ROTL32(f, rech_md5_s[i]);
    }
    ctx->state[0] += a; ctx->state[1] += b; ctx->state[2] += c; ctx->state[3] += d;
}

static void rech_md5_init(rech_md5_ctx *ctx) {
    ctx->bitlen = 0;
    ctx->buflen = 0;
    ctx->state[0] = 0x67452301; ctx->state[1] = 0xefcdab89;
    ctx->state[2] = 0x98badcfe; ctx->state[3] = 0x10325476;
}

static void rech_md5_update(rech_md5_ctx *ctx, const unsigned char *data, size_t len) {
    for (size_t i = 0; i < len; i++) {
        ctx->buffer[ctx->buflen++] = data[i];
        if (ctx->buflen == 64) {
            rech_md5_transform(ctx, ctx->buffer);
            ctx->bitlen += 512;
            ctx->buflen = 0;
        }
    }
}

static void rech_md5_final(rech_md5_ctx *ctx, unsigned char out[16]) {
    size_t i = ctx->buflen;
    ctx->bitlen += (uint64_t)ctx->buflen * 8;
    ctx->buffer[i++] = 0x80;
    if (i > 56) {
        while (i < 64) {
            ctx->buffer[i++] = 0;
        }
        rech_md5_transform(ctx, ctx->buffer);
        i = 0;
    }
    while (i < 56) {
        ctx->buffer[i++] = 0;
    }
    for (int j = 0; j < 8; j++) {
        ctx->buffer[i++] = (unsigned char)((ctx->bitlen >> (j * 8)) & 0xff);
    }
    rech_md5_transform(ctx, ctx->buffer);
    for (int j = 0; j < 4; j++) {
        out[j * 4] = (unsigned char)(ctx->state[j] & 0xff);
        out[j * 4 + 1] = (unsigned char)((ctx->state[j] >> 8) & 0xff);
        out[j * 4 + 2] = (unsigned char)((ctx->state[j] >> 16) & 0xff);
        out[j * 4 + 3] = (unsigned char)((ctx->state[j] >> 24) & 0xff);
    }
}

// rech_sha512_ctx/rech_sha512_* - SHA-512 (FIPS 180-4) - тот же принцип,
// что и SHA-256 выше, только 64-битные слова (8 слов состояния, 80
// раундов, блок 128 байт) и другие раундовые константы/сдвиги. НЕ сломан
// (в отличие от SHA-1/MD5 выше) - годится для новых паролей/подписей, у
// него просто более длинный (64-байтовый) дайджест, чем у SHA-256.
static const uint64_t rech_sha512_k[80] = {
    0x428a2f98d728ae22ULL,0x7137449123ef65cdULL,0xb5c0fbcfec4d3b2fULL,0xe9b5dba58189dbbcULL,
    0x3956c25bf348b538ULL,0x59f111f1b605d019ULL,0x923f82a4af194f9bULL,0xab1c5ed5da6d8118ULL,
    0xd807aa98a3030242ULL,0x12835b0145706fbeULL,0x243185be4ee4b28cULL,0x550c7dc3d5ffb4e2ULL,
    0x72be5d74f27b896fULL,0x80deb1fe3b1696b1ULL,0x9bdc06a725c71235ULL,0xc19bf174cf692694ULL,
    0xe49b69c19ef14ad2ULL,0xefbe4786384f25e3ULL,0x0fc19dc68b8cd5b5ULL,0x240ca1cc77ac9c65ULL,
    0x2de92c6f592b0275ULL,0x4a7484aa6ea6e483ULL,0x5cb0a9dcbd41fbd4ULL,0x76f988da831153b5ULL,
    0x983e5152ee66dfabULL,0xa831c66d2db43210ULL,0xb00327c898fb213fULL,0xbf597fc7beef0ee4ULL,
    0xc6e00bf33da88fc2ULL,0xd5a79147930aa725ULL,0x06ca6351e003826fULL,0x142929670a0e6e70ULL,
    0x27b70a8546d22ffcULL,0x2e1b21385c26c926ULL,0x4d2c6dfc5ac42aedULL,0x53380d139d95b3dfULL,
    0x650a73548baf63deULL,0x766a0abb3c77b2a8ULL,0x81c2c92e47edaee6ULL,0x92722c851482353bULL,
    0xa2bfe8a14cf10364ULL,0xa81a664bbc423001ULL,0xc24b8b70d0f89791ULL,0xc76c51a30654be30ULL,
    0xd192e819d6ef5218ULL,0xd69906245565a910ULL,0xf40e35855771202aULL,0x106aa07032bbd1b8ULL,
    0x19a4c116b8d2d0c8ULL,0x1e376c085141ab53ULL,0x2748774cdf8eeb99ULL,0x34b0bcb5e19b48a8ULL,
    0x391c0cb3c5c95a63ULL,0x4ed8aa4ae3418acbULL,0x5b9cca4f7763e373ULL,0x682e6ff3d6b2b8a3ULL,
    0x748f82ee5defb2fcULL,0x78a5636f43172f60ULL,0x84c87814a1f0ab72ULL,0x8cc702081a6439ecULL,
    0x90befffa23631e28ULL,0xa4506cebde82bde9ULL,0xbef9a3f7b2c67915ULL,0xc67178f2e372532bULL,
    0xca273eceea26619cULL,0xd186b8c721c0c207ULL,0xeada7dd6cde0eb1eULL,0xf57d4f7fee6ed178ULL,
    0x06f067aa72176fbaULL,0x0a637dc5a2c898a6ULL,0x113f9804bef90daeULL,0x1b710b35131c471bULL,
    0x28db77f523047d84ULL,0x32caab7b40c72493ULL,0x3c9ebe0a15c9bebcULL,0x431d67c49c100d4cULL,
    0x4cc5d4becb3e42b6ULL,0x597f299cfc657e2aULL,0x5fcb6fab3ad6faecULL,0x6c44198c4a475817ULL,
};

typedef struct {
    uint64_t state[8];
    uint64_t bitlen;
    unsigned char buffer[128];
    size_t buflen;
} rech_sha512_ctx;

#define RECH_ROTR64(x, n) (((x) >> (n)) | ((x) << (64 - (n))))

static void rech_sha512_transform(rech_sha512_ctx *ctx, const unsigned char *data) {
    uint64_t m[80];
    for (int i = 0; i < 16; i++) {
        m[i] = 0;
        for (int j = 0; j < 8; j++) {
            m[i] = (m[i] << 8) | (uint64_t)data[i * 8 + j];
        }
    }
    for (int i = 16; i < 80; i++) {
        uint64_t s0 = RECH_ROTR64(m[i - 15], 1) ^ RECH_ROTR64(m[i - 15], 8) ^ (m[i - 15] >> 7);
        uint64_t s1 = RECH_ROTR64(m[i - 2], 19) ^ RECH_ROTR64(m[i - 2], 61) ^ (m[i - 2] >> 6);
        m[i] = m[i - 16] + s0 + m[i - 7] + s1;
    }
    uint64_t a = ctx->state[0], b = ctx->state[1], c = ctx->state[2], d = ctx->state[3];
    uint64_t e = ctx->state[4], f = ctx->state[5], g = ctx->state[6], h = ctx->state[7];
    for (int i = 0; i < 80; i++) {
        uint64_t s1 = RECH_ROTR64(e, 14) ^ RECH_ROTR64(e, 18) ^ RECH_ROTR64(e, 41);
        uint64_t ch = (e & f) ^ ((~e) & g);
        uint64_t t1 = h + s1 + ch + rech_sha512_k[i] + m[i];
        uint64_t s0 = RECH_ROTR64(a, 28) ^ RECH_ROTR64(a, 34) ^ RECH_ROTR64(a, 39);
        uint64_t maj = (a & b) ^ (a & c) ^ (b & c);
        uint64_t t2 = s0 + maj;
        h = g; g = f; f = e; e = d + t1;
        d = c; c = b; b = a; a = t1 + t2;
    }
    ctx->state[0] += a; ctx->state[1] += b; ctx->state[2] += c; ctx->state[3] += d;
    ctx->state[4] += e; ctx->state[5] += f; ctx->state[6] += g; ctx->state[7] += h;
}

static void rech_sha512_init(rech_sha512_ctx *ctx) {
    ctx->bitlen = 0;
    ctx->buflen = 0;
    ctx->state[0] = 0x6a09e667f3bcc908ULL; ctx->state[1] = 0xbb67ae8584caa73bULL;
    ctx->state[2] = 0x3c6ef372fe94f82bULL; ctx->state[3] = 0xa54ff53a5f1d36f1ULL;
    ctx->state[4] = 0x510e527fade682d1ULL; ctx->state[5] = 0x9b05688c2b3e6c1fULL;
    ctx->state[6] = 0x1f83d9abfb41bd6bULL; ctx->state[7] = 0x5be0cd19137e2179ULL;
}

static void rech_sha512_update(rech_sha512_ctx *ctx, const unsigned char *data, size_t len) {
    for (size_t i = 0; i < len; i++) {
        ctx->buffer[ctx->buflen++] = data[i];
        if (ctx->buflen == 128) {
            rech_sha512_transform(ctx, ctx->buffer);
            ctx->bitlen += 1024;
            ctx->buflen = 0;
        }
    }
}

// Паддинг - 128-байтовый блок, 128-битное поле длины (мы храним только
// младшие 64 бита - на практике сообщение короче 2^64 бит, ~2 эксабайта -
// верхние 8 байт поля длины всегда нулевые, тот же практический предел,
// что уже неявно принят у SHA-256 выше).
static void rech_sha512_final(rech_sha512_ctx *ctx, unsigned char out[64]) {
    size_t i = ctx->buflen;
    ctx->bitlen += (uint64_t)ctx->buflen * 8;
    ctx->buffer[i++] = 0x80;
    if (i > 112) {
        while (i < 128) {
            ctx->buffer[i++] = 0;
        }
        rech_sha512_transform(ctx, ctx->buffer);
        i = 0;
    }
    while (i < 112) {
        ctx->buffer[i++] = 0;
    }
    for (int j = 0; j < 8; j++) {
        ctx->buffer[i++] = 0;
    }
    for (int j = 7; j >= 0; j--) {
        ctx->buffer[i++] = (unsigned char)((ctx->bitlen >> (j * 8)) & 0xff);
    }
    rech_sha512_transform(ctx, ctx->buffer);
    for (int j = 0; j < 8; j++) {
        out[j * 8] = (unsigned char)((ctx->state[j] >> 56) & 0xff);
        out[j * 8 + 1] = (unsigned char)((ctx->state[j] >> 48) & 0xff);
        out[j * 8 + 2] = (unsigned char)((ctx->state[j] >> 40) & 0xff);
        out[j * 8 + 3] = (unsigned char)((ctx->state[j] >> 32) & 0xff);
        out[j * 8 + 4] = (unsigned char)((ctx->state[j] >> 24) & 0xff);
        out[j * 8 + 5] = (unsigned char)((ctx->state[j] >> 16) & 0xff);
        out[j * 8 + 6] = (unsigned char)((ctx->state[j] >> 8) & 0xff);
        out[j * 8 + 7] = (unsigned char)(ctx->state[j] & 0xff);
    }
}

// rech_sha3_256_* - SHA3_256 (FIPS 202) - sponge-конструкция на базе
// перестановки Keccak-f[1600], принципиально другая, чем Меркла-Дамгора у
// SHA-256/1/MD5/SHA-512 выше (нет message schedule/раундовых W[]-слов -
// вместо этого 1600-битное состояние из 25 64-битных "полос", 24 раунда
// theta/rho/pi/chi/iota). Состояние хранится как 200-байтовый массив (не
// union'ом uint64_t/uint8_t - typedef-punning через union для чтения "не
// того" члена, которым он был записан, undefined behavior в C, хотя многие
// компиляторы его разрешают как расширение) - rech_keccakf сама
// пакует/распаковывает 8-байтовые "полосы" в little-endian явными
// сдвигами на входе/выходе перестановки.
//
// Константы (rech_keccakf_rndc/rotc/piln) и структура самой перестановки -
// стандартный вид эталонной реализации Keccak-семейства (не собственное
// изобретение) - соответствие подтверждено тестовыми векторами
// (SHA3-256("")/SHA3-256("abc")), не только "похоже на правильное".
static const uint64_t rech_keccakf_rndc[24] = {
    0x0000000000000001ULL, 0x0000000000008082ULL, 0x800000000000808aULL, 0x8000000080008000ULL,
    0x000000000000808bULL, 0x0000000080000001ULL, 0x8000000080008081ULL, 0x8000000000008009ULL,
    0x000000000000008aULL, 0x0000000000000088ULL, 0x0000000080008009ULL, 0x000000008000000aULL,
    0x000000008000808bULL, 0x800000000000008bULL, 0x8000000000008089ULL, 0x8000000000008003ULL,
    0x8000000000008002ULL, 0x8000000000000080ULL, 0x000000000000800aULL, 0x800000008000000aULL,
    0x8000000080008081ULL, 0x8000000000008080ULL, 0x0000000080000001ULL, 0x8000000080008008ULL,
};

static const int rech_keccakf_rotc[24] = {
    1, 3, 6, 10, 15, 21, 28, 36, 45, 55, 2, 14,
    27, 41, 56, 8, 25, 43, 62, 18, 39, 61, 20, 44,
};

static const int rech_keccakf_piln[24] = {
    10, 7, 11, 17, 18, 3, 5, 16, 8, 21, 24, 4,
    15, 23, 19, 13, 12, 2, 20, 14, 22, 9, 6, 1,
};

#define RECH_ROTL64(x, n) (((x) << (n)) | ((x) >> (64 - (n))))

static void rech_keccakf(unsigned char *stb) {
    uint64_t st[25];
    for (int i = 0; i < 25; i++) {
        st[i] = 0;
        for (int b = 0; b < 8; b++) {
            st[i] |= ((uint64_t)stb[i * 8 + b]) << (8 * b);
        }
    }
    uint64_t bc[5], t;
    for (int r = 0; r < 24; r++) {
        for (int i = 0; i < 5; i++) {
            bc[i] = st[i] ^ st[i + 5] ^ st[i + 10] ^ st[i + 15] ^ st[i + 20];
        }
        for (int i = 0; i < 5; i++) {
            t = bc[(i + 4) %% 5] ^ RECH_ROTL64(bc[(i + 1) %% 5], 1);
            for (int j = 0; j < 25; j += 5) {
                st[j + i] ^= t;
            }
        }
        t = st[1];
        for (int i = 0; i < 24; i++) {
            int j = rech_keccakf_piln[i];
            uint64_t tmp = st[j];
            st[j] = RECH_ROTL64(t, rech_keccakf_rotc[i]);
            t = tmp;
        }
        for (int j = 0; j < 25; j += 5) {
            uint64_t bc2[5];
            for (int i = 0; i < 5; i++) {
                bc2[i] = st[j + i];
            }
            for (int i = 0; i < 5; i++) {
                st[j + i] ^= (~bc2[(i + 1) %% 5]) & bc2[(i + 2) %% 5];
            }
        }
        st[0] ^= rech_keccakf_rndc[r];
    }
    for (int i = 0; i < 25; i++) {
        for (int b = 0; b < 8; b++) {
            stb[i * 8 + b] = (unsigned char)((st[i] >> (8 * b)) & 0xff);
        }
    }
}

typedef struct {
    unsigned char st[200];
    int rsiz;
    int pt;
} rech_sha3_ctx;

static void rech_sha3_256_init(rech_sha3_ctx *ctx) {
    memset(ctx->st, 0, sizeof(ctx->st));
    ctx->rsiz = 200 - 2 * 32;
    ctx->pt = 0;
}

static void rech_sha3_256_update(rech_sha3_ctx *ctx, const unsigned char *data, size_t len) {
    int j = ctx->pt;
    for (size_t i = 0; i < len; i++) {
        ctx->st[j++] ^= data[i];
        if (j >= ctx->rsiz) {
            rech_keccakf(ctx->st);
            j = 0;
        }
    }
    ctx->pt = j;
}

static void rech_sha3_256_final(rech_sha3_ctx *ctx, unsigned char out[32]) {
    ctx->st[ctx->pt] ^= 0x06;
    ctx->st[ctx->rsiz - 1] ^= 0x80;
    rech_keccakf(ctx->st);
    memcpy(out, ctx->st, 32);
}

// rech_hex_decode - обратная операция к rech_bytes_to_hex ниже (HEX-строка
// в байты, не байты в HEX) - нужна "расшифровать" (aes.go)/"проверить_
// подпись" (signing.go), обеим достаточно usesCrypto=true (обе тянут его
// вместе со своим отдельным флагом), значит уместнее здесь, в общем блоке,
// чем дублировать в обоих местах.
static unsigned char *rech_hex_decode(const char *hex, size_t *outlen) {
    size_t hexlen = strlen(hex);
    if (hexlen %% 2 != 0) {
        return NULL;
    }
    size_t n = hexlen / 2;
    unsigned char *out = malloc(n > 0 ? n : 1);
    for (size_t i = 0; i < n; i++) {
        unsigned int b;
        if (sscanf(hex + i * 2, "%%2x", &b) != 1) {
            free(out);
            return NULL;
        }
        out[i] = (unsigned char)b;
    }
    *outlen = n;
    return out;
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

static char *rech_sha1_hex(const char *s) {
    rech_sha1_ctx ctx;
    rech_sha1_init(&ctx);
    rech_sha1_update(&ctx, (const unsigned char *)s, strlen(s));
    unsigned char digest[20];
    rech_sha1_final(&ctx, digest);
    return rech_bytes_to_hex(digest, 20);
}

static char *rech_md5_hex(const char *s) {
    rech_md5_ctx ctx;
    rech_md5_init(&ctx);
    rech_md5_update(&ctx, (const unsigned char *)s, strlen(s));
    unsigned char digest[16];
    rech_md5_final(&ctx, digest);
    return rech_bytes_to_hex(digest, 16);
}

static char *rech_sha512_hex(const char *s) {
    rech_sha512_ctx ctx;
    rech_sha512_init(&ctx);
    rech_sha512_update(&ctx, (const unsigned char *)s, strlen(s));
    unsigned char digest[64];
    rech_sha512_final(&ctx, digest);
    return rech_bytes_to_hex(digest, 64);
}

static char *rech_sha3_256_hex(const char *s) {
    rech_sha3_ctx ctx;
    rech_sha3_256_init(&ctx);
    rech_sha3_256_update(&ctx, (const unsigned char *)s, strlen(s));
    unsigned char digest[32];
    rech_sha3_256_final(&ctx, digest);
    return rech_bytes_to_hex(digest, 32);
}

// rech_hash_hex - диспетчер builtin'а "хеш(алгоритм, данные)" по значению
// члена перечисления "АлгоритмХеша" - ИНДЕКСЫ ДОЛЖНЫ БУКВАЛЬНО СОВПАДАТЬ с
// internal/semantic/types.go, StandardEnums["АлгоритмХеша"] (0=SHA256,
// 1=SHA1, 2=MD5, 3=SHA512, 4=SHA3_256) - тот же контракт, что у
// hashByAlgorithm в internal/interp/crypto.go. Значение вне диапазона
// возможно, только если вызвать "хеш" с голым Целое напрямую, не через сам
// enum (тип параметра - Kind int, компилятор не проверяет диапазон) -
// аварийное завершение, не неопределённое поведение.
static char *rech_hash_hex(int algorithm, const char *s) {
    switch (algorithm) {
    case 0:
        return rech_sha256_hex(s);
    case 1:
        return rech_sha1_hex(s);
    case 2:
        return rech_md5_hex(s);
    case 3:
        return rech_sha512_hex(s);
    case 4:
        return rech_sha3_256_hex(s);
    default:
        rech_panic("%[2]s");
        return NULL;
    }
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
    return rech_text_take(out, oi);
}
`, rtmsg.Base64URLInvalidInput, rtmsg.UnknownHashAlgorithm)
