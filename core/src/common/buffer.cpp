#include "common/buffer.h"

#include <cstdio>
#include <cstring>

namespace panko {

void Buffer::need(size_t n) const {
    if (remaining() < n) {
        throw ParseError("buffer underflow: need " + std::to_string(n) + ", have " +
                         std::to_string(remaining()));
    }
}

void Buffer::writeU8(uint8_t v) { data_.push_back(v); }

void Buffer::writeU16LE(uint16_t v) {
    data_.push_back(static_cast<uint8_t>(v));
    data_.push_back(static_cast<uint8_t>(v >> 8));
}

void Buffer::writeU32LE(uint32_t v) {
    for (int i = 0; i < 4; ++i) data_.push_back(static_cast<uint8_t>(v >> (8 * i)));
}

void Buffer::writeU64LE(uint64_t v) {
    for (int i = 0; i < 8; ++i) data_.push_back(static_cast<uint8_t>(v >> (8 * i)));
}

void Buffer::writeI32LE(int32_t v) { writeU32LE(static_cast<uint32_t>(v)); }
void Buffer::writeI64LE(int64_t v) { writeU64LE(static_cast<uint64_t>(v)); }

void Buffer::writeF32LE(float v) {
    uint32_t bits = 0;
    std::memcpy(&bits, &v, sizeof(bits));
    writeU32LE(bits);
}

void Buffer::writeF64LE(double v) {
    uint64_t bits = 0;
    std::memcpy(&bits, &v, sizeof(bits));
    writeU64LE(bits);
}

void Buffer::writeVarUInt(uint32_t v) {
    while (v >= 0x80) {
        data_.push_back(static_cast<uint8_t>((v & 0x7f) | 0x80));
        v >>= 7;
    }
    data_.push_back(static_cast<uint8_t>(v));
}

void Buffer::writeVarUInt64(uint64_t v) {
    while (v >= 0x80) {
        data_.push_back(static_cast<uint8_t>((v & 0x7f) | 0x80));
        v >>= 7;
    }
    data_.push_back(static_cast<uint8_t>(v));
}

void Buffer::writeZigZag32(int32_t v) {
    writeVarUInt(static_cast<uint32_t>((static_cast<uint32_t>(v) << 1) ^
                                       static_cast<uint32_t>(v >> 31)));
}

void Buffer::writeZigZag64(int64_t v) {
    writeVarUInt64((static_cast<uint64_t>(v) << 1) ^ static_cast<uint64_t>(v >> 63));
}

void Buffer::writeString(const std::string& s) {
    writeVarUInt(static_cast<uint32_t>(s.size()));
    writeBytes(reinterpret_cast<const uint8_t*>(s.data()), s.size());
}

void Buffer::writeBytes(const uint8_t* p, size_t n) {
    if (n == 0) return;
    data_.insert(data_.end(), p, p + n);
}

uint8_t Buffer::readU8() {
    need(1);
    return data_[pos_++];
}

uint16_t Buffer::readU16LE() {
    need(2);
    uint16_t v = static_cast<uint16_t>(data_[pos_]) |
                 static_cast<uint16_t>(data_[pos_ + 1]) << 8;
    pos_ += 2;
    return v;
}

uint32_t Buffer::readU32LE() {
    need(4);
    uint32_t v = 0;
    for (int i = 0; i < 4; ++i) v |= static_cast<uint32_t>(data_[pos_ + i]) << (8 * i);
    pos_ += 4;
    return v;
}

uint64_t Buffer::readU64LE() {
    need(8);
    uint64_t v = 0;
    for (int i = 0; i < 8; ++i) v |= static_cast<uint64_t>(data_[pos_ + i]) << (8 * i);
    pos_ += 8;
    return v;
}

int32_t Buffer::readI32LE() { return static_cast<int32_t>(readU32LE()); }
int64_t Buffer::readI64LE() { return static_cast<int64_t>(readU64LE()); }

float Buffer::readF32LE() {
    uint32_t bits = readU32LE();
    float v = 0;
    std::memcpy(&v, &bits, sizeof(v));
    return v;
}

double Buffer::readF64LE() {
    uint64_t bits = readU64LE();
    double v = 0;
    std::memcpy(&v, &bits, sizeof(v));
    return v;
}

uint32_t Buffer::readVarUInt() {
    uint32_t result = 0;
    int shift = 0;
    for (int i = 0; i < 5; ++i) {
        uint8_t b = readU8();
        result |= static_cast<uint32_t>(b & 0x7f) << shift;
        if ((b & 0x80) == 0) return result;
        shift += 7;
    }
    throw ParseError("varuint 超过 5 字节");
}

uint64_t Buffer::readVarUInt64() {
    uint64_t result = 0;
    int shift = 0;
    for (int i = 0; i < 10; ++i) {
        uint8_t b = readU8();
        result |= static_cast<uint64_t>(b & 0x7f) << shift;
        if ((b & 0x80) == 0) return result;
        shift += 7;
    }
    throw ParseError("varuint64 超过 10 字节");
}

int32_t Buffer::readZigZag32() {
    uint32_t raw = readVarUInt();
    return static_cast<int32_t>((raw >> 1) ^ (~(raw & 1) + 1));
}

int64_t Buffer::readZigZag64() {
    uint64_t raw = readVarUInt64();
    return static_cast<int64_t>((raw >> 1) ^ (~(raw & 1) + 1));
}

std::string Buffer::readString() {
    uint32_t n = readVarUInt();
    need(n);
    std::string s(reinterpret_cast<const char*>(data_.data() + pos_), n);
    pos_ += n;
    return s;
}

std::vector<uint8_t> Buffer::readBytes(size_t n) {
    need(n);
    std::vector<uint8_t> out(data_.begin() + static_cast<long>(pos_),
                             data_.begin() + static_cast<long>(pos_ + n));
    pos_ += n;
    return out;
}

void Buffer::skip(size_t n) {
    need(n);
    pos_ += n;
}

std::string hexDump(const uint8_t* data, size_t size) {
    static const char* digits = "0123456789abcdef";
    std::string out;
    out.reserve(size * 3);
    for (size_t i = 0; i < size; ++i) {
        if (i) out.push_back(' ');
        out.push_back(digits[data[i] >> 4]);
        out.push_back(digits[data[i] & 0x0f]);
    }
    return out;
}

}  // namespace panko
