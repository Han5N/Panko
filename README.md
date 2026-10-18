# Panko

网易版 Minecraft 基岩版的单机器人框架。无桌面界面，启动后在本地提供 HTTP API、
WebUI 和可选的调试日志流；插件以独立进程运行，崩溃不会影响机器人本体。

> Panko 与网易、Mojang、Microsoft 均无隶属关系。仅用于学习、研究与个人自动化，
> 使用者需自行遵守游戏服务条款。项目不附带任何账号、认证凭据。

## 当前阶段：Beta

已经可用：

- hub 控制面：API、WebUI、SSE 事件流、调试日志、插件宿主与崩溃重启
- 插件 API v1：清单、权限校验、JSON-RPC、事件订阅与回放
- 示例插件 `plugins/echo`，端到端跑通
- botd 底座：帧与包编解码、会话状态、事件序列、本地控制接口
- 单元测试：Go 与 C++ 两侧共 30+ 用例

尚未接入：

- **真实 RakNet 连接与网易登录链**。botd 当前运行在内置模拟会话下，可以完整
  驱动 API 与插件，但不会连接游戏服务器。
- 数据包压缩（NetworkSettings 之后的 snappy）与区块解析。
- NetherNet / WebSocket / WebRTC 传输。

这些是下一个里程碑的内容，接口和事件格式不会因此改变。

## 架构

```
插件进程 (任意语言)
      │  stdio + JSON-RPC
      ▼
   hub (Go)  ── 127.0.0.1:3030  API
      │      ── 127.0.0.1:3031  WebUI
      │      ── 127.0.0.1:3032  调试日志 SSE（仅 DEBUG）
      │  TCP + 行分隔 JSON
      ▼
   botd (C++) ── 127.0.0.1:3039  内部接口，不对外暴露
```

- `botd` 负责连接、协议、世界状态和事件序列，单进程单机器人。
- `hub` 负责生命周期、对外 API、插件宿主和 WebUI，是唯一用户入口。
- 插件永远在独立进程中，通过清单声明权限。

## 端口

| 端口 | 用途 | 默认监听 | 开启条件 |
| --- | --- | --- | --- |
| 3030 | API 服务 | 127.0.0.1 | 总是 |
| 3031 | WebUI | 127.0.0.1 | 总是 |
| 3032 | 调试服务（SSE 日志） | 127.0.0.1 | `debug: true` |
| 3039 | botd 内部接口 | 127.0.0.1 | 总是 |

全部只监听回环地址。机器人会持有账号令牌，不要把端口暴露到公网。

## 快速开始

需要 Go 1.24+ 与 CMake 3.20+（C++20 编译器）。

```bash
# 1. 构建 hub
go build -o hub-bin ./hub/cmd/hub

# 2. 构建 botd
cmake -S core -B build -DCMAKE_BUILD_TYPE=Release
cmake --build build --config Release

# 3. 准备配置
cp config.example.json config.json
cp core/core.example.json core.json

# 4. 启动（两个终端）
./build/botd --config core.json
./hub-bin -config config.json
```

打开 <http://127.0.0.1:3031> 可以看到状态、插件列表和实时事件。
示例插件会自动启动，在输入框发送 `!echo hello` 会收到 `hello`。

## 配置

`config.json`（hub）：

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

`core.json`（botd）：

```json
{
  "core": { "addr": "127.0.0.1:3039" },
  "session": {
    "server_address": "sim.local:19132",
    "player_name": "PankoBot",
    "protocol_version": 860
  }
}
```

相对路径以配置文件所在目录为基准。

## API

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/healthz` | 服务与核心连接状态 |
| GET | `/v1/status` | 机器人状态 |
| POST | `/v1/say` | `{"text": "..."}` |
| POST | `/v1/command` | `{"command": "..."}` |
| GET | `/v1/blocks?x=&y=&z=` | 查询单个方块 |
| POST | `/v1/blocks` | `{"positions": [{"x":0,"y":0,"z":0}]}` |
| GET | `/v1/entities?x=&y=&z=&radius=` | 查询实体 |
| GET | `/v1/events?topics=chat&since=0` | SSE 事件流 |
| GET | `/v1/plugins` | 插件列表 |
| POST | `/v1/plugins/{name}/{start\|stop\|restart}` | 插件控制 |

WebUI 通过 `/api/*` 同源代理访问 API，因此不需要 CORS 配置。

## 插件

插件协议、权限和完整方法列表见 [docs/plugin-api.md](docs/plugin-api.md)。
最小示例见 [plugins/echo](plugins/echo)。

## 开发

```bash
go vet ./... && go test ./...
cmake --build build --config Release && ctest --test-dir build -C Release --output-on-failure
```

开发时也可以用 `hub/cmd/mockcore` 替代 botd，它不依赖 C++ 构建：

```bash
go run ./hub/cmd/mockcore -addr 127.0.0.1:3039 -chat
```

## 文档

- [架构与边界](docs/architecture.md)
- [插件 API v1](docs/plugin-api.md)
- [协议笔记](docs/protocol-notes.md)
- [参与开发](CONTRIBUTING.md)
- [安全策略](SECURITY.md)

## 许可证

MIT，见 [LICENSE](LICENSE)。第三方组件见 [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md)。
