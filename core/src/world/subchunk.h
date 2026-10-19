#pragma once

#include <cstdint>
#include <string>
#include <vector>

namespace panko {

// 一个子区块的 16×16×16 方块数据。blocks 长度固定为 4096，
// 下标为 x*256 + z*16 + y，这是基岩版区块存储的排列方式。
struct SubChunkSection {
    bool has_y_index = false;
    int y_index = 0;
    std::vector<uint32_t> blocks;
};

// decodeSubChunkPayload 解析 SubChunk 包里的单段载荷。
// 支持格式版本 8 与 9；版本 9 带 y_index。多层（水logged）只保留第 0 层。
bool decodeSubChunkPayload(const std::vector<uint8_t>& payload,
                           std::vector<SubChunkSection>& out, std::string* error = nullptr);

}  // namespace panko
