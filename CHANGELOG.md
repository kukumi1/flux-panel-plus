# Changelog

## 2.1.28-plus.1

首个 Flux Panel Plus 公开版本，基于上游 Flux Panel 2.1.28。

### Added

- 多跳转发路由与逐跳 reconcile。
- 跨节点连接流量审计及 x-ui/sing-box 审计采集。
- 节点二进制自更新与审计健康监控。
- HA 节点组、DDNS 和故障转移。
- Telegram 隧道、转发、节点监控和流量审计控制台。
- Telegram 隧道与转发新增向导。
- 22 个 Telegram Bot 命令与命令菜单。
- Telegram HTTP Keep-Alive、后台诊断、异步 Callback 和 Worker 队列。

### Operations

- GHCR `amd64` / `arm64` 多架构镜像。
- GitHub Actions CI、镜像发布和 Release 工作流。
- 一键安装、更新、备份、状态、日志与卸载脚本。
- Docker Compose 源码构建覆盖文件。

### Security

- 公开版本使用干净 Git 历史，不包含生产运维记录、节点 Secret、真实服务器地址或数据库备份。
- Release 附带 SHA-256 校验表。
