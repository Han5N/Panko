#include "protocol/packets.h"

#include <cstring>

namespace panko::protocol {

std::vector<uint8_t> frame(const Packet& packet) {
    Buffer body;
    packet.encode(body);

    Buffer idbuf;
    idbuf.writeVarUInt(packet.id());

    Buffer out;
    out.writeVarUInt(static_cast<uint32_t>(idbuf.size() + body.size()));
    out.writeBytes(idbuf.bytes());
    out.writeBytes(body.bytes());
    return out.bytes();
}

void LoginPacket::encode(Buffer& out) const {
    Buffer payload;
    payload.writeU32LE(static_cast<uint32_t>(chain.size()));
    payload.writeBytes(reinterpret_cast<const uint8_t*>(chain.data()), chain.size());
    payload.writeU32LE(static_cast<uint32_t>(skin.size()));
    payload.writeBytes(reinterpret_cast<const uint8_t*>(skin.data()), skin.size());

    // 登录包协议版本使用大端序，这是与其它字段不同的地方。
    out.writeU8(static_cast<uint8_t>(protocol_version >> 24));
    out.writeU8(static_cast<uint8_t>(protocol_version >> 16));
    out.writeU8(static_cast<uint8_t>(protocol_version >> 8));
    out.writeU8(static_cast<uint8_t>(protocol_version));
    out.writeVarUInt(static_cast<uint32_t>(payload.size()));
    out.writeBytes(payload.bytes());
}

void RequestNetworkSettings::encode(Buffer& out) const {
    out.writeU8(static_cast<uint8_t>(protocol_version >> 24));
    out.writeU8(static_cast<uint8_t>(protocol_version >> 16));
    out.writeU8(static_cast<uint8_t>(protocol_version >> 8));
    out.writeU8(static_cast<uint8_t>(protocol_version));
}

void NetworkSettings::decode(Buffer& in) {
    compression_threshold = in.readU16LE();
    compression_algorithm = in.readU16LE();
    client_throttle = static_cast<int8_t>(in.readU8());
    client_throttle_threshold = static_cast<int8_t>(in.readU8());
    client_throttle_scalar = in.readF32LE();
}

void ServerToClientHandshake::decode(Buffer& in) { jwt = in.readString(); }

void PlayStatus::decode(Buffer& in) {
    // 状态字段是大端序。
    uint32_t v = 0;
    for (int i = 0; i < 4; ++i) v = (v << 8) | in.readU8();
    status = v;
}

void DisconnectPacket::decode(Buffer& in) {
    reason = static_cast<int32_t>(in.readVarUInt());
    message = in.readString();
    if (!in.eof()) {
        filtered_message = in.readString();
    }
}

void ResourcePackClientResponse::encode(Buffer& out) const {
    out.writeU8(status);
    out.writeU16LE(static_cast<uint16_t>(pack_ids.size()));
    for (const auto& id : pack_ids) {
        out.writeString(id);
    }
}

void StartGame::decode(Buffer& in) {
    entity_id = in.readZigZag64();
    runtime_id = in.readVarUInt64();
    gamemode = in.readZigZag32();
    x = in.readF32LE();
    y = in.readF32LE();
    z = in.readF32LE();
    pitch = in.readF32LE();
    yaw = in.readF32LE();
    head_yaw = in.readF32LE();

    // 剩余字段数量多且随版本变化，先保留原始数据供后续解析。
    raw = in.readBytes(in.remaining());
}

void TextPacket::encode(Buffer& out) const {
    out.writeU8(type);
    out.writeU8(needs_translation ? 1 : 0);
    if (type == 1 || type == 2) {
        out.writeString(source);
        out.writeString(message);
        out.writeU32LE(xuid);
    } else if (type == 8 || type == 9) {
        out.writeString(source);
        out.writeString(message);
    } else {
        out.writeString(message);
    }
}

void TextPacket::decode(Buffer& in) {
    type = in.readU8();
    needs_translation = in.readU8() != 0;

    switch (type) {
        case 1:
            source = in.readString();
            message = in.readString();
            xuid = in.readU32LE();
            break;
        case 2:
            source = in.readString();
            message = in.readString();
            break;
        case 8:
        case 9:
            source = in.readString();
            message = in.readString();
            break;
        default:
            message = in.readString();
            break;
    }
}

void CommandRequest::encode(Buffer& out) const {
    out.writeString(command);
    out.writeZigZag32(static_cast<int32_t>(origin));

    std::string raw_uuid = uuid;
    raw_uuid.resize(16, '\0');
    out.writeBytes(reinterpret_cast<const uint8_t*>(raw_uuid.data()), 16);

    out.writeString(request_id);
    if (origin == kOriginDevConsole || origin == kOriginTest) {
        out.writeZigZag64(player_id);
    }
    out.writeU8(internal ? 1 : 0);
    out.writeZigZag32(version);
    out.writeU8(unlimit ? 1 : 0);
}

void CommandOutput::decode(Buffer& in) {
    origin = in.readVarUInt();

    std::string raw_uuid;
    raw_uuid.resize(16);
    for (int i = 0; i < 16; ++i) {
        raw_uuid[static_cast<size_t>(i)] = static_cast<char>(in.readU8());
    }
    request_id = in.readString();

    if (origin == kOriginDevConsole || origin == kOriginTest) {
        in.readZigZag64();
    }

    output_type = in.readU8();
    success_count = in.readVarUInt();

    uint32_t count = in.readVarUInt();
    messages.reserve(count);
    for (uint32_t i = 0; i < count; ++i) {
        CommandOutputMessage msg;
        msg.success = in.readU8() != 0;
        msg.message = in.readString();
        uint32_t params = in.readVarUInt();
        msg.parameters.reserve(params);
        for (uint32_t j = 0; j < params; ++j) {
            msg.parameters.push_back(in.readString());
        }
        messages.push_back(std::move(msg));
    }

    if (output_type == 4 && !in.eof()) {
        data_set = in.readString();
    }
}

void RequestChunkRadius::encode(Buffer& out) const {
    out.writeZigZag32(radius);
    out.writeU8(max_radius);
}

void ChunkRadiusUpdated::decode(Buffer& in) { radius = in.readZigZag32(); }

}  // namespace panko::protocol
