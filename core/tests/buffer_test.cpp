#include "common/buffer.h"
#include "test.h"

using panko::Buffer;
using panko::ParseError;

TEST_CASE(varuint_roundtrip) {
    const uint32_t values[] = {0, 1, 127, 128, 300, 16384, 0xFFFFFF, 0xFFFFFFFFu};
    for (uint32_t v : values) {
        Buffer b;
        b.writeVarUInt(v);
        CHECK_EQ(b.readVarUInt(), v);
    }
}

TEST_CASE(varuint64_roundtrip) {
    const uint64_t values[] = {0, 127, 128, 1ULL << 32, 0xFFFFFFFFFFFFFFFFULL};
    for (uint64_t v : values) {
        Buffer b;
        b.writeVarUInt64(v);
        CHECK_EQ(b.readVarUInt64(), v);
    }
}

TEST_CASE(zigzag_roundtrip) {
    const int32_t values[] = {0, -1, 1, -64, 63, -1000000, 1000000};
    for (int32_t v : values) {
        Buffer b;
        b.writeZigZag32(v);
        CHECK_EQ(b.readZigZag32(), v);
    }

    const int64_t values64[] = {0, -1, 1, -10000000000LL, 10000000000LL};
    for (int64_t v : values64) {
        Buffer b;
        b.writeZigZag64(v);
        CHECK_EQ(b.readZigZag64(), v);
    }
}

TEST_CASE(string_and_bytes) {
    Buffer b;
    b.writeString("stone");
    b.writeString("");
    CHECK_EQ(b.readString(), std::string("stone"));
    CHECK_EQ(b.readString(), std::string(""));

    Buffer c;
    c.writeU8(1);
    c.writeU16LE(0x1234);
    c.writeU32LE(0x12345678);
    c.writeU64LE(0x123456789ABCDEF0ULL);
    CHECK_EQ(c.readU8(), uint8_t{1});
    CHECK_EQ(c.readU16LE(), uint16_t{0x1234});
    CHECK_EQ(c.readU32LE(), uint32_t{0x12345678});
    CHECK_EQ(c.readU64LE(), uint64_t{0x123456789ABCDEF0ULL});
}

TEST_CASE(float_roundtrip) {
    Buffer b;
    b.writeF32LE(3.5f);
    b.writeF64LE(-2.25);
    CHECK(b.readF32LE() == 3.5f);
    CHECK(b.readF64LE() == -2.25);
}

TEST_CASE(underflow_throws) {
    Buffer b;
    b.writeU8(1);
    bool thrown = false;
    try {
        b.readU32LE();
    } catch (const ParseError&) {
        thrown = true;
    }
    CHECK(thrown);
}

TEST_CASE(hex_dump) {
    const uint8_t data[] = {0x00, 0x0f, 0xff};
    CHECK_EQ(panko::hexDump(data, 3), std::string("00 0f ff"));
}
