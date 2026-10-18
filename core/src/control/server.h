#pragma once

#include <atomic>
#include <cstdint>
#include <map>
#include <memory>
#include <mutex>
#include <string>
#include <thread>

#include "bot/session.h"

namespace panko {

// ControlServer 在本地暴露行分隔 JSON 接口，供 hub 调用。
// 只监听回环地址；协议与 hub/internal/coreclient 保持一致。
class ControlServer {
public:
    ControlServer(std::string addr, Session& session);
    ~ControlServer();

    // run 阻塞直到 stop 被调用。
    void run();
    void stop();

    // broadcast 把事件推送给所有已订阅的客户端。
    void broadcast(const Event& event);

private:
    struct Client {
        intptr_t fd = -1;
        std::mutex write_mu;
        bool subscribed = false;
    };

    void acceptLoop();
    void handleClient(std::shared_ptr<Client> client);
    void dispatch(std::shared_ptr<Client> client, const nlohmann::json& request);
    void sendJson(std::shared_ptr<Client> client, const nlohmann::json& msg);
    void sendEvent(std::shared_ptr<Client> client, const Event& event);
    void removeClient(intptr_t fd);

    std::string addr_;
    Session& session_;

    std::atomic<bool> running_{false};
    intptr_t listen_fd_ = -1;

    std::mutex clients_mu_;
    std::map<intptr_t, std::shared_ptr<Client>> clients_;
    std::mutex subs_mu_;
};

}  // namespace panko
