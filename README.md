<div align="center">

# NexusAi

**一个功能完整的 AI 对话平台**

集成用户认证、AI 对话、RAG 检索增强、图像识别、语音合成等多种功能

[![Go](https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat&logo=go)](https://golang.org/)
[![Gin](https://img.shields.io/badge/Gin-v1.12.0-008EC3?style=flat)](https://github.com/gin-gonic/gin)
[![MySQL](https://img.shields.io/badge/MySQL-8.0+-4479A1?style=flat&logo=mysql&logoColor=white)](https://www.mysql.com/)
[![Redis](https://img.shields.io/badge/Redis-6.0+-DC382D?style=flat&logo=redis&logoColor=white)](https://redis.io/)
[![License](https://img.shields.io/badge/License-MIT-green?style=flat)](LICENSE)

[功能特性](#-功能特性) • [快速开始](#-快速开始) • [API 文档](#-api-文档) • [配置说明](#️-配置)

</div>

---

## 📖 目录

- [功能特性](#-功能特性)
- [技术栈](#-技术栈)
- [项目结构](#-项目结构)
- [快速开始](#-快速开始)
- [配置说明](#️-配置)
- [API 文档](#-api-文档)
- [MCP 服务](#-mcp-服务)
- [数据模型](#-数据模型)
- [开发指南](#-开发指南)

---

## ✨ 功能特性

### 🤖 AI 对话
- 多轮对话与会话管理
- 流式响应 (SSE)
- 动态模型切换
- Agent 模式 (ReAct)
- MCP 工具调用
- 上下文长度智能管理

### 🔍 RAG 检索增强
- 知识库文件上传 (PDF/TXT/DOCX)
- 文档向量化存储 (Qdrant)
- 会话级别知识隔离
- 自动检索增强上下文

### 🖼️ 图像识别
- MobileNetV2 图像分类
- ONNX Runtime 推理
- 1000 种 ImageNet 分类支持

### 🔊 语音合成
- 阿里云智能语音服务
- 文本转语音 (WAV 16000Hz)

### 🔐 用户系统
- 邮箱注册/登录
- JWT Token 认证
- 验证码邮件发送

### ⚡ 性能优化
- 多级 Redis 缓存架构
- RabbitMQ 异步消息处理
- 接口限流保护

---

## 🛠 技术栈

| 分类 | 技术 |
|------|------|
| **后端框架** | Go + Gin |
| **数据库** | MySQL + Redis + Qdrant |
| **消息队列** | RabbitMQ |
| **AI 框架** | CloudWeGo Eino |
| **AI 推理** | ONNX Runtime |
| **协议支持** | MCP (Model Context Protocol) |
| **认证** | JWT |

---

## 📁 项目结构

```
NexusAi/
├── main.go                 # 应用入口
├── config/                 # 配置管理
├── common/                 # 公共组件
│   ├── ai_helper/          # AI 模型封装
│   ├── mcp/                # MCP 服务实现
│   ├── rag/                # RAG 检索增强
│   ├── image/              # 图像识别
│   └── tts/                # 语音合成
├── controller/             # 控制器层
├── service/                # 服务层
├── dao/                    # 数据访问层
├── model/                  # 数据模型
├── middleware/             # 中间件
├── router/                 # 路由定义
├── pkg/                    # 工具包
└── docs/                   # API 文档
```

---

## 🚀 快速开始

### 环境要求

- Go 1.21+
- MySQL 8.0+
- Redis 6.0+
- Qdrant (可选，用于 RAG)
- RabbitMQ 3.x (可选)

### 安装运行

```bash
# 克隆项目
git clone https://github.com/Nanping5/NexusAi.git
cd NexusAi

# 安装依赖
go mod download

# 复制配置文件
cp config.toml.example config.toml
cp .env.example .env

# 修改配置（填入真实的数据库、API密钥等）
vim config.toml
vim .env

# 运行服务
go run main.go
```

服务默认运行在 `http://localhost:8080`

---

## ⚙️ 配置

### 配置文件

| 文件 | 说明 |
|------|------|
| `config.toml` | 主配置文件（数据库、Redis、JWT 等） |
| `.env` | 环境变量（API 密钥、代理等敏感配置） |

### 主要配置项

```toml
# config.toml
[main_config]
app_name = "NexusAi"
host = "0.0.0.0"
port = 8080

[mysql_config]
host = "127.0.0.1"
port = 3306
user = "root"
password = "your_password"
db_name = "NexusAi"

[redis_config]
host = "127.0.0.1"
port = 6379

[jwt_config]
secret_key = "your_jwt_secret_key"
```

```bash
# .env
RAG_OPENAI_API_KEY=your_api_key
SERPAPI_KEY=your_serpapi_key
ALIBABA_ACCESS_KEY_ID=your_access_key
ALIBABA_ACCESS_KEY_SECRET=your_access_secret
AMAP_API_KEY=your_amap_key
```

---

## 📚 API 文档

详细 API 文档请参阅 [docs/api/](docs/api/) 目录：

| 模块 | 文档 | 说明 |
|------|------|------|
| 用户 | [user_api.md](docs/api/user_api.md) | 注册、登录、用户信息 |
| AI 会话 | [ai_api.md](docs/api/ai_api.md) | 对话、流式响应、历史记录 |
| 图像识别 | [image_api.md](docs/api/image_api.md) | 图像分类识别 |
| RAG | [rag_api.md](docs/api/rag_api.md) | 知识库文件上传 |
| 语音合成 | [tts_api.md](docs/api/tts_api.md) | 文本转语音 |
| 管理员 | [admin_api.md](docs/api/admin_api.md) | 管理员登录、模型配置 |

### 快速示例

```bash
# 登录获取 Token
curl -X POST "http://localhost:8080/api/v1/user/login" \
  -H "Content-Type: application/json" \
  -d '{"email":"user@example.com","password":"123456"}'

# AI 对话（流式）
curl -X POST "http://localhost:8080/api/v1/ai/session/create/stream" \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{"user_question":"你好","model_config_id":"<config_id>"}'

# 图像识别
curl -X POST "http://localhost:8080/api/v1/image/recognize" \
  -H "Authorization: Bearer <token>" \
  -F "image=@/path/to/image.jpg"
```

---

## 🔌 MCP 服务

MCP (Model Context Protocol) 提供外部工具集成能力：

```bash
# 启动所有 MCP 服务
go run common/mcp/main.go

# 启动指定服务
go run common/mcp/main.go --services=weather,search,tts,translate
```

| 服务 | 功能 |
|------|------|
| weather | 天气查询 |
| search | 网络搜索 |
| translate | 文本翻译 |
| tts | 语音合成 |

---

## 📊 数据模型

<details>
<summary>点击查看数据表结构</summary>

### User (用户表)
| 字段 | 类型 | 说明 |
|------|------|------|
| id | uint | 主键 |
| user_id | varchar(20) | 用户ID (U+6位数字) |
| nickname | varchar(50) | 昵称 |
| email | varchar(100) | 邮箱 (唯一) |
| password | varchar(255) | 密码 (bcrypt加密) |

### Session (会话表)
| 字段 | 类型 | 说明 |
|------|------|------|
| id | uint | 主键 |
| session_id | varchar(36) | 会话ID (UUID) |
| user_id | varchar(20) | 用户ID |
| title | varchar(100) | 会话标题 |

### Message (消息表)
| 字段 | 类型 | 说明 |
|------|------|------|
| id | uint | 主键 |
| session_id | varchar(36) | 会话ID |
| content | text | 消息内容 |
| is_user | bool | 是否用户消息 |

### AIModelConfig (AI模型配置表)
| 字段 | 类型 | 说明 |
|------|------|------|
| id | uint | 主键 |
| config_id | varchar(36) | 配置ID (UUID) |
| name | varchar(100) | 显示名称 |
| model_type | varchar(50) | 模型类型 |
| base_url | varchar(255) | API地址 |
| model_name | varchar(100) | 模型名称 |
| api_key | varchar(255) | API密钥 |
| is_enabled | bool | 是否启用 |
| is_default | bool | 是否默认 |

</details>

---

## 🧪 开发指南

```bash
# 构建
go build -o nexusai main.go

# 测试
go test ./...

# MCP 服务构建
go build -o mcp_server ./common/mcp
```

---

## 📄 License

[MIT License](LICENSE)

---

<div align="center">

**[⬆ 回到顶部](#nexusai)**

Made with ❤️ by [Nanping5](https://github.com/Nanping5)

</div>
