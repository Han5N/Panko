#include "world/block_registry.h"

#include "world/block_registry_data.h"

namespace panko {

bool lookupBlockHash(uint32_t hash, BlockState& out) {
    size_t lo = 0;
    size_t hi = kBlockRegistryCount;
    while (lo < hi) {
        size_t mid = lo + (hi - lo) / 2;
        if (kBlockRegistry[mid].hash < hash) {
            lo = mid + 1;
        } else {
            hi = mid;
        }
    }
    if (lo < kBlockRegistryCount && kBlockRegistry[lo].hash == hash) {
        out.name = kBlockRegistry[lo].name;
        out.states = kBlockRegistry[lo].states;
        return true;
    }
    return false;
}

size_t blockRegistrySize() { return kBlockRegistryCount; }

uint32_t fnv1a32(const uint8_t* data, size_t size) {
    uint32_t hash = 0x811c9dc5;
    for (size_t i = 0; i < size; ++i) {
        hash ^= data[i];
        hash *= 0x01000193;
    }
    return hash;
}

}  // namespace panko
