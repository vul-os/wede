#!/usr/bin/env bash
set -euo pipefail

REPO="webcrft/wede"
BINARY="wede"
echo "Installing wede..."
echo ""

# Check dependencies
if ! command -v curl &> /dev/null; then
  echo "Error: curl is required but not installed."
  echo "  Install it with your package manager:"
  echo "    Ubuntu/Debian: sudo apt install curl"
  echo "    macOS:         brew install curl"
  echo "    Fedora:        sudo dnf install curl"
  exit 1
fi

# Detect OS and set install directory
OS="$(uname -s)"
case "$OS" in
  Linux*)
    OS="linux"
    INSTALL_DIR="${HOME}/.local/bin"
    ;;

  Darwin*)
    OS="darwin"
    INSTALL_DIR="${HOME}/.local/bin"
    ;;
  MINGW*|MSYS*|CYGWIN*)
    OS="windows"
    INSTALL_DIR="${LOCALAPPDATA:-$HOME/AppData/Local}/wede"
    ;;
  *)
    echo "Error: Unsupported operating system: $OS"
    exit 1
    ;;
esac

# Detect architecture
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64)  ARCH="amd64" ;;
  aarch64) ARCH="arm64" ;;
  arm64)   ARCH="arm64" ;;
  *)
    echo "Error: Unsupported architecture: $ARCH"
    exit 1
    ;;
esac

echo "  OS:   $OS"
echo "  Arch: $ARCH"
echo ""

# Get latest release tag
LATEST=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | sed -E 's/.*"tag_name": *"([^"]+)".*/\1/')

if [ -z "$LATEST" ]; then
  echo "Error: Could not determine the latest release."
  exit 1
fi

echo "  Version: $LATEST"

# Build download URL
DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${LATEST}/${BINARY}-${OS}-${ARCH}"

echo "  Downloading from: $DOWNLOAD_URL"
echo ""

# Download to temp file
TMP_DIR=$(mktemp -d)
TMP_FILE="${TMP_DIR}/${BINARY}"
trap 'rm -rf "$TMP_DIR"' EXIT

curl -fsSL -o "$TMP_FILE" "$DOWNLOAD_URL"
chmod +x "$TMP_FILE"

# Install binary
mkdir -p "$INSTALL_DIR"
mv "$TMP_FILE" "${INSTALL_DIR}/${BINARY}"
echo "  Installed to ${INSTALL_DIR}/${BINARY}"

# Check if install dir is in PATH
if ! echo "$PATH" | tr ':' '\n' | grep -q "^${INSTALL_DIR}$"; then
  echo ""
  echo "  Warning: ${INSTALL_DIR} is not in your PATH."
  echo "  Run this to add it:"
  echo ""
  case "$OS" in
    darwin)
      echo "    echo 'export PATH=\"\$HOME/.local/bin:\$PATH\"' >> ~/.zshrc && source ~/.zshrc"
      ;;
    linux)
      echo "    echo 'export PATH=\"\$HOME/.local/bin:\$PATH\"' >> ~/.bashrc && source ~/.bashrc"
      ;;
    windows)
      echo "    setx PATH \"%PATH%;${INSTALL_DIR}\""
      ;;
  esac
fi

# Create default config
CONFIG_DIR="${HOME}/.config/wede"
CONFIG_FILE="${CONFIG_DIR}/wede.config.json"
mkdir -p "$CONFIG_DIR"

DEFAULT_PASSWORD=$(LC_ALL=C tr -dc 'A-Za-z0-9' < /dev/urandom | head -c 16 || true)

if [ -f "$CONFIG_FILE" ]; then
  echo ""
  echo "  Config already exists at ${CONFIG_FILE}"
  echo "  Skipping config creation."
else
  cat > "$CONFIG_FILE" <<CONF
{
  "password": "${DEFAULT_PASSWORD}",
  "port": "9090"
}
CONF
  echo ""
  echo "  Config created at: ${CONFIG_FILE}"
  echo ""
  echo "  ┌─────────────────────────────────────────────┐"
  echo "  │  Admin password: ${DEFAULT_PASSWORD}         │"
  echo "  │  Port:           9090                        │"
  echo "  │                                              │"
  echo "  │  To change, edit: ${CONFIG_FILE}  │"
  echo "  └─────────────────────────────────────────────┘"
fi

echo ""
echo "  Done! Run 'wede' to start."
echo ""
echo "  Quick start:"
echo "    cd /path/to/your/project"
echo "    wede ."
echo "    open http://localhost:9090"
echo ""
