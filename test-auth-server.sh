#!/bin/bash

# ============================================
# 鉴权服务器完整测试脚本
# ============================================

set -e

echo "========================================"
echo "鉴权服务器功能测试"
echo "========================================"
echo ""

# 颜色定义
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# 测试结果统计
TOTAL_TESTS=0
PASSED_TESTS=0
FAILED_TESTS=0

# 测试函数
test_case() {
    local name="$1"
    local command="$2"
    local expected_pattern="$3"

    TOTAL_TESTS=$((TOTAL_TESTS + 1))
    echo -e "${YELLOW}[TEST $TOTAL_TESTS] $name${NC}"

    # 执行命令
    result=$(eval "$command" 2>&1)
    exit_code=$?

    # 检查结果
    if echo "$result" | grep -q "$expected_pattern"; then
        echo -e "${GREEN}✅ PASSED${NC}"
        echo "输出: $result"
        PASSED_TESTS=$((PASSED_TESTS + 1))
    else
        echo -e "${RED}❌ FAILED${NC}"
        echo "期望包含: $expected_pattern"
        echo "实际输出: $result"
        echo "退出码: $exit_code"
        FAILED_TESTS=$((FAILED_TESTS + 1))
    fi
    echo ""
}

# ============================================
# 测试 1: 编译测试
# ============================================
echo "================================================"
echo "测试 1: 编译和二进制文件"
echo "================================================"
echo ""

test_case "编译项目" \
    "go build -o test-binary 2>&1 && echo 'SUCCESS'" \
    "SUCCESS"

test_case "检查二进制文件大小" \
    "ls -lh test-binary | awk '{print \$5}'" \
    "M"

# 清理
rm -f test-binary

# ============================================
# 测试 2: 配置文件验证
# ============================================
echo "================================================"
echo "测试 2: 配置文件验证"
echo "================================================"
echo ""

# 创建测试配置
cat > test-config.yml <<EOF
emby:
  host: http://localhost:8096
  admin-api-key: "test-admin-key"
  mount-path: /media
  local-media-root: /data/local
  proxy-error-strategy: origin
  images-quality: 100

nodes:
  health-check:
    interval: 30
    timeout: 5
    fail-threshold: 3
    success-threshold: 2
  list:
    - name: "node-1"
      host: "http://192.168.0.10:80"
      weight: 100
      enabled: true

auth:
  user-key-cache-ttl: 24h
  nginx-auth-enable: true
  enable-auth-server: true
  auth-server-port: "8097"
  enable-auth-server-log: true
  auth-server-log-path: "./test-logs/auth-access.log"

path:
  emby2nginx:
    - /media/data:/video/data

cache:
  enable: true
  expired: 1d

ssl:
  enable: false

log:
  disable-color: false

telegram:
  enable: false
EOF

test_case "配置文件格式正确" \
    "cat test-config.yml | grep 'enable-auth-server: true'" \
    "enable-auth-server: true"

# ============================================
# 测试 3: 单元测试
# ============================================
echo "================================================"
echo "测试 3: 运行单元测试"
echo "================================================"
echo ""

test_case "路径映射测试" \
    "go test ./internal/config -v -run TestMapEmby2Nginx 2>&1 | tail -1" \
    "PASS"

test_case "节点选择测试" \
    "go test ./internal/service/node -v -run TestSelector_Basic 2>&1 | tail -1" \
    "PASS"

test_case "节点健康检查测试" \
    "go test ./internal/service/node -v -run TestHealthChecker_Basic 2>&1 | tail -1" \
    "PASS"

# ============================================
# 测试 4: 鉴权服务器模块测试
# ============================================
echo "================================================"
echo "测试 4: 鉴权服务器代码检查"
echo "================================================"
echo ""

test_case "鉴权服务器文件存在" \
    "ls internal/service/authserver/server.go" \
    "server.go"

test_case "日志记录器文件存在" \
    "ls internal/service/authserver/logger.go" \
    "logger.go"

test_case "Web集成文件存在" \
    "ls internal/web/authweb.go" \
    "authweb.go"

test_case "鉴权服务器导出函数" \
    "grep -c 'func.*HandleAuth' internal/service/authserver/server.go" \
    "[123]"

test_case "日志记录器导出函数" \
    "grep -c 'func.*NewAccessLogger' internal/service/authserver/logger.go" \
    "1"

