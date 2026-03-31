#!/bin/bash

# ==========================================
# FINS Automation Uninstallation Script
# ==========================================

set -e

# Define color output
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

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
REAL_HOME=$(eval echo "~$REAL_USER")

# 2. Check and request sudo privileges
if [ "$EUID" -ne 0 ]; then
  log_warn "This script requires root privileges to stop services and remove files."
  log_warn "Please run using: sudo ./uninstall.sh, or enter password to continue:"
  sudo -v
fi

# 3. Stop and remove systemd service
log_info "Stopping and disabling finsd service..."
if systemctl is-active --quiet finsd; then
    sudo systemctl stop finsd
    log_info "finsd service stopped."
fi

if systemctl is-enabled --quiet finsd; then
    sudo systemctl disable finsd
    log_info "finsd service disabled."
fi

SYSTEMD_FILE="/etc/systemd/system/finsd.service"
if [ -f "$SYSTEMD_FILE" ]; then
    sudo rm "$SYSTEMD_FILE"
    sudo systemctl daemon-reload
    log_info "systemd service file removed."
fi

# 4. Remove binaries
log_info "Removing binary files from /usr/local/bin/..."
[ -f /usr/local/bin/fins ] && sudo rm /usr/local/bin/fins && log_info "Removed /usr/local/bin/fins"
[ -f /usr/local/bin/finsd ] && sudo rm /usr/local/bin/finsd && log_info "Removed /usr/local/bin/finsd"

# 5. Remove configuration directory
FINS_DIR="$REAL_HOME/.fins"
if [ -d "$FINS_DIR" ]; then
    log_warn "Removing configuration directory: $FINS_DIR"
    sudo rm -rf "$FINS_DIR"
    log_success "Configuration directory removed."
else
    log_info "No configuration directory found at $FINS_DIR"
fi

# 6. Final success message
echo ""
echo -e "${GREEN}======================================================================${NC}"
echo -e "${GREEN}  🎉 FINS Uninstallation Complete!${NC}"
echo -e "${GREEN}======================================================================${NC}"
echo ""
log_info "FINS has been completely removed from your system."
echo ""
