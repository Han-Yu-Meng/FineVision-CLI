#!/bin/bash

set -e

GITHUB_USER="Han-Yu-Meng"
GITHUB_REPO="fins-cli"
BRANCH="main"

# --- 颜色输出定义 ---
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

log_info() { echo -e "${BLUE}[INFO]${NC} $1"; }
log_success() { echo -e "${GREEN}[SUCCESS]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WARNING]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }

# --- 1. 捕获当前环境代理变量 ---
# 即使使用 sudo 运行，也要确保这些变量被记住
USER_HTTP_PROXY="${http_proxy:-$HTTP_PROXY}"
USER_HTTPS_PROXY="${https_proxy:-$HTTPS_PROXY}"
USER_ALL_PROXY="${all_proxy:-$ALL_PROXY}"
USER_NO_PROXY="${no_proxy:-$NO_PROXY}"

# --- 2. 检测网络环境 ---
log_info "Checking network location..."
IS_CHINA=false

if curl -4 -s -m 3 --noproxy "*" https://www.baidu.com > /dev/null; then
    if ! curl -4 -s -m 2 --noproxy "*" https://github.com > /dev/null; then
        IS_CHINA=true
    fi
else
    IS_CHINA=false
fi

if [ "$IS_CHINA" = "false" ]; then
    if timedatectl 2>/dev/null | grep -q "Asia/Shanghai"; then
        IS_CHINA=true
    fi
fi

if [ "$IS_CHINA" = "true" ]; then
    GH_PROXY="https://gh-proxy.com/"
    log_info "Location: China. Using GitHub mirror: ${GH_PROXY}"
else
    GH_PROXY=""
    log_info "Location: International. Direct connection."
fi

# --- 3. 获取真实用户信息 ---
if [ -n "$SUDO_USER" ]; then
    REAL_USER="$SUDO_USER"
else
    REAL_USER="$USER"
fi
if [ "$REAL_USER" = "root" ] || [ -z "$REAL_USER" ]; then
    REAL_USER=$(logname 2>/dev/null || echo "$SUDO_USER")
fi
if [ -z "$REAL_USER" ]; then
    REAL_USER=$(who | awk '{print $1}' | head -n 1)
fi
REAL_HOME=$(getent passwd "$REAL_USER" | cut -d: -f6)

log_info "Target user: $REAL_USER, Home: $REAL_HOME"

# --- 4. 权限检查与初始化 ---
if [ "$EUID" -ne 0 ]; then
  log_warn "This script requires root privileges. Please enter password:"
  sudo -v
fi

if pgrep -x "finsd" > /dev/null; then
    sudo pkill -9 -x "finsd"
    log_info "Terminated active finsd processes."
fi

# --- 5. 安装依赖 ---
log_info "Installing system dependencies..."
REQUIRED_PKGS=("ninja-build" "build-essential" "curl" "jq" "wget" "aria2" "git")
UBUNTU_VERSION=$(lsb_release -rs 2>/dev/null || echo "0.0")
if (( $(echo "$UBUNTU_VERSION >= 22.04" | bc -l) )); then
    REQUIRED_PKGS+=("mold")
fi

sudo apt-get update -y
sudo apt-get install -y "${REQUIRED_PKGS[@]}"

# --- 6. 架构检测 ---
ARCH=$(uname -m)
case $ARCH in
    x86_64) BINARY_SUFFIX="amd64" ;;
    aarch64|arm64) BINARY_SUFFIX="arm64" ;;
    *) log_error "Unsupported architecture: $ARCH"; exit 1 ;;
esac
log_info "Architecture: $ARCH ($BINARY_SUFFIX)"

# --- 7. 获取版本及下载链接 ---
log_info "Fetching release information..."
API_URL="https://api.github.com/repos/$GITHUB_USER/$GITHUB_REPO/releases/latest"
# 使用代理（如果存在）请求 API
LATEST_RELEASE=$(curl -s --connect-timeout 5 -H "Accept: application/vnd.github.v3+json" "$API_URL" || echo "")

# 解析或回退
FINS_URL=$(echo "$LATEST_RELEASE" | jq -r ".assets[]? | select(.name == \"fins-linux-$BINARY_SUFFIX\") | .browser_download_url" | head -n 1)
FINSD_URL=$(echo "$LATEST_RELEASE" | jq -r ".assets[]? | select(.name == \"finsd-linux-$BINARY_SUFFIX\") | .browser_download_url" | head -n 1)

if [ -z "$FINS_URL" ] || [ "$FINS_URL" = "null" ]; then
    log_warn "API rate limited or unreachable. Using fallback URLs."
    FINS_URL="https://github.com/$GITHUB_USER/$GITHUB_REPO/releases/download/latest/fins-linux-$BINARY_SUFFIX"
    FINSD_URL="https://github.com/$GITHUB_USER/$GITHUB_REPO/releases/download/latest/finsd-linux-$BINARY_SUFFIX"
fi

# 应用中国区加速
FINS_URL="${GH_PROXY}${FINS_URL}"
FINSD_URL="${GH_PROXY}${FINSD_URL}"

