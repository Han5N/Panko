# 插件 API v1

插件是独立进程，通过标准输入输出与 hub 通信，一行一个 JSON-RPC 2.0 消息。
stdout 只能写协议消息，日志一律写 stderr。

## 目录结构

```
plugins/
└── my-plugin/
    ├── manifest.json
    └── plugin.py
```

## manifest.json

```json
{
  "name": "echo",
  "version": "0.1.0",
  "api_version": "v1",
  "entry": ["python3", "plugin.py"],
  "description": "聊天回显",
  "author": "you",
  "permissions": ["chat.read", "chat.send"],
  "topics": ["chat"],
  "auto_start": true
}
```

| 字段 | 说明 |
| --- | --- |
| `name` | 只能包含字母、数字、`-`、`_`，也是 API 里的插件标识 |
| `version` | 插件自身版本 |
| `api_version` | 当前只支持 `v1` |
| `entry` | 启动命令，工作目录是插件目录 |
| `permissions` | 权限列表，缺省即无权限 |
| `topics` | 初始订阅的事件 topic，空表示订阅所有已授权 topic |
| `auto_start` | hub 启动时是否自动拉起 |

Windows 上如果没有 `python3`，把 `entry` 改成 `["python", "plugin.py"]`。

## 权限

| 权限 | 允许的操作 |
| --- | --- |
| `chat.read` | 订阅 `chat` 事件 |
| `chat.send` | 调用 `panko.say` |
| `command` | 调用 `panko.command` |
| `world.read` | 查询方块与实体，订阅 `block` / `player` 事件 |
| `world.write` | 调用 `panko.place_blocks` |
| `raw.packet` | 原始数据包能力，默认不授予 |

权限在 hub 侧校验。插件未声明的权限一律拒绝，不会因为方法名看起来安全就放行。

## hub → 插件

### panko.init

进程启动后发送一次，插件应在此完成订阅。

```json
{
  "jsonrpc": "2.0",
  "method": "panko.init",
  "params": {
    "api_version": "v1",
    "plugin": "echo",
    "permissions": ["chat.read", "chat.send"],
    "topics": ["chat"]
  }
}
```

### panko.event

```json
{
  "jsonrpc": "2.0",
  "method": "panko.event",
  "params": {
    "seq": 42,
    "topic": "chat",
    "time": "2026-10-19T04:49:13.595Z",
    "payload": { "sender": "PankoBot", "message": "hello" }
  }
}
```

事件按 `seq` 单调递增。插件队列满时事件会被丢弃，重连后可用 `panko.subscribe`
带 `since` 回放（回放范围受核心缓冲区限制）。

### panko.shutdown

hub 退出或停止插件时发送。插件应在 3 秒内自行退出。

## 插件 → hub

请求都必须带 `id`。hub 会原样回传，响应格式为标准 JSON-RPC。

| 方法 | 参数 | 权限 | 说明 |
| --- | --- | --- | --- |
| `panko.ping` | 无 | 无 | 返回当前时间 |
| `panko.status` | 无 | 无 | 返回核心连接状态 |
| `panko.log` | `{level, message}` | 无 | 写 hub 日志（仍建议日志走 stderr） |
| `panko.subscribe` | `{topics: []}` | 按 topic | 追加订阅 |
| `panko.say` | `{text}` | `chat.send` | 以机器人身份发言 |
| `panko.command` | `{command}` | `command` | 执行命令 |
| `panko.block.get` | `{x, y, z}` | `world.read` | 查询单个方块 |
| `panko.blocks.get` | `{positions: [{x,y,z}]}` | `world.read` | 批量查询，最多 4096 |
| `panko.entities.list` | `{x, y, z, radius}` | `world.read` | 查询半径内实体 |
| `panko.place_blocks` | `{blocks: [{position, block}]}` | `world.write` | 批量放置 |

### 示例请求

```json
{"jsonrpc":"2.0","id":1,"method":"panko.say","params":{"text":"hello"}}
```

```json
{"jsonrpc":"2.0","id":1,"result":{"ok":true}}
```

```json
{"jsonrpc":"2.0","id":1,"error":{"code":-32001,"message":"缺少权限 chat.send"}}
```

错误码：`-32001` 权限不足，`-32002` 核心调用失败，`-32601` 未知方法，
`-32602` 参数错误。

## 最小插件

```python
import json, sys

def send(msg):
    sys.stdout.write(json.dumps(msg) + "\n")
    sys.stdout.flush()

for line in sys.stdin:
    msg = json.loads(line)
    if msg.get("method") == "panko.init":
        send({"jsonrpc": "2.0", "id": 1, "method": "panko.subscribe", "params": {"topics": ["chat"]}})
    elif msg.get("method") == "panko.event":
        ev = msg["params"]
        if ev["topic"] == "chat" and ev["payload"]["message"] == "!ping":
            send({"jsonrpc": "2.0", "id": 2, "method": "panko.say", "params": {"text": "pong"}})
    elif msg.get("method") == "panko.shutdown":
        break
```

完整示例见 `plugins/echo/`。

## 版本策略

`api_version` 只在发生不兼容变更时递增。新增方法、新增权限属于向后兼容变更，
不会改变 `api_version`。v1 的稳定接口在 1.x 系列内不会移除；弃用会提前一个
小版本在文档和日志中标注。
