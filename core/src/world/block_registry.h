#pragma once

#include <cstddef>
#include <cstdint>
#include <string>

namespace panko {

// BlockState 是查表结果。states 保留原始 JSON 字符串，避免为每种方块
// 定义不同的结构。
struct BlockState {
    std::string name;
    std::string states;
};

// lookupBlockHash 在方块哈希表中查找。表按哈希升序排列。
// 找不到时返回 false，调用方应显示为未知方块而不是崩溃。
bool lookupBlockHash(uint32_t hash, BlockState& out);

// blockRegistrySize 返回表项数量，供测试和诊断使用。
size_t blockRegistrySize();

// fnv1a32 是方块哈希使用的算法，输入是 little-endian NBT 编码。
uint32_t fnv1a32(const uint8_t* data, size_t size);

}  // namespace panko
