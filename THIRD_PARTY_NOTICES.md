# 第三方组件清单

本文件记录 Panko 分发物中包含的第三方代码。新增或升级依赖时必须同步更新此表，
否则 CI 的许可检查会失败。

## 直接依赖

| 组件 | 版本 | 许可证 | 是否修改 | 链接方式 | 备注 |
| --- | --- | --- | --- | --- | --- |
| NTvector | 协议版本 860 | MIT | 是（移植） | 源码 | 协议层与登录链来源；方块哈希表已作为静态数据引入 |
| OpenSSL | 3.5.0 | Apache-2.0 | 否 | 静态 | 需保留 LICENSE 与 NOTICE |
| libwebsockets | 4.5 | MIT（含少数 BSD-3/BSD-2/zlib/OFL 文件） | 否 | 静态 | 被保留的例外文件见其 LICENSE |
| libdatachannel | 随仓库快照 | MPL-2.0 | 否 | 静态 | MPL 为文件级 copyleft，修改过的文件必须公开 |
| SLikeNet | 随仓库快照 | MIT | 待确认 | 静态 | 源自 RakNet，原始版权声明需保留 |
| nlohmann/json | 3.11.3 | MIT | 否 | 头文件 | core 使用 |
| jsoncpp | 1.9.6 | MIT | 否 | 静态 | 仅在移植 NTvector 代码需要时引入 |
| zlib | 1.3.1 | zlib | 否 | 静态 | |

## 运行时依赖

| 组件 | 版本 | 许可证 | 备注 |
| --- | --- | --- | --- |
| Go 标准库 | 随构建工具链 | BSD-3-Clause | hub 使用，随二进制分发时需保留版权声明 |
| Python | 3.12.2 | PSF-2.0 | 仅当启用 Python 插件宿主时随包分发 |

## 分发注意事项

- **MPL-2.0（libdatachannel）**：以源码形式分发时无需额外动作；若修改了其中任何
  文件，修改后的文件必须以 MPL-2.0 公开。未修改的文件不受影响。
- **Apache-2.0（OpenSSL）**：分发二进制时需附带许可证文本，并保留其 NOTICE 内容。
- **MIT / BSD / zlib**：保留版权声明与许可证文本即可。
- 本项目自身使用 MIT，与上述许可证兼容；不要把 GPL 系代码引入核心仓库。

## 更新流程

1. 修改 `core/third_party/` 或 `hub/go.mod` 中的依赖版本。
2. 运行 `tools/check_licenses`（待实现）核对许可证是否仍在允许列表内。
3. 同步更新本表格中的版本与备注。
