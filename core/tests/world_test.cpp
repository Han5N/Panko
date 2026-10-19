#include "common/buffer.h"
#include "test.h"
#include "world/block_registry.h"
#include "world/chunk_store.h"
#include "world/subchunk.h"

using namespace panko;

namespace {

constexpr uint32_t kStone = 0x80310e21;
constexpr uint32_t kGrass = 0xde3128b4;
constexpr uint32_t kAir = 0xdbf44120;

// 构造一个 v9 子区块载荷，bits 位宽，调色板为给定哈希。
std::vector<uint8_t> makePayload(int bits, const std::vector<uint32_t>& palette,
                                 int y_index) {
    Buffer out;
    out.writeU8(9);
    out.writeU8(1);
    out.writeU8(static_cast<uint8_t>(y_index));
    out.writeU8(static_cast<uint8_t>(bits << 1));

    int per_word = 32 / bits;
    size_t words = (4096 + per_word - 1) / per_word;
    std::vector<uint32_t> raw(words, 0);
    for (int i = 0; i < 4096; ++i) {
        uint32_t index = static_cast<uint32_t>(i % static_cast<int>(palette.size()));
        raw[static_cast<size_t>(i / per_word)] |= index << ((i % per_word) * bits);
    }
    for (uint32_t word : raw) {
        out.writeU32LE(word);
    }

    out.writeZigZag32(static_cast<int32_t>(palette.size()));
    for (uint32_t hash : palette) {
        out.writeZigZag32(static_cast<int32_t>(hash));
    }
    return out.bytes();
}

}  // namespace

TEST_CASE(block_registry_lookup) {
    CHECK_EQ(blockRegistrySize(), size_t{15845});

    BlockState stone;
    CHECK(lookupBlockHash(kStone, stone));
    CHECK_EQ(stone.name, std::string("minecraft:stone"));

    BlockState grass;
    CHECK(lookupBlockHash(kGrass, grass));
    CHECK_EQ(grass.name, std::string("minecraft:grass_block"));

    BlockState air;
    CHECK(lookupBlockHash(kAir, air));
    CHECK_EQ(air.name, std::string("minecraft:air"));

    BlockState missing;
    CHECK(!lookupBlockHash(0x12345678, missing));
}

TEST_CASE(fnv1a32_vectors) {
    CHECK_EQ(fnv1a32(nullptr, 0), uint32_t{0x811c9dc5});
    const uint8_t a[] = {'a'};
    CHECK_EQ(fnv1a32(a, 1), uint32_t{0xe40c292c});
}

TEST_CASE(subchunk_decode_bits1) {
    std::vector<SubChunkSection> sections;
    std::string error;
    auto payload = makePayload(1, {kStone, kGrass}, 3);

    CHECK(decodeSubChunkPayload(payload, sections, &error));
    CHECK_EQ(sections.size(), size_t{1});
    CHECK(sections[0].has_y_index);
    CHECK_EQ(sections[0].y_index, 3);
    CHECK_EQ(sections[0].blocks.size(), size_t{4096});
    CHECK_EQ(sections[0].blocks[0], kStone);
    CHECK_EQ(sections[0].blocks[1], kGrass);
    CHECK_EQ(sections[0].blocks[2], kStone);
}

TEST_CASE(subchunk_decode_single_block) {
    Buffer payload;
    payload.writeU8(9);
    payload.writeU8(1);
    payload.writeU8(static_cast<uint8_t>(-1));
    payload.writeU8(0);  // bits = 0
    payload.writeZigZag32(static_cast<int32_t>(kStone));

    std::vector<SubChunkSection> sections;
    CHECK(decodeSubChunkPayload(payload.bytes(), sections));
    CHECK_EQ(sections.size(), size_t{1});
    CHECK_EQ(sections[0].y_index, -1);
    CHECK_EQ(sections[0].blocks[100], kStone);
}

TEST_CASE(subchunk_rejects_truncated) {
    Buffer payload;
    payload.writeU8(9);
    payload.writeU8(1);
    payload.writeU8(0);
    payload.writeU8(2);  // 声明 bits=1，但没有索引数据

    std::vector<SubChunkSection> sections;
    std::string error;
    CHECK(!decodeSubChunkPayload(payload.bytes(), sections, &error));
    CHECK(!error.empty());
}

TEST_CASE(chunk_store_lookup) {
    ChunkStore store;

    SubChunkSection section;
    section.blocks.assign(4096, kStone);
    section.blocks[1 * 256 + 3 * 16 + 2] = kGrass;  // x=1, y=2, z=3

    store.applySection(0, 0, 0, section);
    CHECK_EQ(store.sectionCount(), size_t{1});

    uint32_t hash = 0;
    CHECK(store.blockAt(1, 2, 3, hash));
    CHECK_EQ(hash, kGrass);

    CHECK(store.blockAt(0, 0, 0, hash));
    CHECK_EQ(hash, kStone);

    // 未加载的段与区块。
    CHECK(!store.blockAt(1, 40, 3, hash));
    CHECK(!store.blockAt(100, 0, 100, hash));
}

TEST_CASE(chunk_store_negative_coords) {
    ChunkStore store;

    SubChunkSection section;
    section.blocks.assign(4096, kStone);
    store.applySection(-1, -1, -1, section);

    uint32_t hash = 0;
    CHECK(store.blockAt(-1, -1, -1, hash));
    CHECK_EQ(hash, kStone);

    // (-16,-16,-16) 属于 (-1,-1,-1) 号段；(-17,-17,-17) 属于 (-2,-2,-2)。
    CHECK(store.blockAt(-16, -16, -16, hash));
    CHECK(!store.blockAt(-17, -17, -17, hash));
}
