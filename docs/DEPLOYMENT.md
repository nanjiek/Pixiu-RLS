# Pixiu-RLS 部署指南

## 概述

本文档提供 Pixiu-RLS 在生产环境的部署指南，包括单机部署、集群部署和容器化部署方案。

## 部署架构

### 单机部署

```
┌─────────────────┐
│   Application   │
│    (Client)     │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│   Pixiu-RLS     │
│   (Single)      │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│   Redis         │
└─────────────────┘
```

### 集群部署（推荐）

```
                    ┌─────────────────┐
                    │   Application   │
                    │    (Client)     │
                    └────────┬────────┘
                             │
                    ┌────────▼────────┐
                    │   Load Balancer │
                    │   (Nginx/LB4)   │
                    └────────┬────────┘
                             │
         ┌───────────────────┼───────────────────┐
         │                   │                   │
    ┌────▼────┐         ┌────▼────┐         ┌────▼────┐
    │ RLS-1   │         │ RLS-2   │         │ RLS-3   │
    └────┬────┘         └────┬────┘         └────┬────┘
         │                   │                   │
         └───────────────────┼───────────────────┘
                             │
                    ┌────────▼────────┐
                    │  Redis Cluster  │
                    │  (3 Master +    │
                    │   3 Replica)    │
                    └─────────────────┘
```

## 环境准备

### 系统要求

| 组件 | 最低要求 | 推荐配置 |
|------|---------|---------|
| **CPU** | 2 核 | 4 核+ |
| **内存** | 2GB | 8GB+ |
| **磁盘** | 10GB | 50GB+ SSD |
| **网络** | 100Mbps | 1Gbps+ |
| **操作系统** | Linux 3.10+ | Linux 4.x+ |

### 软件依赖

- **Go**: 1.21+
- **Redis**: 7.0+
- **Docker** (可选): 20.10+
- **Nginx** (可选): 1.18+

## 单机部署

### 1. 安装 Redis

```bash
# Ubuntu/Debian
sudo apt-get update
sudo apt-get install redis-server

# CentOS/RHEL
sudo yum install redis

# 启动 Redis
sudo systemctl start redis
sudo systemctl enable redis

# 验证
redis-cli ping
```

### 2. 编译应用

```bash
# 克隆代码
git clone https://github.com/your-org/pixiu-rls.git
cd pixiu-rls

# 编译
CGO_ENABLED=0 go build -o rls-http \
  -ldflags "-s -w" \
  ./cmd/rls-http

# 验证
./rls-http -version
```

### 3. 配置文件

创建配置文件 `/etc/pixiu-rls/config.yaml`:

```yaml
server:
  httpAddr: ":8080"

redis:
  addr: "localhost:6379"
  db: 0
  prefix: "pixiu:rls"
  updatesChannel: "pixiu:rls:updates"

features:
  audit: "none"
  localFallback: false

bootstrapRules:
  - ruleId: "global-default"
    match: "*"
    algo: "sliding_window"
    windowMs: 1000
    limit: 1000
    dims: ["ip"]
    enabled: true
```

### 4. 创建 Systemd 服务

创建 `/etc/systemd/system/pixiu-rls.service`:

```ini
[Unit]
Description=Pixiu Rate Limiting Service
After=network.target redis.service
Wants=redis.service

[Service]
Type=simple
User=pixiu
Group=pixiu
WorkingDirectory=/opt/pixiu-rls
ExecStart=/opt/pixiu-rls/rls-http -c /etc/pixiu-rls/config.yaml
Restart=on-failure
RestartSec=10s

# 资源限制
LimitNOFILE=65536
LimitNPROC=4096

# 安全加固
NoNewPrivileges=true
PrivateTmp=true

[Install]
WantedBy=multi-user.target
```

### 5. 启动服务

```bash
# 创建用户
sudo useradd -r -s /bin/false pixiu

# 设置权限
sudo mkdir -p /opt/pixiu-rls /etc/pixiu-rls
sudo cp rls-http /opt/pixiu-rls/
sudo chown -R pixiu:pixiu /opt/pixiu-rls

# 启动服务
sudo systemctl daemon-reload
sudo systemctl start pixiu-rls
sudo systemctl enable pixiu-rls

# 查看状态
sudo systemctl status pixiu-rls
```

