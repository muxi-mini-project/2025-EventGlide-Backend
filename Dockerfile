# 第一阶段：构建 Go 应用
FROM golang:1.27.3-alpine AS builder

# 设置 Go 代理为七牛云的代理
ENV GOPROXY=https://goproxy.cn,direct

# 声明构建参数，并赋值给环境变量
ARG Project_Name
ARG PORT

# 切换到 /app 目录
WORKDIR /app

# 拷贝基础文件和项目目录
COPY . /app
RUN go mod tidy

# 构建应用
RUN GO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o app

# 第二阶段：构建最终镜像
FROM alpine:3.24

ARG Project_Name
ARG PORT

# 安装 tzdata 设置时区，显式装 ca-certificates 保证出网 TLS 校验不依赖基础镜像自带
RUN apk add --no-cache tzdata ca-certificates

# 设置时区为 Asia/Shanghai
RUN ln -sf /usr/share/zoneinfo/Asia/Shanghai /etc/localtime && \
    echo "Asia/Shanghai" > /etc/timezone

# 设置工作目录为 /app
WORKDIR /app

# 生产模式：关闭 gin 的 debug 路由打印与调试断言；
# 访问日志由 gin.Logger 中间件输出，不受 GIN_MODE 影响。
ENV GIN_MODE=release

# 健康检查：容器内 busybox wget 探测 /healthz（进程存活 + HTTP 可用）
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
  CMD wget -q -O /dev/null http://127.0.0.1:8080/healthz || exit 1

# 从 builder 复制编译好的二进制文件（如果只需要二进制文件，可直接复制 app 文件）
COPY --from=builder /app .

# 设置环境变量 PORT
ENV PORT=${PORT}

# 开放端口
EXPOSE ${PORT}

# 启动用户服务
CMD ["./app","--config","/app/config/conf.yaml"]