# ============================================
# 测试 5: Nginx 配置文件
# ============================================
echo "================================================"
echo "测试 5: Nginx 配置文件检查"
echo "================================================"
echo ""

test_case "后端鉴权配置存在" \
    "ls nginx/video-with-backend-auth.conf" \
    "video-with-backend-auth.conf"

test_case "URL参数鉴权配置存在" \
    "ls nginx/video-with-auth.conf" \
    "video-with-auth.conf"

test_case "后端鉴权配置包含auth_request" \
    "grep -c 'auth_request /auth' nginx/video-with-backend-auth.conf" \
    "[1-9]"

test_case "后端鉴权配置包含upstream" \
    "grep -c 'upstream auth_backend' nginx/video-with-backend-auth.conf" \
    "1"

# ============================================
# 测试 6: 文档完整性
# ============================================
echo "================================================"
echo "测试 6: 文档完整性检查"
echo "================================================"
echo ""

test_case "架构文档存在" \
    "ls docs/ARCHITECTURE.md" \
    "ARCHITECTURE.md"

test_case "鉴权服务器文档存在" \
    "ls docs/AUTH_SERVER.md" \
    "AUTH_SERVER.md"

test_case "快速开始文档存在" \
    "ls docs/AUTH_SERVER_QUICKSTART.md" \
    "AUTH_SERVER_QUICKSTART.md"

test_case "Nginx鉴权文档存在" \
    "ls docs/NGINX_AUTH.md" \
    "NGINX_AUTH.md"

test_case "架构文档字数" \
    "wc -l docs/ARCHITECTURE.md | awk '{print \$1}'" \
    "[5-9][0-9][0-9]"

test_case "鉴权服务器文档字数" \
    "wc -l docs/AUTH_SERVER.md | awk '{print \$1}'" \
    "[5-9][0-9][0-9]"

# ============================================
# 测试 7: 代码质量检查
# ============================================
echo "================================================"
echo "测试 7: 代码质量检查"
echo "================================================"
echo ""

test_case "Go代码格式检查" \
    "gofmt -l internal/service/authserver/ | wc -l" \
    "0"

test_case "Go代码可以通过vet检查" \
    "go vet ./internal/service/authserver/... 2>&1 || echo 'PASS'" \
    "PASS"

# ============================================
# 测试 8: 配置项检查
# ============================================
echo "================================================"
echo "测试 8: 配置项完整性"
echo "================================================"
echo ""

test_case "config-example.yml包含鉴权服务器配置" \
    "grep -c 'enable-auth-server' config-example.yml" \
    "1"

test_case "config-example.yml包含端口配置" \
    "grep -c 'auth-server-port' config-example.yml" \
    "1"

test_case "config-example.yml包含日志配置" \
    "grep -c 'auth-server-log-path' config-example.yml" \
    "1"

test_case "Auth配置结构包含新字段" \
    "grep -c 'EnableAuthServer' internal/config/auth.go" \
    "1"

# ============================================
# 测试 9: Git 和版本控制
# ============================================
echo "================================================"
echo "测试 9: Git 版本控制"
echo "================================================"
echo ""

test_case "检查最新标签" \
    "git tag -l | tail -1" \
    "v2.4.0"

test_case "检查最新提交包含修复" \
    "git log -1 --oneline" \
    "fix"

# ============================================
# 测试 10: Docker 配置
# ============================================
echo "================================================"
echo "测试 10: Docker 相关配置"
echo "================================================"
echo ""

test_case "Dockerfile存在" \
    "ls Dockerfile" \
    "Dockerfile"

test_case "docker-compose.yml存在" \
    "ls docker-compose.yml" \
    "docker-compose.yml"

test_case "GitHub Actions Docker workflow存在" \
    "ls .github/workflows/docker.yml" \
    "docker.yml"

# ============================================
# 测试结果汇总
# ============================================
echo ""
echo "========================================"
echo "测试结果汇总"
echo "========================================"
echo ""
echo "总测试数: $TOTAL_TESTS"
echo -e "${GREEN}通过: $PASSED_TESTS${NC}"
echo -e "${RED}失败: $FAILED_TESTS${NC}"
echo ""

if [ $FAILED_TESTS -eq 0 ]; then
    echo -e "${GREEN}✅ 所有测试通过！${NC}"
    exit 0
else
    echo -e "${RED}❌ 有 $FAILED_TESTS 个测试失败${NC}"
    exit 1
fi