### 6. 验证部署

```bash
# 健康检查
curl http://localhost:8080/health

# 限流测试
curl -X POST http://localhost:8080/v1/allow \
  -H "Content-Type: application/json" \
  -d '{"ruleId":"global-default","dims":{"ip":"192.168.1.1"}}'
```

## 集群部署

### 1. Redis 集群

#### 使用 Redis Sentinel (推荐)

```bash
# 主节点配置
# /etc/redis/redis-master.conf
bind 0.0.0.0
port 6379
daemonize yes
pidfile /var/run/redis/redis-master.pid
logfile /var/log/redis/redis-master.log
dir /var/lib/redis/master

# 从节点配置
# /etc/redis/redis-slave.conf
bind 0.0.0.0
port 6379
replicaof 192.168.1.100 6379
daemonize yes

# Sentinel 配置
# /etc/redis/sentinel.conf
sentinel monitor mymaster 192.168.1.100 6379 2
sentinel down-after-milliseconds mymaster 5000
sentinel parallel-syncs mymaster 1
sentinel failover-timeout mymaster 10000
```

启动：

```bash
redis-server /etc/redis/redis-master.conf
redis-server /etc/redis/redis-slave.conf
redis-sentinel /etc/redis/sentinel.conf
```

#### 应用配置连接 Sentinel

```yaml
redis:
  addr: "sentinel://192.168.1.101:26379,192.168.1.102:26379,192.168.1.103:26379/mymaster"
  # 其他配置...
```

### 2. Pixiu-RLS 集群

在多台服务器上重复单机部署步骤，注意：

- 所有实例连接同一个 Redis 集群
- 所有实例使用相同的配置（除了 httpAddr）
- 通过负载均衡器分发流量

### 3. 负载均衡配置

#### Nginx 配置

创建 `/etc/nginx/conf.d/pixiu-rls.conf`:

```nginx
upstream pixiu_rls_backend {
    least_conn;  # 最少连接算法
    
    server 192.168.1.10:8080 max_fails=3 fail_timeout=30s;
    server 192.168.1.11:8080 max_fails=3 fail_timeout=30s;
    server 192.168.1.12:8080 max_fails=3 fail_timeout=30s;
    
    keepalive 32;
}

server {
    listen 80;
    server_name rls.example.com;
    
    location / {
        proxy_pass http://pixiu_rls_backend;
        proxy_http_version 1.1;
        proxy_set_header Connection "";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        
        # 超时设置
        proxy_connect_timeout 5s;
        proxy_send_timeout 5s;
        proxy_read_timeout 5s;
    }
    
    # 健康检查
    location /health {
        access_log off;
        proxy_pass http://pixiu_rls_backend/health;
    }
}
```

## Docker 部署

### 1. 构建镜像

创建 `Dockerfile`:

```dockerfile
FROM golang:1.21-alpine AS builder

WORKDIR /build
COPY . .

RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -a \
    -ldflags '-s -w -extldflags "-static"' \
    -o rls-http ./cmd/rls-http

FROM alpine:latest

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

COPY --from=builder /build/rls-http .
COPY configs/rls.yaml /etc/pixiu-rls/config.yaml

EXPOSE 8080

ENTRYPOINT ["/app/rls-http"]
CMD ["-c", "/etc/pixiu-rls/config.yaml"]
```

构建：

```bash
docker build -t pixiu-rls:latest .
```

### 2. Docker Compose 部署

创建 `docker-compose.yaml`:

