#include "bot/session.h"

#include <chrono>
#include <cstdio>
#include <ctime>
#include <stdexcept>

namespace panko {

namespace {

constexpr size_t kRingCapacity = 1024;

std::string formatTime(std::chrono::system_clock::time_point tp) {
    auto secs = std::chrono::time_point_cast<std::chrono::seconds>(tp);
    auto ms = std::chrono::duration_cast<std::chrono::milliseconds>(tp - secs).count();

    std::time_t t = std::chrono::system_clock::to_time_t(tp);
    std::tm tm{};
#if defined(_WIN32)
    gmtime_s(&tm, &t);
#else
    gmtime_r(&t, &tm);
#endif

    char buf[64];
    std::snprintf(buf, sizeof(buf), "%04d-%02d-%02dT%02d:%02d:%02d.%03dZ", tm.tm_year + 1900,
                  tm.tm_mon + 1, tm.tm_mday, tm.tm_hour, tm.tm_min, tm.tm_sec,
                  static_cast<int>(ms));
    return buf;
}

}  // namespace

int64_t unixNow() {
    return std::chrono::duration_cast<std::chrono::seconds>(
               std::chrono::system_clock::now().time_since_epoch())
        .count();
}

std::string isoNow() { return formatTime(std::chrono::system_clock::now()); }

Session::Session(std::string server_address, std::string player_name, uint32_t protocol_version)
    : server_address_(std::move(server_address)),
      player_name_(std::move(player_name)),
      protocol_version_(protocol_version) {}

void Session::setEventSink(EventSink sink) {
    std::lock_guard<std::mutex> lock(mu_);
    sink_ = std::move(sink);
}

void Session::start() {
    {
        std::lock_guard<std::mutex> lock(mu_);
        if (state_ == "online" || state_ == "connecting") return;
        state_ = "online";
        online_since_ = unixNow();
    }

    emit("session", {{"kind", "connected"},
                     {"reason", ""},
                     {"player_name", player_name_},
                     {"server_address", server_address_}});
}

void Session::stop() {
    bool was_online = false;
    {
        std::lock_guard<std::mutex> lock(mu_);
        was_online = state_ == "online";
        state_ = "offline";
        online_since_ = 0;
    }
    if (was_online) {
        emit("session", {{"kind", "disconnected"}, {"reason", "本地主动断开"}});
    }
}

nlohmann::json Session::status() const {
    std::lock_guard<std::mutex> lock(mu_);
    nlohmann::json out = {
        {"state", state_},
        {"player_name", player_name_},
        {"server_address", server_address_},
        {"protocol_version", std::to_string(protocol_version_)},
    };
    if (online_since_ != 0) {
        out["online_since"] = online_since_;
    }
    return out;
}

nlohmann::json Session::say(const std::string& text) {
    {
        std::lock_guard<std::mutex> lock(mu_);
        if (state_ != "online") {
            throw std::runtime_error("机器人未上线");
        }
    }
    emit("chat", {{"sender", player_name_}, {"message", text}});
    return {{"ok", true}};
}

nlohmann::json Session::command(const std::string& command) {
    {
        std::lock_guard<std::mutex> lock(mu_);
        if (state_ != "online") {
            throw std::runtime_error("机器人未上线");
        }
    }
    // 模拟会话不执行真实命令，只返回可预测的结果，供插件联调使用。
    return {{"ok", true}, {"output", "sim: " + command}};
}

nlohmann::json Session::block(int x, int y, int z) const {
    std::string name = "minecraft:air";
    if (y < 63) {
        name = "minecraft:stone";
    } else if (y == 63) {
        name = "minecraft:grass_block";
    }

    return {
        {"loaded", true},
        {"position", {{"x", x}, {"y", y}, {"z", z}}},
        {"block", {{"name", name}, {"states", nlohmann::json::object()}}},
    };
}

nlohmann::json Session::blocks(const std::vector<nlohmann::json>& positions) const {
    nlohmann::json out;
    out["blocks"] = nlohmann::json::array();
    for (const auto& pos : positions) {
        int x = pos.value("x", 0);
        int y = pos.value("y", 0);
        int z = pos.value("z", 0);
        auto one = block(x, y, z);
        out["blocks"].push_back(one["block"]);
    }
    return out;
}

nlohmann::json Session::entities(double x, double y, double z, double radius) const {
    (void)radius;
    return {{"entities", nlohmann::json::array({{{"runtime_id", 1},
                                                 {"type", "minecraft:player"},
                                                 {"name_tag", player_name_},
                                                 {"position", {{"x", x}, {"y", y}, {"z", z}}}}})}};
}

nlohmann::json Session::place(const nlohmann::json& blocks) {
    {
        std::lock_guard<std::mutex> lock(mu_);
        if (state_ != "online") {
            throw std::runtime_error("机器人未上线");
        }
    }

    if (!blocks.is_array()) {
        throw std::runtime_error("blocks 必须是数组");
    }

    int accepted = 0;
    for (const auto& item : blocks) {
        if (!item.is_object() || !item.contains("position")) continue;
        const auto& pos = item["position"];
        int x = pos.value("x", 0);
        int y = pos.value("y", 0);
        int z = pos.value("z", 0);
        auto before = block(x, y, z);
        std::string name = "minecraft:stone";
        if (item.contains("block") && item["block"].is_object()) {
            name = item["block"].value("name", name);
        }
        emit("block", {{"position", {{"x", x}, {"y", y}, {"z", z}}},
                       {"before", before["block"]},
                       {"after", {{"name", name}, {"states", nlohmann::json::object()}}}});
        ++accepted;
    }

    return {{"accepted", accepted}, {"rejected", 0}};
}

void Session::emit(const std::string& topic, nlohmann::json payload) {
    EventSink sink;
    Event ev;
    {
        std::lock_guard<std::mutex> lock(mu_);
        ev.seq = ++seq_;
        ev.topic = topic;
        ev.time = isoNow();
        ev.payload = std::move(payload);

        ring_.push_back(ev);
        if (ring_.size() > kRingCapacity) ring_.pop_front();
        sink = sink_;
    }
    if (sink) sink(ev);
}

std::vector<Event> Session::replay(uint64_t since) const {
    std::lock_guard<std::mutex> lock(mu_);
    std::vector<Event> out;
    for (const auto& ev : ring_) {
        if (ev.seq > since) out.push_back(ev);
    }
    return out;
}

uint64_t Session::lastSeq() const {
    std::lock_guard<std::mutex> lock(mu_);
    return seq_;
}

}  // namespace panko
