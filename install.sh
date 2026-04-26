#!/bin/bash

set -e

GITHUB_USER="Han-Yu-Meng"
GITHUB_REPO="fins-cli"
BRANCH="main"

# Define color output
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

log_info() { echo -e "${BLUE}[INFO]${NC} $1"; }
log_success() { echo -e "${GREEN}[SUCCESS]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WARNING]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }

# Detect if running in China (using ipapi.co or similar)
# We use a timeout to avoid hanging if the service is unavailable
log_info "Checking network location..."
IS_CHINA=false
if curl -s --connect-timeout 3 https://ipapi.co/country/ | grep -q "CN"; then
    IS_CHINA=true
fi

if [ "$IS_CHINA" = "true" ]; then
    GH_PROXY="https://gh-proxy.com/"
    log_info "Location: China. Using GitHub proxy."
else
    GH_PROXY=""
    log_info "Location: International. Direct connection."
fi

# Detect if running in GitHub Actions for APT optimization
if [ "$GITHUB_ACTIONS" = "true" ]; then
    export DEBIAN_FRONTEND=noninteractive
    echo 'Acquire::Retries "5";' | sudo tee /etc/apt/apt.conf.d/80-retries
    echo 'Acquire::ForceIPv4 "true";' | sudo tee /etc/apt/apt.conf.d/80-force-ipv4
    log_info "GitHub Actions environment detected. Apt optimized."
fi

# 1. Get the real user
if [ -n "$SUDO_USER" ]; then
    REAL_USER="$SUDO_USER"
else
    REAL_USER="$USER"
fi

# More reliable way to get the actual logged-in user
if [ "$REAL_USER" = "root" ] || [ -z "$REAL_USER" ]; then
    REAL_USER=$(logname 2>/dev/null || echo "$SUDO_USER")
fi
if [ -z "$REAL_USER" ]; then
    REAL_USER=$(who | awk '{print $1}' | head -n 1)
fi
REAL_HOME=$(getent passwd "$REAL_USER" | cut -d: -f6)

log_info "Current installation target user: $REAL_USER, Directory: $REAL_HOME"

# 2. Check and request sudo privileges
if [ "$EUID" -ne 0 ]; then
  log_warn "This script requires root privileges to install dependencies and system services."
  log_warn "Please run using: sudo ./install.sh, or enter password to continue:"
  sudo -v
fi

if pgrep -x "finsd" > /dev/null; then
    sudo pkill -9 -x "finsd"
    log_info "Terminated active finsd processes."
fi

# 3. Install system dependencies
log_info "Installing system dependencies"

# Function to check if packages are installed
check_packages() {
    local pkgs=("$@")
    local missing_pkgs=()
    for pkg in "${pkgs[@]}"; do
        if ! dpkg -s "$pkg" >/dev/null 2>&1; then
            missing_pkgs+=("$pkg")
        fi
    done
    echo "${missing_pkgs[@]}"
}

REQUIRED_PKGS=("ninja-build" "build-essential" "curl" "jq" "wget" "aria2")
UBUNTU_VERSION=$(lsb_release -rs 2>/dev/null || echo "0.0")
if (( $(echo "$UBUNTU_VERSION >= 22.04" | bc -l) )); then
    REQUIRED_PKGS+=("mold")
fi

download_file() {
    local url=$1
    local output=$2
    local name=$3
    log_info "Downloading $name with multi-threaded aria2..."
    if ! sudo aria2c -x 16 -s 16 -d "$(dirname "$output")" -o "$(basename "$output")" "$url"; then
        log_error "Failed to download $name from $url"
        return 1
    fi
    return 0
}

MISSING_PKGS=$(check_packages "${REQUIRED_PKGS[@]}")

if [ -z "$MISSING_PKGS" ]; then
    log_success "All required packages are already installed. Skipping apt-get."
else
    log_info "Missing packages: $MISSING_PKGS. Installing..."
    sudo apt-get update -y
    sudo apt-get install -y $MISSING_PKGS
    log_success "System dependencies installed successfully."
fi

# 4. detect system architecture
ARCH=$(uname -m)
case $ARCH in
    x86_64)
        BINARY_SUFFIX="amd64"
        log_info "Detected architecture: x86_64 (amd64)"
        ;;
    aarch64|arm64)
        BINARY_SUFFIX="arm64"
        log_info "Detected architecture: aarch64 (arm64)"
        ;;
    *)
        log_error "Unsupported architecture: $ARCH"
        exit 1
        ;;
