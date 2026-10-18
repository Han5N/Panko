#pragma once

#include <cstdint>
#include <stdexcept>
#include <string>
#include <vector>

namespace panko {

// ParseError 表示数据包字段不足或格式非法。解析错误应当快速失败，
// 不要用默认值掩盖协议不匹配。
class ParseError : public std::runtime_error {
public:
    using std::runtime_error::runtime_error;
};

// Buffer 同时承担读写：写入追加到末尾，读取从 pos 前进。
class Buffer {
public:
    Buffer() = default;
    explicit Buffer(std::vector<uint8_t> data) : data_(std::move(data)) {}

    const uint8_t* data() const { return data_.data(); }
    size_t size() const { return data_.size(); }
    size_t pos() const { return pos_; }
    size_t remaining() const { return data_.size() - pos_; }
    bool eof() const { return pos_ >= data_.size(); }
    void resetRead() { pos_ = 0; }
    std::vector<uint8_t>& bytes() { return data_; }
    const std::vector<uint8_t>& bytes() const { return data_; }

    void writeU8(uint8_t v);
    void writeU16LE(uint16_t v);
    void writeU32LE(uint32_t v);
    void writeU64LE(uint64_t v);
    void writeI32LE(int32_t v);
    void writeI64LE(int64_t v);
    void writeF32LE(float v);
    void writeF64LE(double v);
    void writeVarUInt(uint32_t v);
    void writeVarUInt64(uint64_t v);
    void writeZigZag32(int32_t v);
    void writeZigZag64(int64_t v);
    void writeString(const std::string& s);
    void writeBytes(const uint8_t* p, size_t n);
    void writeBytes(const std::vector<uint8_t>& v) { writeBytes(v.data(), v.size()); }

    uint8_t readU8();
    uint16_t readU16LE();
    uint32_t readU32LE();
    uint64_t readU64LE();
    int32_t readI32LE();
    int64_t readI64LE();
    float readF32LE();
    double readF64LE();
    uint32_t readVarUInt();
    uint64_t readVarUInt64();
    int32_t readZigZag32();
    int64_t readZigZag64();
    std::string readString();
    std::vector<uint8_t> readBytes(size_t n);
    void skip(size_t n);

private:
    void need(size_t n) const;

    std::vector<uint8_t> data_;
    size_t pos_ = 0;
};

std::string hexDump(const uint8_t* data, size_t size);

}  // namespace panko
