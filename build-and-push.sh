#!/bin/bash

# ================================================
# NexusAi 本地构建并推送到镜像仓库
# 支持：阿里云、Docker Hub、私有仓库
# ================================================

set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

echo -e "${BLUE}╔════════════════════════════════════════════╗${NC}"
echo -e "${BLUE}║     NexusAi 镜像构建与推送脚本            ║${NC}"
echo -e "${BLUE}╚════════════════════════════════════════════╝${NC}"
echo ""

# ============ 配置变量 ============
# 镜像仓库地址（可修改为你的仓库）
# GitHub: ghcr.io/用户名/镜像名
# 阿里云: registry.cn-hangzhou.aliyuncs.com/命名空间/镜像名
DOCKER_REGISTRY="${DOCKER_REGISTRY:-ghcr.io}"
DOCKER_NAMESPACE="${DOCKER_NAMESPACE:-nanping5}"
IMAGE_TAG="${IMAGE_TAG:-latest}"

# 镜像名称
BACKEND_IMAGE="${DOCKER_REGISTRY}/${DOCKER_NAMESPACE}/nexusai"
NGINX_IMAGE="${DOCKER_REGISTRY}/${DOCKER_NAMESPACE}/nexusai-nginx"

echo -e "${GREEN}镜像配置:${NC}"
echo "  仓库地址: ${DOCKER_REGISTRY}"
echo "  命名空间: ${DOCKER_NAMESPACE}"
echo "  标签:     ${IMAGE_TAG}"
echo ""

# ============ 1. 检查 Docker ============
echo -e "${GREEN}[1/5] 检查 Docker 环境...${NC}"
if ! command -v docker &> /dev/null; then
    echo -e "${RED}错误: 未安装 Docker，请先安装 Docker${NC}"
    exit 1
fi
echo -e "${GREEN}Docker 版本: $(docker --version)${NC}"

# ============ 2. 构建前端 ============
echo -e "${GREEN}[2/5] 构建前端...${NC}"

cd frontend

# 检查 Node.js
if ! command -v node &> /dev/null; then
    echo -e "${RED}错误: 未安装 Node.js${NC}"
    exit 1
fi

echo "Node 版本: $(node --version)"
echo "npm 版本:  $(npm --version)"

# 安装依赖
if [ ! -d "node_modules" ]; then
    echo "安装依赖..."
    npm install
fi

# 构建生产版本
echo "构建前端..."
npm run build

# 构建 Nginx 镜像
echo "构建 Nginx 镜像..."
docker build -t "${NGINX_IMAGE}:${IMAGE_TAG}" .

cd ..

# ============ 3. 构建后端 ============
echo -e "${GREEN}[3/5] 构建后端...${NC}"

# 检查 Go
if ! command -v go &> /dev/null; then
    echo -e "${RED}错误: 未安装 Go${NC}"
    exit 1
fi

echo "Go 版本: $(go version)"

# 构建后端镜像
docker build -t "${BACKEND_IMAGE}:${IMAGE_TAG}" .

# ============ 4. 推送镜像 ============
echo -e "${GREEN}[4/5] 推送镜像到仓库...${NC}"

# 检查是否已登录
echo -e "${YELLOW}提示: 如果未登录镜像仓库，请先执行 docker login${NC}"
echo ""

read -p "是否已登录镜像仓库? (y/n): " logged_in
if [ "$logged_in" != "y" ]; then
    echo "请先登录镜像仓库:"
    echo "  docker login ${DOCKER_REGISTRY}"
    exit 1
fi

# 推送后端镜像
echo "推送后端镜像..."
docker push "${BACKEND_IMAGE}:${IMAGE_TAG}"

# 推送 Nginx 镜像
echo "推送 Nginx 镜像..."
docker push "${NGINX_IMAGE}:${IMAGE_TAG}"

# ============ 5. 输出部署信息 ============
echo -e "${GREEN}[5/5] 构建完成！${NC}"
echo ""
echo -e "${BLUE}╔════════════════════════════════════════════╗${NC}"
echo -e "${BLUE}║              部署信息                      ║${NC}"
echo -e "${BLUE}╚════════════════════════════════════════════╝${NC}"
echo ""
echo -e "${GREEN}镜像已推送:${NC}"
echo "  后端: ${BACKEND_IMAGE}:${IMAGE_TAG}"
echo "  前端: ${NGINX_IMAGE}:${IMAGE_TAG}"
echo ""
echo -e "${GREEN}在服务器上执行:${NC}"
echo ""
echo "  # 1. 登录镜像仓库"
echo "  docker login ${DOCKER_REGISTRY}"
echo ""
echo "  # 2. 设置环境变量"
echo "  export DOCKER_REGISTRY=${DOCKER_REGISTRY}"
echo "  export DOCKER_NAMESPACE=${DOCKER_NAMESPACE}"
echo "  export IMAGE_TAG=${IMAGE_TAG}"
echo ""
echo "  # 3. 拉取镜像"
echo "  docker-compose -f docker-compose.full.yml pull"
echo ""
echo "  # 4. 启动服务"
echo "  docker-compose -f docker-compose.full.yml up -d"
echo ""
echo -e "${YELLOW}提示: 服务器上需要配置好 config.toml 和 .env 文件${NC}"
echo ""
