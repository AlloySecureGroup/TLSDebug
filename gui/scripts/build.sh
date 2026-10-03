#!/usr/bin/env sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
GUI_DIR=$(dirname "$SCRIPT_DIR")
REPO_DIR=$(dirname "$GUI_DIR")
TARGET_OS=$(go env GOOS)
PROXY_NAME=tlsproxy

if [ "$TARGET_OS" = "windows" ]; then
  PROXY_NAME=tlsproxy.exe
fi

cd "$GUI_DIR"
go run github.com/wailsapp/wails/v2/cmd/wails@v2.16.0 build "$@"

if [ "$TARGET_OS" = "darwin" ]; then
  DESTINATION="$GUI_DIR/build/bin/TLSDebug.app/Contents/MacOS/$PROXY_NAME"
else
  DESTINATION="$GUI_DIR/build/bin/$PROXY_NAME"
fi

go build -o "$DESTINATION" "$REPO_DIR/tlsproxy.go"
echo "Built TLSDebug desktop application and proxy in $GUI_DIR/build/bin"
