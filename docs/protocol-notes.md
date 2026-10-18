# 协议笔记

记录已经确认的协议细节，避免重复踩坑。字段布局以 NTvector 的实现为参考
（MIT，版权归原作者），并在 `core/src/protocol/` 中独立实现与测试。

## 帧格式

```
varuint 长度 | varuint 包 ID | 包体
```

长度只计算“包 ID + 包体”，不含长度字段自身。

**坑**：包 ID 是 varuint。ID 193（RequestNetworkSettings）编码为 `C1 01`，
不是单字节 `C1`。NTvector 里写成 `WriteUInt8(ID())` 再加一个 `1` 字段，恰好
拼成了正确的 varuint；实现时不要照抄成单字节。

## 已使用的包 ID

| ID | 包 | 方向 |
| --- | --- | --- |
| 1 | Login | C → S |
| 2 | PlayStatus | S → C |
| 3 | ServerToClientHandshake | S → C |
| 4 | ClientToServerHandshake | C → S |
| 5 | Disconnect | S → C |
| 6 | ResourcePacksInfo | S → C |
| 7 | ResourcePackStack | S → C |
| 8 | ResourcePackClientResponse | C → S |
| 9 | Text | 双向 |
| 10 | SetTime | S → C |
| 11 | StartGame | S → C |
| 19 | MovePlayer | 双向 |
| 58 | LevelChunk | S → C |
| 63 | PlayerList | S → C |
| 69 | RequestChunkRadius | C → S |
| 70 | ChunkRadiusUpdated | S → C |
| 77 | CommandRequest | C → S |
| 79 | CommandOutput | S → C |
| 143 | NetworkSettings | S → C |
| 144 | PlayerAuthInput | C → S |
| 174 | SubChunk | S → C |
| 193 | RequestNetworkSettings | C → S |

协议版本 860 对应 1.21.120（网易版）。版本变化时先核对这张表，再核对字段。

## Login

```
u32 大端  协议版本
varuint   负载长度
  u32 小端  chain 长度
  bytes     chain
  u32 小端  skin 长度
  bytes     skin
```

注意协议版本号是大端，而内部长度是小端。

## RequestNetworkSettings

```
u32 大端  协议版本
```

同样是大端。PlayStatus 的状态码也是大端 u32，这两个包容易写反。

## NetworkSettings

```
u16 小端  压缩阈值
u16 小端  压缩算法（1 = snappy）
i8        客户端限流
i8        限流阈值
f32 小端  限流系数
```

阈值生效后，超过阈值的包需要压缩。当前版本尚未实现压缩，因此只在登录早期
解析该包，不发送大包。

## Text

聊天（type = 1）：

```
u8     类型
u8     是否需要翻译
string 发送者
string 内容
u32    发送者 xuid
```

type 2 / 8 / 9 的字段组合不同，解析时按类型分支处理，不要假定统一布局。

## CommandRequest

```
string 命令
varint 来源类型（zigzag）
16 字节 UUID
string 请求 ID
[来源为 DevConsole 或 Test 时] varint64 玩家 ID
bool   内部来源
varint 命令版本
bool   unlimit
```

命令版本随游戏更新变化，作为配置项而不是硬编码。

## CommandBlockUpdate

如果后续要做命令块，必须注意 1.21.120 的 `filtered_name` 字段：它位于
`name` 之后、`should_track_output` 之前。缺失会导致服务端解析越界并踢人。

## 压缩

`NetworkSettings.compression_algorithm == 1` 表示 snappy。阈值通常为 512，
超过阈值的包体需要先解压/压缩。未实现压缩时只能处理登录早期的小包。

## 尚未覆盖

- 区块与子区块的完整解析（palette、heightmap、biome）
- 实体元数据与属性
- 网易私有的 Rotor / RC4 与反作弊 RPC 握手
- NetherNet 信令

这些属于下一个里程碑。实现时应先补单元测试，再接入真实连接。
