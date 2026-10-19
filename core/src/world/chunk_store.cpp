#include "world/chunk_store.h"

namespace panko {

namespace {

constexpr int kSectionSize = 16;
constexpr size_t kBlocksPerSection = 16 * 16 * 16;

int floorDiv16(int value) {
    return value >= 0 ? value / kSectionSize : -((-value + kSectionSize - 1) / kSectionSize);
}

}  // namespace

void ChunkStore::applySection(int x, int y, int z, const SubChunkSection& section) {
    if (section.blocks.size() != kBlocksPerSection) {
        return;
    }
    std::lock_guard<std::mutex> lock(mu_);
    chunks_[{x, z}][y] = section.blocks;
}

bool ChunkStore::blockAt(int x, int y, int z, uint32_t& hash) const {
    int sx = floorDiv16(x);
    int sy = floorDiv16(y);
    int sz = floorDiv16(z);

    std::lock_guard<std::mutex> lock(mu_);
    auto chunk = chunks_.find({sx, sz});
    if (chunk == chunks_.end()) {
        return false;
    }
    auto section = chunk->second.find(sy);
    if (section == chunk->second.end() || section->second.size() != kBlocksPerSection) {
        return false;
    }

    int lx = x - sx * kSectionSize;
    int ly = y - sy * kSectionSize;
    int lz = z - sz * kSectionSize;

    // 基岩版的排列是 x*256 + z*16 + y。
    hash = section->second[static_cast<size_t>(lx * 256 + lz * 16 + ly)];
    return true;
}

size_t ChunkStore::sectionCount() const {
    std::lock_guard<std::mutex> lock(mu_);
    size_t count = 0;
    for (const auto& [key, sections] : chunks_) {
        (void)key;
        count += sections.size();
    }
    return count;
}

void ChunkStore::clear() {
    std::lock_guard<std::mutex> lock(mu_);
    chunks_.clear();
}

}  // namespace panko