```yaml
version: '3.8'

services:
  redis:
    image: redis:7-alpine
    container_name: pixiu-redis
    ports:
      - "6379:6379"
    volumes:
      - redis-data:/data
    command: redis-server --appendonly yes
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 5s
      timeout: 3s
      retries: 5

  rls-1:
    image: pixiu-rls:latest
    container_name: pixiu-rls-1
    ports:
      - "8080:8080"
    depends_on:
      redis:
        condition: service_healthy
    environment:
      - REDIS_ADDR=redis:6379
    volumes:
      - ./configs/rls.yaml:/etc/pixiu-rls/config.yaml
    restart: unless-stopped

  rls-2:
    image: pixiu-rls:latest
    container_name: pixiu-rls-2
    ports:
      - "8081:8080"
    depends_on:
      redis:
        condition: service_healthy
    environment:
      - REDIS_ADDR=redis:6379
    volumes:
      - ./configs/rls.yaml:/etc/pixiu-rls/config.yaml
    restart: unless-stopped

  nginx:
    image: nginx:alpine
    container_name: pixiu-nginx
    ports:
      - "80:80"
    volumes:
      - ./nginx.conf:/etc/nginx/nginx.conf
    depends_on:
      - rls-1
      - rls-2
    restart: unless-stopped

volumes:
  redis-data:
```

启动：

```bash
docker-compose up -d
```

## Kubernetes 部署

### 1. ConfigMap

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: pixiu-rls-config
  namespace: pixiu
data:
  config.yaml: |
    server:
      httpAddr: ":8080"
    redis:
      addr: "redis-service:6379"
      db: 0
      prefix: "pixiu:rls"
      updatesChannel: "pixiu:rls:updates"
    features:
      audit: "none"
      localFallback: false
```

### 2. Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: pixiu-rls
  namespace: pixiu
spec:
  replicas: 3
  selector:
    matchLabels:
      app: pixiu-rls
  template:
    metadata:
      labels:
        app: pixiu-rls
    spec:
      containers:
      - name: rls
        image: pixiu-rls:latest
        ports:
        - containerPort: 8080
          name: http
        volumeMounts:
        - name: config
          mountPath: /etc/pixiu-rls
        resources:
          requests:
            cpu: 500m
            memory: 512Mi
          limits:
            cpu: 2000m
            memory: 2Gi
        livenessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 10
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 5
      volumes:
      - name: config
        configMap:
          name: pixiu-rls-config
```

### 3. Service

```yaml
apiVersion: v1
kind: Service
metadata:
  name: pixiu-rls-service
  namespace: pixiu
spec:
  selector:
    app: pixiu-rls
  ports:
  - port: 80
    targetPort: 8080
  type: LoadBalancer
```

### 4. HPA (水平自动扩缩容)

```yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: pixiu-rls-hpa
  namespace: pixiu
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: pixiu-rls
  minReplicas: 3
  maxReplicas: 10
  metrics:
  - type: Resource
    resource:
      name: cpu
      target:
        type: Utilization
        averageUtilization: 70
  - type: Resource
    resource:
      name: memory
      target:
        type: Utilization
        averageUtilization: 80
```

部署：

```bash
kubectl create namespace pixiu
kubectl apply -f configmap.yaml
kubectl apply -f deployment.yaml
kubectl apply -f service.yaml
kubectl apply -f hpa.yaml
```

## 监控与告警

### 1. Prometheus 监控

添加 Prometheus metrics 端点（需要在代码中实现）：

```yaml
# prometheus.yaml
scrape_configs:
  - job_name: 'pixiu-rls'
    static_configs:
      - targets: ['192.168.1.10:8080', '192.168.1.11:8080', '192.168.1.12:8080']
```

### 2. 关键指标

- `pixiu_rls_requests_total` - 总请求数
- `pixiu_rls_allowed_total` - 允许通过的请求数
- `pixiu_rls_denied_total` - 被拒绝的请求数
- `pixiu_rls_latency_seconds` - 请求延迟
- `pixiu_rls_rules_count` - 规则数量

### 3. 告警规则

```yaml
# alerts.yaml
groups:
  - name: pixiu-rls
    rules:
      - alert: HighDenialRate
        expr: rate(pixiu_rls_denied_total[5m]) / rate(pixiu_rls_requests_total[5m]) > 0.5
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "限流拒绝率过高"
          description: "{{ $labels.instance }} 拒绝率超过 50%"
      
      - alert: ServiceDown
        expr: up{job="pixiu-rls"} == 0
        for: 1m
        labels:
          severity: critical
        annotations:
          summary: "服务宕机"
          description: "{{ $labels.instance }} 已宕机"
```

