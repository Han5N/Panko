#include "control/server.h"

#include <cerrno>
#include <cstdio>
#include <cstring>
#include <stdexcept>

#include <nlohmann/json.hpp>

#if defined(_WIN32)
#include <winsock2.h>
#include <ws2tcpip.h>
using socket_t = SOCKET;
constexpr socket_t kInvalidSocket = INVALID_SOCKET;
static void closeSocket(socket_t s) { closesocket(s); }
#else
#include <arpa/inet.h>
#include <netinet/in.h>
#include <sys/socket.h>
#include <unistd.h>
using socket_t = int;
constexpr socket_t kInvalidSocket = -1;
static void closeSocket(socket_t s) { ::close(s); }
#endif

namespace panko {

namespace {

constexpr size_t kMaxLine = 4 * 1024 * 1024;

bool splitAddress(const std::string& addr, std::string& host, uint16_t& port) {
    auto pos = addr.rfind(':');
    if (pos == std::string::npos) return false;
    host = addr.substr(0, pos);
    try {
        port = static_cast<uint16_t>(std::stoi(addr.substr(pos + 1)));
    } catch (...) {
        return false;
    }
    return !host.empty();
}

void writeAll(socket_t fd, const std::string& data) {
    size_t sent = 0;
    while (sent < data.size()) {
        int n = static_cast<int>(::send(fd, data.data() + sent,
                                        static_cast<int>(data.size() - sent), 0));
        if (n <= 0) return;
        sent += static_cast<size_t>(n);
    }
}

}  // namespace

ControlServer::ControlServer(std::string addr, Session& session)
    : addr_(std::move(addr)), session_(session) {}

ControlServer::~ControlServer() { stop(); }

void ControlServer::run() {
    std::string host;
    uint16_t port = 0;
    if (!splitAddress(addr_, host, port)) {
        throw std::runtime_error("无效的监听地址: " + addr_);
    }

#if defined(_WIN32)
    WSADATA wsa{};
    if (WSAStartup(MAKEWORD(2, 2), &wsa) != 0) {
        throw std::runtime_error("WSAStartup 失败");
    }
#endif

    socket_t fd = ::socket(AF_INET, SOCK_STREAM, 0);
    if (fd == kInvalidSocket) {
        throw std::runtime_error("创建 socket 失败");
    }

    int one = 1;
    ::setsockopt(fd, SOL_SOCKET, SO_REUSEADDR, reinterpret_cast<const char*>(&one),
                 sizeof(one));

    sockaddr_in addr{};
    addr.sin_family = AF_INET;
    addr.sin_port = htons(port);
    if (::inet_pton(AF_INET, host.c_str(), &addr.sin_addr) != 1) {
        closeSocket(fd);
        throw std::runtime_error("解析监听地址失败: " + host);
    }

    if (::bind(fd, reinterpret_cast<sockaddr*>(&addr), sizeof(addr)) != 0) {
        closeSocket(fd);
        throw std::runtime_error("绑定 " + addr_ + " 失败: " + std::strerror(errno));
    }
    if (::listen(fd, 8) != 0) {
        closeSocket(fd);
        throw std::runtime_error("监听失败: " + std::string(std::strerror(errno)));
    }

    listen_fd_ = static_cast<intptr_t>(fd);
    running_ = true;
    acceptLoop();
}

void ControlServer::stop() {
    running_ = false;
    if (listen_fd_ != -1) {
        closeSocket(static_cast<socket_t>(listen_fd_));
        listen_fd_ = -1;
    }

    std::lock_guard<std::mutex> lock(clients_mu_);
    for (auto& [fd, client] : clients_) {
        closeSocket(static_cast<socket_t>(fd));
        client->fd = -1;
    }
    clients_.clear();
}

void ControlServer::acceptLoop() {
    while (running_) {
        sockaddr_in peer{};
#if defined(_WIN32)
        int len = sizeof(peer);
#else
        socklen_t len = sizeof(peer);
#endif

        socket_t fd = ::accept(static_cast<socket_t>(listen_fd_),
                               reinterpret_cast<sockaddr*>(&peer), &len);
        if (fd == kInvalidSocket) {
            if (!running_) break;
            continue;
        }

        auto client = std::make_shared<Client>();
        client->fd = static_cast<intptr_t>(fd);

        {
            std::lock_guard<std::mutex> lock(clients_mu_);
            clients_[client->fd] = client;
        }

        std::thread([this, client]() { handleClient(client); }).detach();
    }
}

void ControlServer::handleClient(std::shared_ptr<Client> client) {
    socket_t fd = static_cast<socket_t>(client->fd);
    std::string buffer;
    char chunk[65536];

    while (running_) {
        int n = static_cast<int>(::recv(fd, chunk, sizeof(chunk), 0));
        if (n <= 0) break;
        buffer.append(chunk, static_cast<size_t>(n));

        size_t pos = 0;
        while (true) {
            auto nl = buffer.find('\n', pos);
            if (nl == std::string::npos) break;
            std::string line = buffer.substr(pos, nl - pos);
            pos = nl + 1;

            if (line.empty()) continue;
            try {
                auto request = nlohmann::json::parse(line);
                dispatch(client, request);
            } catch (const std::exception& e) {
                nlohmann::json err = {
                    {"jsonrpc", "2.0"},
                    {"error", {{"code", "parse_error"}, {"message", e.what()}}},
                };
                sendJson(client, err);
            }
        }
        if (pos > 0) buffer.erase(0, pos);
        if (buffer.size() > kMaxLine) {
            break;
        }
    }

    closeSocket(fd);
    client->fd = -1;
    removeClient(static_cast<intptr_t>(fd));
}

void ControlServer::dispatch(std::shared_ptr<Client> client, const nlohmann::json& request) {
    nlohmann::json id = request.value("id", nlohmann::json());
    std::string method = request.value("method", "");
    nlohmann::json params = request.value("params", nlohmann::json::object());

    auto reply = [&](nlohmann::json result) {
        sendJson(client, {{"id", id}, {"result", std::move(result)}});
    };
    auto fail = [&](const std::string& code, const std::string& message) {
        sendJson(client, {{"id", id}, {"error", {{"code", code}, {"message", message}}}});
    };

    try {
        if (method == "events.subscribe") {
            uint64_t since = params.value("since", 0ULL);
            std::lock_guard<std::mutex> lock(subs_mu_);
            for (const auto& ev : session_.replay(since)) {
                sendEvent(client, ev);
            }
            client->subscribed = true;
            reply({{"since", since}});
            return;
        }

        if (method == "bot.status") {
            reply(session_.status());
            return;
        }

        if (method == "bot.say") {
            if (!params.contains("text") || !params["text"].is_string()) {
                fail("invalid_params", "text 必须是字符串");
                return;
            }
            reply(session_.say(params["text"].get<std::string>()));
            return;
        }

        if (method == "bot.command") {
            if (!params.contains("command") || !params["command"].is_string()) {
                fail("invalid_params", "command 必须是字符串");
                return;
            }
            reply(session_.command(params["command"].get<std::string>()));
            return;
        }

        if (method == "world.block") {
            int x = params.value("x", 0);
            int y = params.value("y", 0);
            int z = params.value("z", 0);
            reply(session_.block(x, y, z));
            return;
        }

        if (method == "world.blocks") {
            if (!params.contains("positions") || !params["positions"].is_array()) {
                fail("invalid_params", "positions 必须是数组");
                return;
            }
            reply(session_.blocks(params["positions"].get<std::vector<nlohmann::json>>()));
            return;
        }

        if (method == "world.entities") {
            double x = params.value("x", 0.0);
            double y = params.value("y", 0.0);
            double z = params.value("z", 0.0);
            double radius = params.value("radius", 16.0);
            reply(session_.entities(x, y, z, radius));
            return;
        }

        if (method == "world.place") {
            if (!params.contains("blocks")) {
                fail("invalid_params", "缺少 blocks");
                return;
            }
            reply(session_.place(params["blocks"]));
            return;
        }

        fail("method_not_found", "未知方法 " + method);
    } catch (const std::exception& e) {
        fail("failed", e.what());
    }
}

void ControlServer::sendJson(std::shared_ptr<Client> client, const nlohmann::json& msg) {
    if (client->fd == -1) return;
    std::lock_guard<std::mutex> lock(client->write_mu);
    std::string line = msg.dump();
    line.push_back('\n');
    writeAll(static_cast<socket_t>(client->fd), line);
}

void ControlServer::sendEvent(std::shared_ptr<Client> client, const Event& event) {
    nlohmann::json msg = {
        {"event",
         {{"seq", event.seq},
          {"topic", event.topic},
          {"time", event.time},
          {"payload", event.payload}}},
    };
    sendJson(client, msg);
}

void ControlServer::broadcast(const Event& event) {
    std::lock_guard<std::mutex> lock(subs_mu_);
    for (auto& [fd, client] : clients_) {
        if (client->subscribed) sendEvent(client, event);
    }
}

void ControlServer::removeClient(intptr_t fd) {
    std::lock_guard<std::mutex> lock(clients_mu_);
    clients_.erase(fd);
}

}  // namespace panko
