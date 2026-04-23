#!/usr/bin/env bash
set -euo pipefail

BINARY_NAME="server-logger"
INSTALL_BIN="/usr/local/bin/${BINARY_NAME}"
CONFIG_DIR="/etc/server-logger"
DATA_DIR="/var/lib/server-logger"
LOG_DIR="/var/log/server-logger"
SERVICE_PATH="/etc/systemd/system/server-logger.service"

detect_os() {
  if [[ -f /etc/os-release ]]; then
    . /etc/os-release
    echo "${ID:-unknown}"
    return
  fi
  echo "unknown"
}

ensure_root() {
  if [[ "${EUID}" -ne 0 ]]; then
    echo "Run install.sh as root."
    exit 1
  fi
}

ensure_binary() {
  if [[ -x "./server-logger" ]]; then
    cp ./server-logger "${INSTALL_BIN}"
  elif [[ -x "./build/server-logger-linux-amd64" ]]; then
    cp ./build/server-logger-linux-amd64 "${INSTALL_BIN}"
  else
    echo "Binary not found. Run 'make build' first."
    exit 1
  fi
  chmod 0755 "${INSTALL_BIN}"
}

main() {
  ensure_root
  OS_ID="$(detect_os)"
  case "${OS_ID}" in
    ubuntu|debian|centos|rhel|rocky|almalinux)
      ;;
    *)
      echo "Unsupported OS: ${OS_ID}"
      exit 1
      ;;
  esac

  mkdir -p "${CONFIG_DIR}" "${DATA_DIR}" "${LOG_DIR}"
  ensure_binary
  install -m 0644 configs/config.yaml "${CONFIG_DIR}/config.yaml"
  install -m 0644 service/server-logger.service "${SERVICE_PATH}"

  systemctl daemon-reload
  systemctl enable server-logger
  systemctl restart server-logger

  echo "Installed ${BINARY_NAME} on ${OS_ID}"
  systemctl --no-pager --full status server-logger || true
}

main "$@"
