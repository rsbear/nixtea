#!/usr/bin/env bash
set -e

check_nix() {
  if ! command -v nix &> /dev/null; then
    echo "Error: nix is required but not installed"
    echo "Please visit https://determinate.systems/posts/determinate-nix-installer"
    exit 1
  fi
}

check_root() {
  if [ "$EUID" -ne 0 ]; then
    echo "Error: Please run as root"
    echo "Try: sudo bash install.sh"
    exit 1
  fi
}

setup_directories() {
  mkdir -p /etc/nixtea/ssh
  chmod 755 /etc/nixtea
  chmod 700 /etc/nixtea/ssh
}

install_binary() {
  echo "Installing nixtea via nix..."
  pkg_path=$(nix build github:rsbear/nixtea --no-link --print-out-paths)
  
  if [ -z "$pkg_path" ]; then
    echo "Error: Failed to build nixtea package"
    exit 1
  }

  ln -sf "$pkg_path/bin/nixtea" /usr/local/bin/nixtea
}

main() {
  echo "Installing nixtea..."
  check_root
  check_nix
  setup_directories
  install_binary
  echo "Installation complete! Run 'nixtea' to start."
}

main
