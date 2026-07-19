# Security Policy

## Supported versions

仅最新 GitHub Release 接收安全修复。生产部署建议固定到最新稳定版本，并在升级前备份数据库。

## Reporting a vulnerability

请使用 GitHub 的私密安全报告功能：

<https://github.com/kukumi1/flux-panel-plus/security/advisories/new>

不要在公开 Issue 中提交以下内容：

- Telegram Bot Token
- 节点 Secret
- 数据库密码或数据库备份
- Cloudflare/API Token
- JWT Secret
- SSH 私钥
- 可直接访问的生产面板地址与管理员凭据

报告中请提供受影响版本、复现步骤、影响范围和建议修复方式。收到报告后会先确认问题，再协调修复和公开时间。

## Deployment guidance

- 使用 HTTPS 和强随机 Secret。
- `.env` 权限设置为 `0600`。
- 不公开 MySQL 与后端 `6365` 端口。
- 定期检查管理员账号、Telegram 绑定和节点权限。
- 更新前创建离线备份，并测试恢复流程。
- 仅从本仓库 Release 或 GHCR 拉取镜像，并核对 Release 的 `SHA256SUMS`。
