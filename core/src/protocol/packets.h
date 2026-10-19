#pragma once

#include <cstdint>
#include <string>
#include <vector>

#include "common/buffer.h"

namespace panko::protocol {

// 数据包 ID 取自 NTvector 对网易版协议的整理。ID 在帧里以 varuint 编码。
enum PacketId : uint32_t {
    kLogin = 1,
    kPlayStatus = 2,
    kServerToClientHandshake = 3,
    kClientToServerHandshake = 4,
    kDisconnect = 5,
    kResourcePacksInfo = 6,
    kResourcePackStack = 7,
    kResourcePackClientResponse = 8,
    kText = 9,
    kSetTime = 10,
    kStartGame = 11,
    kLevelChunk = 58,
    kPlayerList = 63,
    kRequestChunkRadius = 69,
    kChunkRadiusUpdated = 70,
    kCommandRequest = 77,
    kCommandOutput = 79,
    kNetworkSettings = 143,
    kSubChunk = 174,
    kRequestNetworkSettings = 193,
};

// 命令来源类型，与协议保持一致。
enum CommandOriginType : uint32_t {
    kOriginPlayer = 0,
    kOriginDevConsole = 3,
    kOriginTest = 4,
};

// 资源包响应状态。
enum ResourcePackStatus : uint8_t {
    kResourcePackRefused = 1,
    kResourcePackSendPacks = 2,
    kResourcePackHaveAllPacks = 3,
    kResourcePackCompleted = 4,
};

struct Packet {
    virtual ~Packet() = default;
    virtual uint32_t id() const = 0;
    virtual void encode(Buffer&) const {}
    virtual void decode(Buffer&) {}
};

// frame 生成完整帧：varuint 长度 + varuint 包 ID + 包体。
std::vector<uint8_t> frame(const Packet& packet);

// 不解析具体字段、只保留原始负载的数据包。
struct RawPacket : Packet {
    uint32_t packet_id = 0;
    std::vector<uint8_t> body;

    explicit RawPacket(uint32_t id = 0) : packet_id(id) {}
    uint32_t id() const override { return packet_id; }
    void encode(Buffer& out) const override { out.writeBytes(body); }
};

struct LoginPacket : Packet {
    uint32_t protocol_version = 860;
    std::string chain;
    std::string skin;

    uint32_t id() const override { return kLogin; }
    void encode(Buffer& out) const override;
};

struct RequestNetworkSettings : Packet {
    uint32_t protocol_version = 860;

    uint32_t id() const override { return kRequestNetworkSettings; }
    void encode(Buffer& out) const override;
};

struct NetworkSettings : Packet {
    uint16_t compression_threshold = 0;
    uint16_t compression_algorithm = 0;
    int8_t client_throttle = 0;
    int8_t client_throttle_threshold = 0;
    float client_throttle_scalar = 0;

    uint32_t id() const override { return kNetworkSettings; }
    void decode(Buffer& in) override;
};

struct ServerToClientHandshake : Packet {
    std::string jwt;

    uint32_t id() const override { return kServerToClientHandshake; }
    void decode(Buffer& in) override;
};

struct ClientToServerHandshake : Packet {
    uint32_t id() const override { return kClientToServerHandshake; }
};

struct PlayStatus : Packet {
    uint32_t status = 0;

    uint32_t id() const override { return kPlayStatus; }
    void decode(Buffer& in) override;
};

struct DisconnectPacket : Packet {
    int32_t reason = 0;
    std::string message;
    std::string filtered_message;

    uint32_t id() const override { return kDisconnect; }
    void decode(Buffer& in) override;
};

struct ResourcePackClientResponse : Packet {
    uint8_t status = kResourcePackHaveAllPacks;
    std::vector<std::string> pack_ids;

    uint32_t id() const override { return kResourcePackClientResponse; }
    void encode(Buffer& out) const override;
};

struct StartGame : Packet {
    int64_t entity_id = 0;
    uint64_t runtime_id = 0;
    int32_t gamemode = 0;
    float x = 0, y = 0, z = 0;
    float pitch = 0, yaw = 0, head_yaw = 0;
    // 其余字段尚未解析，保留原始负载，便于后续补齐。
    std::vector<uint8_t> raw;

    uint32_t id() const override { return kStartGame; }
    void decode(Buffer& in) override;
};

struct TextPacket : Packet {
    uint8_t type = 1;
    bool needs_translation = false;
    std::string source;
    std::string message;
    uint32_t xuid = 0;

    uint32_t id() const override { return kText; }
    void encode(Buffer& out) const override;
    void decode(Buffer& in) override;
};

struct CommandRequest : Packet {
    std::string command;
    uint32_t origin = kOriginPlayer;
    std::string uuid;       // 16 字节，不足补零
    std::string request_id;
    int64_t player_id = 0;  // 仅 DevConsole / Test 来源需要
    bool internal = false;
    int32_t version = 52;
    bool unlimit = false;

    uint32_t id() const override { return kCommandRequest; }
    void encode(Buffer& out) const override;
};

struct CommandOutputMessage {
    bool success = false;
    std::string message;
    std::vector<std::string> parameters;
};

struct CommandOutput : Packet {
    uint32_t origin = 0;
    std::string request_id;
    uint8_t output_type = 0;
    uint32_t success_count = 0;
    std::vector<CommandOutputMessage> messages;
    std::string data_set;

    uint32_t id() const override { return kCommandOutput; }
    void decode(Buffer& in) override;
};

struct RequestChunkRadius : Packet {
    int32_t radius = 8;
    uint8_t max_radius = 8;

    uint32_t id() const override { return kRequestChunkRadius; }
    void encode(Buffer& out) const override;
};

struct ChunkRadiusUpdated : Packet {
    int32_t radius = 0;

    uint32_t id() const override { return kChunkRadiusUpdated; }
    void decode(Buffer& in) override;
};

// SubChunk 是网易版按需返回区块段的方式，一次可携带多个段。
struct SubChunkEntry {
    int8_t dx = 0;
    int8_t dy = 0;
    int8_t dz = 0;
    uint8_t result = 0;
    std::vector<uint8_t> payload;
};

struct SubChunkPacket : Packet {
    uint8_t cache_enabled = 0;
    int32_t dimension = 0;
    int32_t origin_x = 0;
    int32_t origin_y = 0;
    int32_t origin_z = 0;
    uint32_t count = 0;
    std::vector<SubChunkEntry> entries;

    uint32_t id() const override { return kSubChunk; }
    void decode(Buffer& in) override;
};

}  // namespace panko::protocol
