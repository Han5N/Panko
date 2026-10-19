#include "protocol/packets.h"
#include "test.h"

using namespace panko;
using namespace panko::protocol;

namespace {

struct DecodedFrame {
    uint32_t id = 0;
    std::vector<uint8_t> body;
};

DecodedFrame decodeFrame(const std::vector<uint8_t>& bytes) {
    Buffer outer(bytes);
    uint32_t len = outer.readVarUInt();
    auto payload = outer.readBytes(len);
    Buffer inner(payload);
    DecodedFrame out;
    out.id = inner.readVarUInt();
    out.body = inner.readBytes(inner.remaining());
    return out;
}

}  // namespace

TEST_CASE(frame_uses_varuint_id) {
    RawPacket packet(kRequestNetworkSettings);
    auto bytes = frame(packet);

    Buffer outer(bytes);
    uint32_t len = outer.readVarUInt();
    CHECK_EQ(len, static_cast<uint32_t>(2));

    // 193 的 varuint 编码是 C1 01，不能写成单个字节。
    CHECK_EQ(bytes[1], uint8_t{0xC1});
    CHECK_EQ(bytes[2], uint8_t{0x01});
}

TEST_CASE(request_network_settings_layout) {
    RequestNetworkSettings packet;
    packet.protocol_version = 860;
    auto decoded = decodeFrame(frame(packet));

    CHECK_EQ(decoded.id, static_cast<uint32_t>(kRequestNetworkSettings));
    CHECK_EQ(decoded.body.size(), size_t{4});
    CHECK_EQ(decoded.body[0], uint8_t{0x00});
    CHECK_EQ(decoded.body[1], uint8_t{0x00});
    CHECK_EQ(decoded.body[2], uint8_t{0x03});
    CHECK_EQ(decoded.body[3], uint8_t{0x5C});
}

TEST_CASE(login_layout) {
    LoginPacket packet;
    packet.protocol_version = 860;
    packet.chain = "abc";
    packet.skin = "xy";

    auto decoded = decodeFrame(frame(packet));
    CHECK_EQ(decoded.id, static_cast<uint32_t>(kLogin));

    Buffer body(decoded.body);
    CHECK_EQ(body.readU8(), uint8_t{0x00});
    CHECK_EQ(body.readU8(), uint8_t{0x00});
    CHECK_EQ(body.readU8(), uint8_t{0x03});
    CHECK_EQ(body.readU8(), uint8_t{0x5C});
    CHECK_EQ(body.readVarUInt(), 13u);
    CHECK_EQ(body.readU32LE(), 3u);
    auto chain = body.readBytes(3);
    CHECK_EQ(std::string(chain.begin(), chain.end()), std::string("abc"));
    CHECK_EQ(body.readU32LE(), 2u);
    auto skin = body.readBytes(2);
    CHECK_EQ(std::string(skin.begin(), skin.end()), std::string("xy"));
}

TEST_CASE(text_roundtrip) {
    TextPacket packet;
    packet.type = 1;
    packet.source = "PankoBot";
    packet.message = "hello";
    packet.xuid = 42;

    auto decoded = decodeFrame(frame(packet));
    CHECK_EQ(decoded.id, static_cast<uint32_t>(kText));

    Buffer body(decoded.body);
    TextPacket parsed;
    parsed.decode(body);

    CHECK_EQ(parsed.type, uint8_t{1});
    CHECK_EQ(parsed.source, std::string("PankoBot"));
    CHECK_EQ(parsed.message, std::string("hello"));
    CHECK_EQ(parsed.xuid, 42u);
}

TEST_CASE(command_request_layout) {
    CommandRequest packet;
    packet.command = "say hi";
    packet.request_id = "req-1";
    packet.version = 52;

    auto decoded = decodeFrame(frame(packet));
    CHECK_EQ(decoded.id, static_cast<uint32_t>(kCommandRequest));

    Buffer body(decoded.body);
    CHECK_EQ(body.readString(), std::string("say hi"));
    CHECK_EQ(body.readZigZag32(), 0);
    for (int i = 0; i < 16; ++i) {
        CHECK_EQ(body.readU8(), uint8_t{0});
    }
    CHECK_EQ(body.readString(), std::string("req-1"));
    CHECK_EQ(body.readU8(), uint8_t{0});
    CHECK_EQ(body.readZigZag32(), 52);
    CHECK_EQ(body.readU8(), uint8_t{0});
}

