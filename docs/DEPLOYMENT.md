# 部署与运维

## 1. 环境要求

- Linux `amd64` 或 `arm64`。
- 建议 2 核 CPU、2 GB 内存、10 GB 可用磁盘。
- Docker Engine 24+，Docker Compose v2。
- 默认开放 `6366/TCP`；节点实际转发端口按业务配置开放 TCP/UDP。
- 正确的系统时间和可访问 GitHub/GHCR 的网络。

面板服务由四部分组成：MySQL、节点二进制初始化容器、Go 后端和 Nginx 静态前端。

## 2. 安装脚本

```bash
curl -fsSL https://raw.githubusercontent.com/kukumi1/flux-panel-plus/main/panel_install.sh \
  -o /tmp/flux-panel-plus.sh
chmod +x /tmp/flux-panel-plus.sh
sudo /tmp/flux-panel-plus.sh install
```

默认安装目录：`/opt/flux-panel-plus`。

可用环境变量：

| 变量 | 默认值 | 说明 |
|---|---|---|
| `FLUX_INSTALL_DIR` | `/opt/flux-panel-plus` | Compose 与 `.env` 保存目录 |
| `FLUX_BACKUP_DIR` | `<安装目录>/backups` | 备份目录 |
| `PANEL_PORT` | `6366` | 面板端口 |
| `FLUX_INSTALL_DOCKER` | `0` | 设为 `1` 时允许安装 Docker |
| `FLUX_ASSUME_YES` | `0` | 设为 `1` 时跳过确认提示 |
| `FLUX_BRANCH` | `main` | Release 不可用时的源码回退分支 |

## 3. 手动安装

```bash
sudo mkdir -p /opt/flux-panel-plus
cd /opt/flux-panel-plus

sudo curl -fsSLO https://github.com/kukumi1/flux-panel-plus/releases/latest/download/docker-compose.yml
sudo curl -fsSL https://github.com/kukumi1/flux-panel-plus/releases/latest/download/env.example -o .env
sudo chmod 600 .env
sudo nano .env
```

必须修改：

```env
DB_PASSWORD=使用 openssl rand -hex 24 生成
JWT_SECRET=使用 openssl rand -hex 24 生成
```

启动：

```bash
sudo docker compose pull
sudo docker compose up -d
sudo docker compose ps
```

获取初始管理员密码：

```bash
sudo docker logs go-backend 2>&1 | grep -E "password|密码"
```

## 4. 固定版本与回滚

`.env` 中的 `FLUX_VERSION` 控制四个组件使用的镜像版本：

```env
FLUX_VERSION=2.1.28-plus.1
```

使用 `latest` 会跟随最近一次稳定构建。生产环境建议固定 Release 版本。

回滚：

```bash
cd /opt/flux-panel-plus
sudo sed -i 's/^FLUX_VERSION=.*/FLUX_VERSION=旧版本号/' .env
sudo docker compose pull
sudo docker compose up -d
```

如果数据库结构发生不可逆变化，应同时恢复升级前数据库备份。

## 5. 更新

推荐：

```bash
sudo /usr/local/bin/flux-panel-plus update
```

更新流程会先导出 MySQL 和配置，再下载最新 Compose、拉取镜像并等待健康检查。失败时恢复旧 Compose 文件。

手动更新：

```bash
cd /opt/flux-panel-plus
sudo docker compose pull
sudo docker compose up -d --remove-orphans
sudo docker compose ps
```

## 6. 备份与恢复

备份：

```bash
sudo /usr/local/bin/flux-panel-plus backup
```

归档包含：

- `.env`
- `docker-compose.yml`
- `database.sql`

备份包含数据库密码，文件权限默认设为 `0600`。不要上传到公开网盘或 Git 仓库。

恢复示例：

```bash
sudo docker compose down
sudo tar -xzf flux-panel-plus-YYYYMMDD_HHMMSS.tar.gz -C /tmp
sudo cp /tmp/flux-panel-plus-*/.env /opt/flux-panel-plus/.env
sudo cp /tmp/flux-panel-plus-*/docker-compose.yml /opt/flux-panel-plus/docker-compose.yml

cd /opt/flux-panel-plus
sudo docker compose up -d mysql
# 等待 MySQL healthy 后：
source .env
sudo docker exec -i -e MYSQL_PWD="$DB_PASSWORD" flux-mysql \
  mysql -u"$DB_USER" "$DB_NAME" < /tmp/flux-panel-plus-*/database.sql
sudo docker compose up -d
```

## 7. HTTPS 反向代理

推荐只把前端端口暴露给反向代理。Nginx 示例：

```nginx
server {
    listen 443 ssl http2;
    server_name panel.example.com;

    ssl_certificate     /etc/letsencrypt/live/panel.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/panel.example.com/privkey.pem;

    location / {
        proxy_pass http://127.0.0.1:6366;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
    }
}
```

随后在 `.env` 中限制监听地址：

```env
PANEL_LISTEN=127.0.0.1
ALLOWED_ORIGINS=https://panel.example.com
```

## 8. IPv6

只有在 Docker daemon 已启用 IPv6 后才能设置：

```env
PANEL_LISTEN=[::]
ENABLE_IPV6=true
```

Docker 示例配置：

```json
{
  "ipv6": true,
  "fixed-cidr-v6": "fd00::/80"
}
```

修改 `/etc/docker/daemon.json` 后重启 Docker，再重新创建 Compose 网络。

## 9. 从源码构建

```bash
git clone https://github.com/kukumi1/flux-panel-plus.git
cd flux-panel-plus
cp .env.example .env
nano .env

docker compose -f docker-compose.yml -f docker-compose.build.yml up -d --build
```

这会从当前源码构建 backend、frontend 和 node-binary 镜像。节点镜像可以单独构建：

```bash
docker build --build-arg VERSION=dev -t flux-panel-plus/node:dev ./go-node
```

## 10. 排障

```bash
cd /opt/flux-panel-plus
docker compose ps
docker compose logs --tail=200 backend
docker compose logs --tail=200 frontend
docker compose logs --tail=200 mysql
docker inspect -f '{{.State.Health.Status}}' go-backend
curl -I http://127.0.0.1:6366
```

常见问题：

- `unauthorized`：GHCR 镜像尚未公开或网络代理修改了认证请求。
- 后端不健康：检查 MySQL 是否 healthy、`.env` 是否完整、磁盘是否已满。
- 节点离线：检查面板地址、节点 Secret、系统时间、防火墙和 WebSocket 反向代理。
- Telegram 慢：检查 VPS 到 `api.telegram.org` 的网络与 DNS。
