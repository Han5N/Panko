#include "world/subchunk.h"

#include <algorithm>
#include <stdexcept>

#include "common/buffer.h"

namespace panko {

namespace {

constexpr int kBlocksPerSection = 16 * 16 * 16;

uint32_t readHash(Buffer& in) {
    // 调色板条目是 zigzag 编码的 32 位哈希。
    return static_cast<uint32_t>(in.readZigZag64());
}

}  // namespace

bool decodeSubChunkPayload(const std::vector<uint8_t>& payload,
                           std::vector<SubChunkSection>& out, std::string* error) {
    try {
        Buffer in(payload);
        uint8_t version = in.readU8();
        uint8_t layer_count = in.readU8();

        int y_index = 0;
        bool has_y_index = false;
        if (version >= 9) {
            y_index = static_cast<int8_t>(in.readU8());
            has_y_index = true;
        }

        if (layer_count == 0) {
            throw ParseError("层数为 0");
        }

        for (uint8_t layer = 0; layer < layer_count; ++layer) {
            uint8_t header = in.readU8();
            int bits = header >> 1;

            std::vector<uint32_t> blocks(kBlocksPerSection, 0);

            if (bits == 0) {
                // 单一方块，无索引、无调色板。
                std::fill(blocks.begin(), blocks.end(), readHash(in));
            } else {
                if (bits > 32) {
                    throw ParseError("位宽超过 32");
                }

                int per_word = 32 / bits;
                size_t words = (kBlocksPerSection + per_word - 1) / per_word;

                // 网易版的存储顺序是“索引在前、调色板在后”，与直觉相反。
                std::vector<uint32_t> raw(words);
                for (auto& word : raw) {
                    word = in.readU32LE();
                }

                int palette_size = static_cast<int>(in.readZigZag64());
                if (palette_size <= 0 || palette_size > 65536) {
                    throw ParseError("调色板大小非法: " + std::to_string(palette_size));
                }

                std::vector<uint32_t> palette(static_cast<size_t>(palette_size));
                for (auto& entry : palette) {
                    entry = readHash(in);
                }

                uint32_t mask = bits == 32 ? 0xFFFFFFFFu : ((1u << bits) - 1);
                for (int i = 0; i < kBlocksPerSection; ++i) {
                    uint32_t word = raw[static_cast<size_t>(i / per_word)];
                    uint32_t index =
                        (word >> ((i % per_word) * bits)) & mask;
                    blocks[static_cast<size_t>(i)] =
                        index < palette.size() ? palette[index] : 0;
                }
            }

            // 第 0 层是主方块，其余层是含水/覆层数据，当前只需要主层。
            if (layer == 0) {
                SubChunkSection section;
                section.has_y_index = has_y_index;
                section.y_index = y_index;
                section.blocks = std::move(blocks);
                out.push_back(std::move(section));
            }
        }

        return true;
    } catch (const std::exception& e) {
        if (error) *error = e.what();
        return false;
    }
}

}  // namespace panko
