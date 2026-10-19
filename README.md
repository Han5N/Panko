# Panko

[![CI](https://github.com/Han5N/Panko/actions/workflows/ci.yml/badge.svg)](https://github.com/Han5N/Panko/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/Han5N/Panko?include_prereleases&label=release)](https://github.com/Han5N/Panko/releases)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/go-1.24%2B-00ADD8.svg)](go.mod)
[![C%2B%2B](https://img.shields.io/badge/C%2B%2B-20-00599C.svg)](core/CMakeLists.txt)

Panko 是一个面向 Minecraft 网易版基岩版的机器人框架。它以无界面方式运行，
在本地提供 HTTP API、WebUI 与实时事件流，插件以独立进程接入，适合长期挂机、
自动化任务与二次开发。

## 简介

Panko 将机器人拆成两层：C++ 编写的协议核心 `botd` 负责连接、协议编解码、
会话状态与世界数据；Go 编写的控制面 `hub` 负责生命周期、对外 API、WebUI 和
插件宿主。两层通过本地接口通信，任何一侧重启都不会中断另一侧的运行。

围绕这套结构，Panko 提供统一的事件序列、可回放的订阅模型、按能力授权的插件
权限，以及开箱即用的管理界面。开发者可以用任意语言编写插件，通过 JSON-RPC
订阅世界变化、查询方块与实体、执行命令或批量放置方块。

## 特性

- **无界面运行**：启动即服务，所有能力通过 API 与 WebUI 暴露
- **双进程架构**：协议核心与控制面分离，故障隔离，独立重启
- **事件驱动**：全局单调递增的事件序号，支持断线回放与断档提示
- **插件系统**：独立进程、清单声明、按权限授权、崩溃自动重启
- **多语言插件**：插件协议基于 JSON-RPC，任意语言均可接入
- **世界数据层**：方块哈希表、子区块解码、区块存储与坐标查询
- **协议工具链**：二进制缓冲、数据包编解码、单元测试与协议笔记
- **本地优先**：所有端口默认绑定回环地址，调试日志按需开启
- **跨平台构建**：Windows 与 Linux 均通过 CI 验证

## 架构

```
插件进程 (任意语言)
      │  stdio + JSON-RPC
      ▼
   hub (Go)  ── 127.0.0.1:3030  API
      │      ── 127.0.0.1:3031  WebUI
      │      ── 127.0.0.1:3032  调试日志 SSE
      │  TCP + 行分隔 JSON
      ▼
   botd (C++) ── 127.0.0.1:3039  内部接口
```

| 组件 | 语言 | 职责 |
| --- | --- | --- |
| `botd` | C++20 | 连接、协议编解码、会话状态、世界数据 |
| `hub` | Go | 生命周期、API、WebUI、插件宿主、日志 |
| 插件 | 任意 | 业务逻辑，通过 API 订阅事件与下发动作 |

## 端口

| 端口 | 用途 | 开启条件 |
| --- | --- | --- |
| 3030 | API 服务 | 总是 |
| 3031 | WebUI | 总是 |
| 3032 | 调试服务（SSE 日志） | `debug: true` |
| 3039 | botd 内部接口 | 总是 |

## 环境要求

| 组件 | 版本 |
| --- | --- |
| Go | 1.24+ |
| CMake | 3.20+ |
| C++ 编译器 | 支持 C++20（MSVC 2022 / GCC 11+ / Clang 14+） |
| 操作系统 | Windows 10+、Linux x86_64 |

## 快速开始

```bash
# 构建控制面
go build -o hub-bin ./hub/cmd/hub

# 构建协议核心
cmake -S core -B build -DCMAKE_BUILD_TYPE=Release
cmake --build build --config Release

# 准备配置
cp config.example.json config.json
cp core/core.example.json core.json

# 启动
./build/botd --config core.json
./hub-bin -config config.json
```

打开 <http://127.0.0.1:3031> 即可看到运行状态、插件列表与实时事件。
内置的 `echo` 插件会自动启动，在页面输入 `!echo hello` 会收到回复。

## 配置

`config.json` 控制 hub：

```json
{
  "debug": false,
  "api": { "addr": "127.0.0.1:3030" },
  "webui": { "addr": "127.0.0.1:3031" },
  "debug_server": { "addr": "127.0.0.1:3032" },
  "core": { "addr": "127.0.0.1:3039" },
  "web_dir": "web",
  "plugins_dir": "plugins"
}
```

`core.json` 控制 botd：

```json
{
  "core": { "addr": "127.0.0.1:3039" },
  "session": {
    "server_address": "127.0.0.1:19132",
    "player_name": "PankoBot",
    "protocol_version": 860
  }
}
```

相对路径以配置文件所在目录为基准。`debug` 为 `true` 时会额外启动 3032 端口，
日志同时输出到标准输出与 `GET /logs`（Server-Sent Events）。

## API

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/healthz` | 服务与核心状态 |
| GET | `/v1/status` | 机器人状态 |
| POST | `/v1/say` | 发送聊天，请求体 `{"text": "..."}` |
| POST | `/v1/command` | 执行命令，请求体 `{"command": "..."}` |
| GET | `/v1/blocks?x=&y=&z=` | 查询单个方块 |
| POST | `/v1/blocks` | 批量查询，`{"positions": [...]}` |
| GET | `/v1/entities?x=&y=&z=&radius=` | 查询范围内实体 |
| GET | `/v1/events?topics=chat&since=0` | SSE 事件流 |
| GET | `/v1/plugins` | 插件列表 |
| POST | `/v1/plugins/{name}/{start\|stop\|restart}` | 插件控制 |

WebUI 通过 `/api/*` 同源代理访问 API，无需额外配置 CORS。

## 插件

插件是独立进程，通过标准输入输出与 hub 进行 JSON-RPC 通信。每个插件用
`manifest.json` 声明入口、订阅的 topic 与所需权限；hub 在调用侧逐项校验。

```json
{
  "name": "echo",
  "version": "0.1.0",
  "api_version": "v1",
  "entry": ["python3", "plugin.py"],
  "permissions": ["chat.read", "chat.send"],
  "topics": ["chat"],
  "auto_start": true
}
```

完整协议、权限列表与错误码见 [docs/plugin-api.md](docs/plugin-api.md)，
可直接运行的示例见 [plugins/echo](plugins/echo)。

## 目录结构

```
.
├── api/proto/        接口契约（proto）
├── core/             C++ 协议核心
│   ├── src/bot/      会话与世界状态
│   ├── src/common/   二进制缓冲
│   ├── src/control/  本地控制接口
│   ├── src/protocol/ 数据包编解码
│   ├── src/world/    方块表、子区块、区块存储
│   └── tests/        单元测试
├── hub/              Go 控制面
│   ├── cmd/hub/      主程序
│   ├── cmd/mockcore/ 开发用模拟核心
│   └── internal/     事件、核心客户端、插件、API
├── plugins/echo/     示例插件
├── docs/             架构、插件 API、协议笔记
└── web/              WebUI 静态资源
```

## 开发

```bash
# Go
go vet ./...
go test ./...

# C++
cmake --build build --config Release
ctest --test-dir build -C Release --output-on-failure
```

开发调试时可以用 `mockcore` 替代 C++ 核心：

```bash
go run ./hub/cmd/mockcore -addr 127.0.0.1:3039 -chat
```

## Roadmap

- `0.3`：RakNet 传输、登录链、数据包压缩
- `0.4`：区块订阅接口、实体与背包查询
- `0.5`：批量建造任务队列、路径与移动
- `1.0`：插件 API 稳定版、插件仓库与升级机制

## 文档

- [架构与边界](docs/architecture.md)
- [插件 API v1](docs/plugin-api.md)
- [协议笔记](docs/protocol-notes.md)
- [参与开发](CONTRIBUTING.md)
- [安全策略](SECURITY.md)

## 许可证

Panko 以 MIT 许可证发布，详见 [LICENSE](LICENSE)。第三方组件与署名见
[THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md)。

## 免责声明

Panko 与网易、Mojang、Microsoft 均无隶属或合作关系。本项目仅用于学习、研究
与个人自动化场景，请遵守游戏服务条款与所在地法律法规。
