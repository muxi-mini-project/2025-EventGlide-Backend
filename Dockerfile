# 第一阶段：构建 Go 应用
FROM golang:alpine AS builder

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
FROM alpine

ARG Project_Name
ARG PORT

# 安装 tzdata 来设置时区
RUN apk add --no-cache tzdata

# 设置时区为 Asia/Shanghai
RUN ln -sf /usr/share/zoneinfo/Asia/Shanghai /etc/localtime && \
    echo "Asia/Shanghai" > /etc/timezone

# 设置工作目录为 /app
WORKDIR /app

# 从 builder 复制编译好的二进制文件（如果只需要二进制文件，可直接复制 app 文件）
COPY --from=builder /app .

# 设置环境变量 PORT
ENV PORT=${PORT}

# 开放端口
EXPOSE ${PORT}

# 启动用户服务
CMD ["./app"]
