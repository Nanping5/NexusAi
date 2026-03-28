#!/bin/bash

# ================================================
# NexusAi 2核2G 服务器部署脚本
# 保留全部服务（除图像识别外）
# ================================================

set -e

echo "🚀 NexusAi 部署脚本启动..."
echo "================================"

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

# 检查是否为 root 用户
if [ "$EUID" -ne 0 ]; then
    echo -e "${YELLOW}建议使用 root 用户执行此脚本${NC}"
fi

# ============ 1. 系统优化 ============
echo -e "${GREEN}[1/7] 系统优化...${NC}"

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

# 系统参数优化
echo "优化系统参数..."
sudo sysctl -p 2>/dev/null || true

# 设置文件描述符限制
cat << EOF | sudo tee /etc/security/limits.d/nexusai.conf
* soft nofile 65535
* hard nofile 65535
root soft nofile 65535
root hard nofile 65535
EOF

# ============ 2. 安装 Docker ============
echo -e "${GREEN}[2/7] 检查 Docker 环境...${NC}"

if ! command -v docker &> /dev/null; then
    echo "安装 Docker..."
    curl -fsSL https://get.docker.com | sh
    sudo usermod -aG docker $USER
    echo -e "${GREEN}Docker 安装完成${NC}"
else
    echo -e "${YELLOW}Docker 已安装${NC}"
fi

if ! command -v docker-compose &> /dev/null; then
    echo "安装 Docker Compose..."
    sudo curl -L "https://github.com/docker/compose/releases/latest/download/docker-compose-$(uname -s)-$(uname -m)" -o /usr/local/bin/docker-compose
    sudo chmod +x /usr/local/bin/docker-compose
    echo -e "${GREEN}Docker Compose 安装完成${NC}"
else
    echo -e "${YELLOW}Docker Compose 已安装${NC}"
fi

# 启动 Docker
sudo systemctl enable docker
sudo systemctl start docker

# ============ 3. 配置文件检查 ============
echo -e "${GREEN}[3/7] 检查配置文件...${NC}"

if [ ! -f config.toml ]; then
    if [ -f config.toml.example ]; then
        cp config.toml.example config.toml
        echo -e "${YELLOW}已复制 config.toml.example -> config.toml${NC}"
        echo -e "${RED}请编辑 config.toml 填入真实配置！${NC}"
    fi
fi

if [ ! -f .env ]; then
    if [ -f .env.example ]; then
        cp .env.example .env
        echo -e "${YELLOW}已复制 .env.example -> .env${NC}"
        echo -e "${RED}请编辑 .env 填入真实配置！${NC}"
    fi
fi

# ============ 4. 构建前端 ============
echo -e "${GREEN}[4/7] 构建前端...${NC}"

if [ -d "frontend" ]; then
    cd frontend

    # 检查 Node.js
    if ! command -v node &> /dev/null; then
        echo "安装 Node.js..."
        curl -fsSL https://deb.nodesource.com/setup_20.x | sudo -E bash -
        sudo apt-get install -y nodejs
    fi

    # 安装依赖并构建
    npm install
    npm run build
    cd ..
    echo -e "${GREEN}前端构建完成${NC}"
else
    echo -e "${YELLOW}未找到 frontend 目录，跳过前端构建${NC}"
fi

# ============ 5. 拉取镜像 ============
echo -e "${GREEN}[5/7] 拉取基础镜像...${NC}"

docker pull mysql:8.0
docker pull redis:7-alpine
docker pull qdrant/qdrant:latest
docker pull rabbitmq:3.12-management-alpine
docker pull nginx:alpine

echo -e "${GREEN}镜像拉取完成${NC}"

# ============ 6. Docker 清理优化 ============
echo -e "${GREEN}[6/7] Docker 清理优化...${NC}"

# 清理未使用资源
docker system prune -f

# 设置 Docker 日志限制
sudo mkdir -p /etc/docker
cat << EOF | sudo tee /etc/docker/daemon.json
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

# ============ 7. 启动服务 ============
echo -e "${GREEN}[7/7] 启动服务...${NC}"

echo ""
echo "================================"
echo -e "${GREEN}✅ 部署准备完成！${NC}"
echo ""
echo "下一步操作："
echo ""
echo "1. 编辑配置文件："
echo "   vim config.toml  # 配置数据库、Redis等"
echo "   vim .env         # 配置API密钥"
echo ""
echo "2. 启动所有服务："
echo "   docker-compose -f docker-compose.full.yml up -d --build"
echo ""
echo "3. 查看服务状态："
echo "   docker-compose -f docker-compose.full.yml ps"
echo ""
echo "4. 查看日志："
echo "   docker-compose -f docker-compose.full.yml logs -f nexusai"
echo ""
echo "5. 停止服务："
echo "   docker-compose -f docker-compose.full.yml down"
echo ""
echo -e "${YELLOW}⚠️  内存优化提示：${NC}"
echo "   - MySQL: 限制 384MB"
echo "   - Redis: 限制 128MB (使用 96MB maxmemory)"
echo "   - Qdrant: 限制 384MB"
echo "   - RabbitMQ: 限制 384MB (高水位 256MB)"
echo "   - Go后端: 限制 256MB"
echo "   - Nginx: 限制 64MB"
echo ""
echo "   预计总内存占用: 1.4-1.6GB"
echo "   剩余系统内存: 400-600MB (含 Swap 2GB)"
echo ""
