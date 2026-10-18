#include <atomic>
#include <chrono>
#include <csignal>
#include <cstdio>
#include <fstream>
#include <thread>

#include <nlohmann/json.hpp>

#include "bot/session.h"
#include "control/server.h"

namespace {

volatile std::sig_atomic_t g_stop = 0;

void onSignal(int) { g_stop = 1; }

std::string nowText() {
    char buf[32];
    std::time_t t = std::time(nullptr);
    std::tm tm{};
#if defined(_WIN32)
    localtime_s(&tm, &t);
#else
    localtime_r(&t, &tm);
#endif
    std::strftime(buf, sizeof(buf), "%H:%M:%S", &tm);
    return buf;
}

void logLine(const std::string& text) {
    std::printf("[%s] %s\n", nowText().c_str(), text.c_str());
    std::fflush(stdout);
}

}  // namespace

int main(int argc, char** argv) {
    std::string config_path = "core.json";
    for (int i = 1; i < argc; ++i) {
        std::string arg = argv[i];
        if ((arg == "--config" || arg == "-c") && i + 1 < argc) {
            config_path = argv[++i];
        }
    }

    nlohmann::json cfg;
    {
        std::ifstream in(config_path);
        if (!in) {
            std::fprintf(stderr, "无法读取配置 %s\n", config_path.c_str());
            return 1;
        }
        try {
            in >> cfg;
        } catch (const std::exception& e) {
            std::fprintf(stderr, "解析配置失败: %s\n", e.what());
            return 1;
        }
    }

    std::string addr = cfg.value("core", nlohmann::json::object()).value("addr", "127.0.0.1:3039");
    auto session_cfg = cfg.value("session", nlohmann::json::object());
    std::string server = session_cfg.value("server_address", "sim.local:19132");
    std::string player = session_cfg.value("player_name", "PankoBot");
    uint32_t protocol = session_cfg.value("protocol_version", 860);

    panko::Session session(server, player, protocol);
    panko::ControlServer server_ctl(addr, session);

    session.setEventSink([&server_ctl](const panko::Event& event) {
        server_ctl.broadcast(event);
    });

    std::signal(SIGINT, onSignal);
    std::signal(SIGTERM, onSignal);

    std::thread server_thread([&server_ctl]() {
        try {
            server_ctl.run();
        } catch (const std::exception& e) {
            std::fprintf(stderr, "控制端口启动失败: %s\n", e.what());
        }
    });

    std::this_thread::sleep_for(std::chrono::milliseconds(120));

    logLine("Panko botd 启动");
    logLine("控制接口: tcp://" + addr);
    logLine("会话模式: 内置模拟（真实 RakNet 登录尚未接入）");

    session.start();

    while (!g_stop) {
        std::this_thread::sleep_for(std::chrono::milliseconds(200));
    }

    logLine("正在退出");
    session.stop();
    server_ctl.stop();
    if (server_thread.joinable()) server_thread.join();

    return 0;
}
