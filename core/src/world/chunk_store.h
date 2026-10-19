#pragma once

#include <cstdint>
#include <map>
#include <mutex>
#include <vector>

#include "world/subchunk.h"

namespace panko {

// ChunkStore 保存已解码的区块段，按 (x, z) 区块与段高度索引。
class ChunkStore {
public:
    // applySection 写入一个 16³ 段。坐标是段坐标，不是方块坐标。
    void applySection(int x, int y, int z, const SubChunkSection& section);

    // blockAt 查询方块哈希。区块或段未加载时返回 false。
    bool blockAt(int x, int y, int z, uint32_t& hash) const;

    size_t sectionCount() const;
    void clear();

private:
    struct ChunkKey {
        int x = 0;
        int z = 0;

        bool operator<(const ChunkKey& other) const {
            return x < other.x || (x == other.x && z < other.z);
        }
    };

    std::map<ChunkKey, std::map<int, std::vector<uint32_t>>> chunks_;
    mutable std::mutex mu_;
};

}  // namespace panko