TEST_CASE(start_game_prefix) {
    Buffer body;
    body.writeZigZag64(-123);
    body.writeVarUInt64(456);
    body.writeZigZag32(1);
    body.writeF32LE(1.5f);
    body.writeF32LE(64.0f);
    body.writeF32LE(-2.5f);
    body.writeF32LE(0.0f);
    body.writeF32LE(90.0f);
    body.writeF32LE(90.0f);
    body.writeString("world");

    StartGame packet;
    packet.decode(body);

    CHECK_EQ(packet.entity_id, int64_t{-123});
    CHECK_EQ(packet.runtime_id, uint64_t{456});
    CHECK_EQ(packet.gamemode, 1);
    CHECK(packet.x == 1.5f);
    CHECK(packet.y == 64.0f);
    CHECK(packet.z == -2.5f);
    CHECK(packet.yaw == 90.0f);
}

TEST_CASE(command_output_decode) {
    Buffer body;
    body.writeVarUInt(0);  // origin = player
    for (int i = 0; i < 16; ++i) body.writeU8(0);
    body.writeString("req-1");
    body.writeU8(0);  // output type
    body.writeVarUInt(1);
    body.writeVarUInt(1);
    body.writeU8(1);
    body.writeString("done");
    body.writeVarUInt(1);
    body.writeString("arg");

    CommandOutput packet;
    packet.decode(body);

    CHECK_EQ(packet.request_id, std::string("req-1"));
    CHECK_EQ(packet.success_count, 1u);
    CHECK_EQ(packet.messages.size(), size_t{1});
    CHECK_EQ(packet.messages[0].message, std::string("done"));
    CHECK_EQ(packet.messages[0].parameters.size(), size_t{1});
}

TEST_CASE(network_settings_decode) {
    Buffer body;
    body.writeU16LE(512);
    body.writeU16LE(1);
    body.writeU8(0);
    body.writeU8(0);
    body.writeF32LE(1.0f);

    NetworkSettings packet;
    packet.decode(body);

    CHECK_EQ(packet.compression_threshold, uint16_t{512});
    CHECK_EQ(packet.compression_algorithm, uint16_t{1});
    CHECK(packet.client_throttle_scalar == 1.0f);
}

TEST_CASE(subchunk_packet_decode) {
    Buffer body;
    body.writeU8(0);          // cache 关闭
    body.writeZigZag32(0);    // 维度
    body.writeZigZag32(0);    // origin x
    body.writeZigZag32(0);    // origin y
    body.writeZigZag32(3);    // origin z
    body.writeU32LE(1);       // 条目数
    body.writeU8(0);          // dx
    body.writeU8(0xFF);       // dy = -1
    body.writeU8(0);          // dz
    body.writeU8(1);          // result = success
    std::vector<uint8_t> payload = {9, 1, 3, 2};
    body.writeVarUInt(static_cast<uint32_t>(payload.size()));
    body.writeBytes(payload);
    body.writeU8(0);          // 无高度图
    body.writeU8(0);          // 无渲染高度图

    SubChunkPacket packet;
    packet.decode(body);

    CHECK_EQ(packet.dimension, 0);
    CHECK_EQ(packet.origin_z, 3);
    CHECK_EQ(packet.count, 1u);
    CHECK_EQ(packet.entries.size(), size_t{1});
    CHECK_EQ(packet.entries[0].dy, int8_t{-1});
    CHECK_EQ(packet.entries[0].result, uint8_t{1});
    CHECK_EQ(packet.entries[0].payload.size(), size_t{4});
}

TEST_CASE(chunk_radius_roundtrip) {
    RequestChunkRadius request;
    request.radius = 8;
    request.max_radius = 8;

    auto decoded = decodeFrame(frame(request));
    CHECK_EQ(decoded.id, static_cast<uint32_t>(kRequestChunkRadius));

    Buffer body(decoded.body);
    CHECK_EQ(body.readZigZag32(), 8);
    CHECK_EQ(body.readU8(), uint8_t{8});

    Buffer updated;
    updated.writeZigZag32(6);
    ChunkRadiusUpdated packet;
    packet.decode(updated);
    CHECK_EQ(packet.radius, 6);
}