esac

# 5. Get the latest version of binary files from GitHub
log_info "Querying the Release version from GitHub..."

# Try to get the latest stable release, fallback to include pre-releases (for 'latest' tag)
API_URL="https://api.github.com/repos/$GITHUB_USER/$GITHUB_REPO/releases/latest"
# Use -s to be silent, but check for rate limits or other issues
LATEST_RELEASE=$(curl -s "$API_URL")

# Improved check: If stable is missing, has no assets, OR rate limited, try the 'latest' tag
RATE_LIMITED=$(echo "$LATEST_RELEASE" | grep -q "rate limit exceeded" && echo "true" || echo "false")
HAS_ASSETS=$(echo "$LATEST_RELEASE" | jq -r 'if .assets then (.assets | length > 0) else false end' 2>/dev/null || echo "false")

if [ "$RATE_LIMITED" = "true" ] || echo "$LATEST_RELEASE" | grep -q "Not Found" || [ "$HAS_ASSETS" != "true" ]; then
    log_warn "API rate limited or latest stable release not found. Attempting to use static 'latest' tag URLs..."
    
    # Define fallback URLs directly based on your naming convention in release.yml
    FINS_URL="${GH_PROXY}https://github.com/$GITHUB_USER/$GITHUB_REPO/releases/download/latest/fins-linux-$BINARY_SUFFIX"
    FINSD_URL="${GH_PROXY}https://github.com/$GITHUB_USER/$GITHUB_REPO/releases/download/latest/finsd-linux-$BINARY_SUFFIX"
    
    log_info "Attempting direct download from: $FINS_URL"
else
    # Parse download links
    # Use strict matching for the binary name to ensure correct architecture
    FINS_URL=$(echo "$LATEST_RELEASE" | jq -r ".assets[]? | select(.name == \"fins-linux-$BINARY_SUFFIX\") | .browser_download_url" | head -n 1)
    FINSD_URL=$(echo "$LATEST_RELEASE" | jq -r ".assets[]? | select(.name == \"finsd-linux-$BINARY_SUFFIX\") | .browser_download_url" | head -n 1)

    # Fallback if specific architecture binary not found (e.g. if release naming is inconsistent)
    if [ -z "$FINS_URL" ] || [ "$FINS_URL" = "null" ]; then
        FINS_URL=$(echo "$LATEST_RELEASE" | jq -r ".assets[]? | select(.name | test(\"fins-linux-$BINARY_SUFFIX\")) | .browser_download_url" | head -n 1)
    fi
    if [ -z "$FINSD_URL" ] || [ "$FINSD_URL" = "null" ]; then
        FINSD_URL=$(echo "$LATEST_RELEASE" | jq -r ".assets[]? | select(.name | test(\"finsd-linux-$BINARY_SUFFIX\")) | .browser_download_url" | head -n 1)
    fi

    # Prefix with proxy
    FINS_URL="${GH_PROXY}${FINS_URL}"
    FINSD_URL="${GH_PROXY}${FINSD_URL}"
fi

if [ -z "$FINS_URL" ] || [ -z "$FINSD_URL" ]; then
    log_error "Could not determine download URLs."
    exit 1
fi

# Download binary files using aria2 (multi-threaded)
if ! download_file "$FINS_URL" "/usr/local/bin/fins" "fins"; then
    exit 1
fi

if ! download_file "$FINSD_URL" "/usr/local/bin/finsd" "finsd"; then
    exit 1
fi

# Grant execution permissions
sudo chmod +x /usr/local/bin/fins
sudo chmod +x /usr/local/bin/finsd
log_success "Binary files downloaded and installed successfully."

