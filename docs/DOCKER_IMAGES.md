# 镜像使用说明

NexusAi 的 Docker 镜像托管在 GitHub Container Registry (ghcr.io)。

## 镜像地址

| 镜像 | 地址 |
|------|------|
| 后端 | `ghcr.io/nanping5/nexusai` |
| 前端 | `ghcr.io/nanping5/nexusai-nginx` |

## 拉取镜像

```bash
# 登录 GitHub Container Registry（公开镜像可跳过）
echo $GITHUB_TOKEN | docker login ghcr.io -u USERNAME --password-stdin

# 拉取最新版本
docker pull ghcr.io/nanping5/nexusai:latest
docker pull ghcr.io/nanping5/nexusai-nginx:latest

# 拉取指定版本
docker pull ghcr.io/nanping5/nexusai:v1.0.0
docker pull ghcr.io/nanping5/nexusai-nginx:v1.0.0
```

## 使用 docker-compose 部署

```bash
# 设置环境变量
export DOCKER_REGISTRY=ghcr.io
export DOCKER_NAMESPACE=nanping5
export IMAGE_TAG=latest

# 拉取并启动
docker-compose -f docker-compose.full.yml pull
docker-compose -f docker-compose.full.yml up -d
```

## 可用标签

- `latest` - 最新稳定版
- `main` - 主分支最新构建
- `v*.*.*` - 版本发布标签
- `sha-*` - Git commit SHA