## 性能优化

### 1. 系统参数优化

```bash
# /etc/sysctl.conf
net.core.somaxconn = 65535
net.ipv4.tcp_max_syn_backlog = 65535
net.ipv4.ip_local_port_range = 1024 65535
net.ipv4.tcp_fin_timeout = 30
net.ipv4.tcp_keepalive_time = 300
net.ipv4.tcp_tw_reuse = 1

# 应用配置
sudo sysctl -p
```

### 2. Redis 优化

```bash
# /etc/redis/redis.conf
maxmemory 4gb
maxmemory-policy allkeys-lru
save ""  # 禁用 RDB，使用 AOF
appendonly yes
appendfsync everysec
```

### 3. 应用优化

- 调整 `GOMAXPROCS` 为 CPU 核心数
- 使用连接池
- 启用 HTTP Keep-Alive
- 合理设置超时时间

## 安全加固

### 1. 网络安全

```bash
# 防火墙配置
sudo firewall-cmd --permanent --add-port=8080/tcp
sudo firewall-cmd --reload

# 只允许特定 IP 访问
sudo firewall-cmd --permanent --add-rich-rule='rule family="ipv4" source address="10.0.0.0/8" port port="8080" protocol="tcp" accept'
```

### 2. Redis 安全

```bash
# redis.conf
requirepass your_strong_password
bind 127.0.0.1 192.168.1.100  # 绑定特定 IP
protected-mode yes
rename-command FLUSHDB ""
rename-command FLUSHALL ""
rename-command CONFIG ""
```

### 3. TLS/SSL

配置 Nginx 使用 HTTPS：

```nginx
server {
    listen 443 ssl http2;
    server_name rls.example.com;
    
    ssl_certificate /path/to/cert.pem;
    ssl_certificate_key /path/to/key.pem;
    
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers HIGH:!aNULL:!MD5;
    
    location / {
        proxy_pass http://pixiu_rls_backend;
    }
}
```

## 故障排查

### 常见问题

#### 1. 服务无法启动

```bash
# 查看日志
sudo journalctl -u pixiu-rls -f

# 检查端口占用
sudo netstat -tulpn | grep 8080

# 检查 Redis 连接
redis-cli -h localhost -p 6379 ping
```

#### 2. 性能问题

```bash
# 查看 CPU 和内存
top -p $(pgrep rls-http)

# 查看 goroutine 数量
curl http://localhost:6060/debug/pprof/goroutine?debug=1

# 查看 Redis 性能
redis-cli --stat
```

#### 3. Redis 连接问题

```bash
# 检查 Redis 连接数
redis-cli CLIENT LIST | wc -l

# 检查慢查询
redis-cli SLOWLOG GET 10
```

## 备份与恢复

### Redis 数据备份

```bash
# 手动备份
redis-cli BGSAVE

# 定时备份脚本
#!/bin/bash
DATE=$(date +%Y%m%d_%H%M%S)
redis-cli BGSAVE
sleep 5
cp /var/lib/redis/dump.rdb /backup/redis_$DATE.rdb
```

### 配置备份

```bash
# 备份配置
tar -czf pixiu-rls-config-$(date +%Y%m%d).tar.gz \
  /etc/pixiu-rls \
  /etc/systemd/system/pixiu-rls.service
```

## 升级指南

### 滚动升级

```bash
# 1. 编译新版本
go build -o rls-http-new ./cmd/rls-http

# 2. 逐个节点升级
for host in rls-1 rls-2 rls-3; do
    ssh $host 'sudo systemctl stop pixiu-rls'
    scp rls-http-new $host:/opt/pixiu-rls/rls-http
    ssh $host 'sudo systemctl start pixiu-rls'
    sleep 10  # 等待服务就绪
done
```

## 参考资料

- [API 文档](./API.md)
- [开发指南](./DEVELOPMENT.md)
- [RCU 架构](./RCU_ARCHITECTURE.md)
- [Redis 官方文档](https://redis.io/documentation)

---

**版本**: v1.0  
**最后更新**: 2024-01