# --- 8. 下载函数 (增强代理支持) ---
download_file() {
    local url=$1
    local output=$2
    local name=$3
    log_info "Downloading $name..."
    sudo rm -f "$output"
    
    # 构造 aria2 代理参数
    local proxy_cmd=""
    if [ -n "$USER_ALL_PROXY" ]; then
        proxy_cmd="--all-proxy=$USER_ALL_PROXY"
    fi

    # 使用 sudo -E 保留环境变量
    if ! sudo -E aria2c -x 16 -s 16 --allow-overwrite=true $proxy_cmd -d "$(dirname "$output")" -o "$(basename "$output")" "$url"; then
        log_warn "Aria2 failed, trying wget fallback..."
        sudo -E wget -q --show-progress "$url" -O "$output"
    fi
}

download_file "$FINS_URL" "/usr/local/bin/fins" "fins"
download_file "$FINSD_URL" "/usr/local/bin/finsd" "finsd"

sudo chmod +x /usr/local/bin/fins /usr/local/bin/finsd
log_success "Binaries installed to /usr/local/bin/"

# --- 9. 配置文件处理 ---
FINS_DIR="$REAL_HOME/.fins"
log_info "Setting up config files in $FINS_DIR..."

sudo -u "$REAL_USER" mkdir -p "$FINS_DIR/logs"

CONFIG_URL="${GH_PROXY}https://raw.githubusercontent.com/$GITHUB_USER/$GITHUB_REPO/$BRANCH/default/config.yaml"
RECIPE_URL="${GH_PROXY}https://raw.githubusercontent.com/$GITHUB_USER/$GITHUB_REPO/$BRANCH/default/recipes.yaml"

# 显式传递代理给 wget
sudo -E -u "$REAL_USER" wget -q "$CONFIG_URL" -O "$FINS_DIR/config.yaml"
sudo -E -u "$REAL_USER" wget -q "$RECIPE_URL" -O "$FINS_DIR/recipes.yaml"
sudo chown -R "$REAL_USER":"$REAL_USER" "$FINS_DIR"

# --- 10. Systemd 服务配置 ---
if pidof systemd 1>/dev/null && [ -d /run/systemd/system ]; then
    log_info "Configuring systemd service..."
    SERVICE_FILE="/etc/systemd/system/finsd.service"
    sudo systemctl stop finsd 2>/dev/null || true

    sudo tee $SERVICE_FILE > /dev/null <<EOF
[Unit]
Description=Finsd Service
After=network.target

[Service]
Type=simple
User=$REAL_USER
Group=$REAL_USER
Environment="HOME=$REAL_HOME"
Environment="USER=$REAL_USER"
Environment="SHELL=/bin/bash"
WorkingDirectory=$REAL_HOME
ExecStart=/bin/bash -c 'eval "\$\$(sed "/[[ \$\$- != *i* ]] && return/d; /\\[ -z \\"\$PS1\\" \\] && return/d" $REAL_HOME/.bashrc)"; exec /usr/local/bin/finsd'
Restart=always
RestartSec=3

[Install]
WantedBy=multi-user.target
EOF

    sudo systemctl daemon-reload
    sudo systemctl enable finsd
    sudo systemctl restart finsd
    log_success "finsd service started."
fi

# --- 11. Clone SDK (使用代理优化的 Git) ---
SDK_DIR="$FINS_DIR/sdk"
log_info "Cloning FineVision SDK to $SDK_DIR..."
FINEVISION_REPO="https://github.com/FINS-Fines/FineVision.git"
[ "$IS_CHINA" = "true" ] && FINEVISION_REPO="${GH_PROXY}${FINEVISION_REPO}"

# 如果已有则删除（可选，根据需求决定是否清空重拉）
# sudo rm -rf "$SDK_DIR"

# 在 Git 命令中注入代理配置
GIT_PROXY_ARGS=""
if [ -n "$USER_HTTP_PROXY" ]; then
    GIT_PROXY_ARGS="-c http.proxy=$USER_HTTP_PROXY -c https.proxy=$USER_HTTPS_PROXY"
fi

if sudo -E -u "$REAL_USER" git $GIT_PROXY_ARGS clone -b dev "$FINEVISION_REPO" "$SDK_DIR" 2>/dev/null || [ -d "$SDK_DIR" ]; then
    log_success "FineVision SDK is ready."
else
    log_warn "Git clone failed. Please check your connection."
fi

# --- 12. 完成提示 ---
echo ""
echo -e "${GREEN}======================================================================${NC}"
echo -e "${GREEN}  FineVision-CLI Installation Complete!${NC}"
echo -e "${GREEN}======================================================================${NC}"
echo -e "${RED}[Important Next Steps]${NC}"
echo ""
echo -e "To use Agent and Inspect features, please run:"
echo -e "  ${YELLOW}fins agent build${NC}"
echo -e "  ${YELLOW}fins inspect build${NC}"
echo ""
echo -e "Help: ${YELLOW}fins --help${NC}"
echo ""
