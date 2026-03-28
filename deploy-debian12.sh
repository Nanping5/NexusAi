#!/bin/bash

# ================================================
# NexusAi Debian 12 服务器部署脚本
# 仅拉取镜像，不做构建
# ================================================

set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

echo -e "${BLUE}╔════════════════════════════════════════════╗${NC}"
echo -e "${BLUE}║     NexusAi Debian 12 服务器部署           ║${NC}"
echo -e "${BLUE}╚════════════════════════════════════════════╝${NC}"
echo ""

# ============ 配置变量 ============
DOCKER_REGISTRY="${DOCKER_REGISTRY:-ghcr.io}"
DOCKER_NAMESPACE="${DOCKER_NAMESPACE:-nanping5}"
IMAGE_TAG="${IMAGE_TAG:-latest}"

# ============ 1. 系统优化 ============
echo -e "${GREEN}[1/6] 系统优化...${NC}"

# 更新系统
echo "更新软件包列表..."
sudo apt-get update -qq

# 安装基础工具
sudo apt-get install -y -qq curl wget gnupg2 ca-certificates lsb-release

# 创建 Swap 分区（2GB）
if [ ! -f /swapfile ]; then
    echo "创建 2GB Swap 分区..."
    sudo fallocate -l 2G /swapfile
    sudo chmod 600 /swapfile
    sudo mkswap /swapfile
    sudo swapon /swapfile
    echo '/swapfile none swap sw 0 0' | sudo tee -a /etc/fstab
    echo "vm.swappiness=10" | sudo tee -a /etc/sysctl.conf
    echo "vm.vfs_cache_pressure=50" | sudo tee -a /etc/sysctl.conf
    echo -e "${GREEN}Swap 创建完成${NC}"
else
    echo -e "${YELLOW}Swap 已存在，跳过${NC}"
fi

# 应用 sysctl 配置
sudo sysctl -p 2>/dev/null || true

# 设置文件描述符限制
cat << EOF | sudo tee /etc/security/limits.d/nexusai.conf > /dev/null
* soft nofile 65535
* hard nofile 65535
root soft nofile 65535
root hard nofile 65535
EOF

echo -e "${GREEN}系统优化完成${NC}"

# ============ 2. 安装 Docker ============
echo -e "${GREEN}[2/6] 安装 Docker...${NC}"

if command -v docker &> /dev/null; then
    echo -e "${YELLOW}Docker 已安装: $(docker --version)${NC}"
else
    echo "安装 Docker..."
    # Debian 12 官方安装方式
    sudo apt-get install -y -qq docker.io docker-compose

    # 启动并设置开机启动
    sudo systemctl enable docker
    sudo systemctl start docker

    # 将当前用户加入 docker 组
    sudo usermod -aG docker $USER

    echo -e "${GREEN}Docker 安装完成${NC}"
fi

# 验证 Docker
sudo docker --version
sudo docker-compose --version 2>/dev/null || sudo docker compose version

# ============ 3. Docker 优化配置 ============
echo -e "${GREEN}[3/6] Docker 优化配置...${NC}"

sudo mkdir -p /etc/docker
cat << EOF | sudo tee /etc/docker/daemon.json > /dev/null
{
    "log-driver": "json-file",
    "log-opts": {
        "max-size": "10m",
        "max-file": "3"
    },
    "storage-driver": "overlay2",
    "live-restore": true,
    "default-ulimits": {
        "nofile": {
            "Name": "nofile",
            "Hard": 65535,
            "Soft": 65535
        }
    }
}
EOF

sudo systemctl restart docker
echo -e "${GREEN}Docker 配置完成${NC}"

# ============ 4. 登录镜像仓库 ============
echo -e "${GREEN}[4/6] 登录镜像仓库...${NC}"

echo -e "${YELLOW}请输入镜像仓库登录信息:${NC}"
echo "仓库地址: ${DOCKER_REGISTRY}"
read -p "用户名: " REGISTRY_USER
read -s -p "密码: " REGISTRY_PASS
echo ""

echo "登录中..."
echo "${REGISTRY_PASS}" | sudo docker login "${DOCKER_REGISTRY}" -u "${REGISTRY_USER}" --password-stdin

echo -e "${GREEN}登录成功${NC}"

# ============ 5. 拉取镜像 ============
echo -e "${GREEN}[5/6] 拉取镜像...${NC}"

export DOCKER_REGISTRY
export DOCKER_NAMESPACE
export IMAGE_TAG

# 拉取所有镜像
sudo docker-compose -f docker-compose.full.yml pull

echo -e "${GREEN}镜像拉取完成${NC}"

# ============ 6. 配置文件检查 ============
echo -e "${GREEN}[6/6] 检查配置文件...${NC}"

if [ ! -f config.toml ]; then
    echo -e "${RED}错误: 未找到 config.toml${NC}"
    echo "请确保已上传配置文件到当前目录"
    exit 1
fi

if [ ! -f .env ]; then
    echo -e "${RED}错误: 未找到 .env${NC}"
    echo "请确保已上传配置文件到当前目录"
    exit 1
fi

echo -e "${GREEN}配置文件检查通过${NC}"

# ============ 完成 ============
echo ""
echo -e "${BLUE}╔════════════════════════════════════════════╗${NC}"
echo -e "${BLUE}║            部署准备就绪                    ║${NC}"
echo -e "${BLUE}╚════════════════════════════════════════════╝${NC}"
echo ""
echo -e "${GREEN}启动服务:${NC}"
echo "  sudo docker-compose -f docker-compose.full.yml up -d"
echo ""
echo -e "${GREEN}查看状态:${NC}"
echo "  sudo docker-compose -f docker-compose.full.yml ps"
echo ""
echo -e "${GREEN}查看日志:${NC}"
echo "  sudo docker-compose -f docker-compose.full.yml logs -f"
echo ""
echo -e "${GREEN}停止服务:${NC}"
echo "  sudo docker-compose -f docker-compose.full.yml down"
echo ""
