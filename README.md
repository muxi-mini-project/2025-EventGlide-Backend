# EventGlide Backend

EventGlide Backend 是一个基于 Go 构建的校园活动管理系统后端，提供活动发布、报名、审批、消息通知、评论互动等功能。

## 项目介绍

该项目主要面向校园活动场景，支持：

- 活动发布与管理
- 活动报名与审批
- 点赞、收藏、评论互动
- 消息通知
- JWT 用户认证
- 文件上传
- Redis 缓存优化

项目采用MVC架构设计，便于维护与扩展。

---

## 技术栈

### Backend

- Go
- Gin
- GORM

### Database

- MySQL
- Redis

### Middleware & Tools

- JWT
- Zap Logger
- Docker

---

## 项目架构

项目采用经典分层架构：

```text
Client
   ↓
Handler（接口层）
   ↓
Service（业务层）
   ↓
Repo（数据访问层）
   ↓
MySQL / Redis
```

### 各层职责

- handler：处理 HTTP 请求与参数校验
- service：实现核心业务逻辑
- repo：数据库访问与持久化
- middleware：JWT、日志、中间件处理
- model：数据库模型定义
- pkg：公共工具包

---

## 项目结构

```text
.
├── api
│   ├── req
│   └── resp
├── internal
│   ├── handler
│   ├── middleware
│   ├── model
│   ├── repo
│   └── service
├── pkg
├── config
├── docs
└── main.go
```

---

## 核心功能

### 用户模块

- 用户登录注册
- JWT 鉴权
- 用户信息管理

### 活动模块

- 活动发布
- 活动报名
- 活动审批
- 活动搜索

### 社交互动模块

- 评论系统
- 点赞收藏
- 消息通知

### 系统优化

- Redis 缓存热点数据
- 分层架构降低耦合
- 异步处理提升性能

---

## Quick Start

### 环境要求

- Go 1.22+
- MySQL 8+
- Redis 7+

---

### 1. 克隆项目

```bash
git clone https://github.com/muxi-mini-project/2025-EventGlide-Backend.git
cd 2025-EventGlide-Backend
```

---

### 2. 安装依赖

```bash
go mod tidy
```

---

### 3. 配置数据库

修改配置文件：

```text
config/conf-example.yaml
```

示例：

```yaml
mysql:
  dsn: username:password@tcp(addr)/dbname?options
  maxIdleConns: 20 # max-idle-connections
  maxOpenConns: 10 # max-open-connections

redis:
  addr: addr

kafka:
  addr: addr

jwt:
  key: secret-key-for-jwt
  ttl: 259200 # time to live

imgbed:
  accessKey: your-access-key
  secretKey: your-secret-key
  bucket: bucket-name
  imgUrl:   # img-store-url

auditor:
  auditUrl: http://localhost:8080/api/v1
  hookUrl: http://localhost:8081
  apiKeyPath: /api/v1/auditor/getApiKey
  apiKey: apiKey
  webhookPath: /api/v1/auditor/webhook
  effect: slow # slow为先审后发，fast为先发后审
```

---

### 4. 启动项目

```bash
go run .
```

服务默认运行于：

```text
http://localhost:8080
```

---

## Docker 部署

```bash
docker-compose up -d
```
