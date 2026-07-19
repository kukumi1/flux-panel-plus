# 开发与发布

## 环境

- Go 1.25.x
- Node.js 22
- npm
- Docker Buildx（构建多架构镜像时）

## 后端

```bash
cd go-backend
go test ./...
go vet ./...
go build -trimpath -buildvcs=false ./...
```

Linux 交叉编译：

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
  go build -trimpath -buildvcs=false \
  -ldflags="-s -w -X flux-panel/go-backend/pkg.Version=dev" \
  -o app-amd64 .
```

## 节点

```bash
cd go-node
go test ./...
go vet ./...
```

## x-ui 审计采集器

```bash
cd xui-audit-shipper
go test ./...
go vet ./...
```

## 前端

```bash
cd nextjs-frontend
npm ci
npm run dev
npm run build
```

静态导出目录为 `nextjs-frontend/out`。

## Docker

```bash
cp .env.example .env
docker compose -f docker-compose.yml -f docker-compose.build.yml up -d --build
```

单独构建：

```bash
docker build --build-arg VERSION=dev -t flux-panel-plus/backend:dev ./go-backend
docker build -t flux-panel-plus/frontend:dev ./nextjs-frontend
docker build --build-arg VERSION=dev -t flux-panel-plus/node:dev ./go-node
docker build -f go-node/Dockerfile.binary --build-arg VERSION=dev \
  -t flux-panel-plus/node-binary:dev ./go-node
```

## 分支与上游同步

推荐远程配置：

```bash
git remote -v
git remote add upstream https://github.com/0xNetuser/flux-panel.git
git fetch upstream --tags
```

同步上游时不要直接覆盖 Plus 功能。建议创建独立分支，先审查数据库模型、安装脚本、Docker Compose、节点协议和 Telegram 文件的冲突，再合并到 `main`。

## 版本发布

版本同时保存在：

- `VERSION`
- `nextjs-frontend/package.json`
- `nextjs-frontend/package-lock.json`

发布步骤：

```bash
# 更新版本文件和 CHANGELOG.md
git add .
git commit -m "chore: release 2.1.28-plus.2"
git tag v2.1.28-plus.2
git push origin main v2.1.28-plus.2
```

`publish.yml` 会：

1. 构建并发布 backend、frontend、node 和 node-binary 多架构 GHCR 镜像。
2. 创建 GitHub Release。
3. 上传 Linux amd64/arm64 二进制、安装脚本、Compose 文件和 SHA-256 校验表。

`main` 分支每次推送会更新 `latest` 与 `sha-xxxxxxx` 镜像标签；正式版本标签同时发布固定版本镜像。

## 提交前检查

```bash
bash -n panel_install.sh install.sh
git diff --check

cd go-backend && go test ./... && go vet ./...
cd ../go-node && go test ./... && go vet ./...
cd ../xui-audit-shipper && go test ./... && go vet ./...
cd ../nextjs-frontend && npm ci && npm run build
```

严禁提交 `.env`、数据库备份、Telegram Token、节点 Secret、Cloudflare Token、SSH 私钥或生产日志。
