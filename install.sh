#!/bin/sh
set -eu

go build -o safeenv .
GOOS=linux GOARCH=amd64 go build -o safeenv-linux-amd64 .
GOOS=linux GOARCH=arm64 go build -o safeenv-linux-arm64 .

if install -m 755 safeenv safeenv-linux-amd64 safeenv-linux-arm64 /usr/local/bin/ 2>/dev/null; then
  :
else
  sudo install -m 755 safeenv safeenv-linux-amd64 safeenv-linux-arm64 /usr/local/bin/
fi

echo "installed: /usr/local/bin/safeenv"
echo "installed: /usr/local/bin/safeenv-linux-amd64"
echo "installed: /usr/local/bin/safeenv-linux-arm64"
