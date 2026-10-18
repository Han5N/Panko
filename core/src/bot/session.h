#pragma once

#include <cstdint>
#include <deque>
#include <functional>
#include <mutex>
#include <string>
#include <vector>

#include <nlohmann/json.hpp>

namespace panko {

struct Event {
    uint64_t seq = 0;
    std::string topic;
    std::string time;
    nlohmann::json payload;
};

// Session 维护机器人的对外状态。当前版本使用内置的模拟会话，
// 真实连接层（RakNet + 登录链）接入时替换 session.cpp 内部的驱动部分，
// 对外的方法与事件格式保持不变。
class Session {
public:
    using EventSink = std::function<void(const Event&)>;

    Session(std::string server_address, std::string player_name, uint32_t protocol_version);

    void setEventSink(EventSink sink);
    void start();
    void stop();

    nlohmann::json status() const;
    nlohmann::json say(const std::string& text);
    nlohmann::json command(const std::string& command);
    nlohmann::json block(int x, int y, int z) const;
    nlohmann::json blocks(const std::vector<nlohmann::json>& positions) const;
    nlohmann::json entities(double x, double y, double z, double radius) const;
    nlohmann::json place(const nlohmann::json& blocks);

    std::vector<Event> replay(uint64_t since) const;
    uint64_t lastSeq() const;

private:
    void emit(const std::string& topic, nlohmann::json payload);

    mutable std::mutex mu_;
    std::string server_address_;
    std::string player_name_;
    uint32_t protocol_version_;
    std::string state_ = "offline";
    int64_t online_since_ = 0;
    uint64_t seq_ = 0;
    std::deque<Event> ring_;
    EventSink sink_;
};

int64_t unixNow();
std::string isoNow();

}  // namespace panko
