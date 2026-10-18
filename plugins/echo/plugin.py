#!/usr/bin/env python3
"""Panko 示例插件：聊天回显。

插件通过标准输入输出与 hub 通信，一行一个 JSON-RPC 消息。
所有日志写 stderr，stdout 只用于协议消息。
"""

import json
import sys

_next_id = 0


def send(message):
    sys.stdout.write(json.dumps(message, ensure_ascii=False) + "\n")
    sys.stdout.flush()


def request(method, params):
    global _next_id
    _next_id += 1
    send({"jsonrpc": "2.0", "id": _next_id, "method": method, "params": params})


def log(text):
    print(text, file=sys.stderr, flush=True)


def main():
    for line in sys.stdin:
        line = line.strip()
        if not line:
            continue
        try:
            msg = json.loads(line)
        except json.JSONDecodeError:
            log("收到非 JSON 数据，已忽略")
            continue

        method = msg.get("method")
        if method == "panko.init":
            log("初始化完成，订阅聊天事件")
            request("panko.subscribe", {"topics": ["chat"]})
        elif method == "panko.event":
            handle_event(msg.get("params") or {})
        elif method == "panko.shutdown":
            log("收到关闭指令")
            return


def handle_event(event):
    if event.get("topic") != "chat":
        return

    payload = event.get("payload") or {}
    message = (payload.get("message") or "").strip()
    sender = payload.get("sender") or "?"

    if message.startswith("!echo "):
        request("panko.say", {"text": message[len("!echo "):]})
        log(f"回复 {sender}: {message}")
    elif message == "!ping":
        request("panko.say", {"text": "pong"})


if __name__ == "__main__":
    main()
