#!/bin/bash

set -e

GITHUB_USER="Han-Yu-Meng"
GITHUB_REPO="fins-cli"
BRANCH="main"

# Detect if running in GitHub Actions
if [ "$IS_GITHUB_ACTION" = "true" ]; then
    GH_PROXY=""
    log_info "GitHub Actions environment detected. Disabling proxy."
else
    GH_PROXY="https://gh-proxy.com/"
fi

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
sudo apt-get update -y

# Detect OS version for mold compatibility (available in Ubuntu 22.04+)
UBUNTU_VERSION=$(lsb_release -rs 2>/dev/null || echo "0.0")
if (( $(echo "$UBUNTU_VERSION >= 22.04" | bc -l) )); then
    sudo apt-get install -y ninja-build build-essential curl jq wget mold
else
    sudo apt-get install -y ninja-build build-essential curl jq wget
fi
log_success "System dependencies installed successfully."

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

log_info "Downloading fins : $FINS_URL"
if ! sudo curl -L "$FINS_URL" -o /usr/local/bin/fins; then
    log_error "Failed to download fins from $FINS_URL"
    exit 1
fi

log_info "Downloading finsd: $FINSD_URL"
if ! sudo curl -L "$FINSD_URL" -o /usr/local/bin/finsd; then
    log_error "Failed to download finsd from $FINSD_URL"
    exit 1
fi

# Grant execution permissions
sudo chmod +x /usr/local/bin/fins
sudo chmod +x /usr/local/bin/finsd
log_success "Binary files downloaded and installed successfully."

# 5. Download default configuration files to user directory ~/.fins/
FINS_DIR="$REAL_HOME/.fins"
log_info "Configuring default files to $FINS_DIR ..."

sudo -u "$REAL_USER" mkdir -p "$FINS_DIR"
sudo -u "$REAL_USER" mkdir -p "$FINS_DIR/logs"

CONFIG_URL="${GH_PROXY}https://raw.githubusercontent.com/$GITHUB_USER/$GITHUB_REPO/$BRANCH/default/config.yaml"
RECIPE_URL="${GH_PROXY}https://raw.githubusercontent.com/$GITHUB_USER/$GITHUB_REPO/$BRANCH/default/recipes.yaml"

sudo -u "$REAL_USER" curl -sL "$CONFIG_URL" -o "$FINS_DIR/config.yaml"
sudo -u "$REAL_USER" curl -sL "$RECIPE_URL" -o "$FINS_DIR/recipes.yaml"

sudo chown -R "$REAL_USER":"$REAL_USER" "$FINS_DIR"

log_success "Configuration files download complete."

echo ""
# 6. Final Tips
echo ""
echo -e "${GREEN}======================================================================${NC}"
echo -e "${GREEN}  🎉 FINS Installation Complete!${NC}"
echo -e "${GREEN}======================================================================${NC}"
echo ""
echo -e "To start the background daemon ${YELLOW}finsd${NC}, please run:"
echo -e "  ${BLUE}nohup finsd > /dev/null 2>&1 &${NC}"
echo ""
echo -e "${RED}[Important Next Steps]${NC}"
echo -e "To use Agent and Inspect features correctly, please run the following commands to compile internal tools:"
echo ""
echo -e "  ${YELLOW}fins agent build${NC}"
echo -e "  ${YELLOW}fins inspect build${NC}"
echo ""
echo -e "You can use ${YELLOW}fins --help${NC} anytime to view the help documentation."
echo ""
