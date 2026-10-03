#!/usr/bin/env sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
GUI_DIR=$(dirname "$SCRIPT_DIR")
REPO_DIR=$(dirname "$GUI_DIR")

mkdir -p "$GUI_DIR/bin"
PROXY_NAME=tlsproxy
if [ "$(go env GOOS)" = "windows" ]; then
  PROXY_NAME=tlsproxy.exe
fi

go build -o "$GUI_DIR/bin/$PROXY_NAME" "$REPO_DIR/tlsproxy.go"

cd "$GUI_DIR"
TLSDEBUG_PROXY_BIN="$GUI_DIR/bin/$PROXY_NAME" \
  go run github.com/wailsapp/wails/v2/cmd/wails@v2.16.0 dev