# 5. Download default configuration files to user directory ~/.fins/
FINS_DIR_NAME=".fins"
FINS_DIR="$REAL_HOME/$FINS_DIR_NAME"
log_info "Configuring default files to $FINS_DIR ..."

sudo -u "$REAL_USER" mkdir -p "$FINS_DIR"
sudo -u "$REAL_USER" mkdir -p "$FINS_DIR/logs"

CONFIG_URL="${GH_PROXY}https://raw.githubusercontent.com/$GITHUB_USER/$GITHUB_REPO/$BRANCH/default/config.yaml"
RECIPE_URL="${GH_PROXY}https://raw.githubusercontent.com/$GITHUB_USER/$GITHUB_REPO/$BRANCH/default/recipes.yaml"

sudo -u "$REAL_USER" wget -q "$CONFIG_URL" -O "$FINS_DIR/config.yaml"
sudo -u "$REAL_USER" wget -q "$RECIPE_URL" -O "$FINS_DIR/recipes.yaml"

sudo chown -R "$REAL_USER":"$REAL_USER" "$FINS_DIR"

log_success "Configuration files download complete."

# 检查 systemd 是否可用
if pidof systemd 1>/dev/null && [ -d /run/systemd/system ]; then
    log_info "Configuring systemd service for finsd..."

    SERVICE_FILE="/etc/systemd/system/finsd.service"

    sudo systemctl stop finsd 2>/dev/null || true

    sudo tee $SERVICE_FILE > /dev/null << 'EOF'
[Unit]
Description=Finsd Service
After=network.target

[Service]
Type=simple
User=REPLACE_USER
Group=REPLACE_USER
Environment="HOME=REPLACE_HOME"
Environment="USER=REPLACE_USER"
Environment="SHELL=/bin/bash"
WorkingDirectory=REPLACE_HOME

ExecStart=/bin/bash -c 'eval "$$(sed "/[[ $$- != *i* ]] && return/d; /\[ -z \"$$PS1\" \] && return/d" REPLACE_HOME/.bashrc)"; exec /usr/local/bin/finsd'

Restart=always
RestartSec=3

[Install]
WantedBy=multi-user.target
EOF

    log_info "Personalizing service file for user: $REAL_USER"
    sudo sed -i "s|REPLACE_USER|$REAL_USER|g" $SERVICE_FILE
    sudo sed -i "s|REPLACE_HOME|$REAL_HOME|g" $SERVICE_FILE

    log_info "Reloading systemd and starting finsd..."
    sudo systemctl daemon-reload
    sudo systemctl enable finsd
    sudo systemctl restart finsd

    sleep 2
    if systemctl is-active --quiet finsd; then
        log_success "finsd service is running."
        
        MAIN_PID=$(systemctl show --property=MainPID finsd | cut -d= -f2)
        if [ "$MAIN_PID" != "0" ]; then
            log_info "Verifying ROS Environment for PID $MAIN_PID:"
            sudo cat /proc/$MAIN_PID/environ | tr '\0' '\n' | grep -E "ROS_DISTRO|PATH|LD_LIBRARY_PATH|PYTHONPATH" | sed 's/^/  - /'
        fi
    else
        log_error "Service failed to start. Run 'journalctl -u finsd -n 20' for debugging."
    fi
else
    log_warn "Systemd not detected. Skipping systemd service installation."
fi

echo ""
# 6. Final Tips
echo ""
echo -e "${GREEN}======================================================================${NC}"
echo -e "${GREEN}  🎉 FINS Installation Complete!${NC}"
echo -e "${GREEN}======================================================================${NC}"
echo -e "${RED}[Important Next Steps]${NC}"
echo -e "To use Agent and Inspect features correctly, please run the following commands to compile internal tools:"
echo ""
echo -e "  ${YELLOW}fins agent build${NC}"
echo -e "  ${YELLOW}fins inspect build${NC}"
echo ""
echo -e "You can use ${YELLOW}fins --help${NC} anytime to view the help documentation."
echo ""
