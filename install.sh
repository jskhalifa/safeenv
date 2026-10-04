#!/bin/sh
set -eu

go build -o safeenv .

if install -m 755 safeenv /usr/local/bin/safeenv 2>/dev/null; then
  echo "installed: /usr/local/bin/safeenv"
else
  sudo install -m 755 safeenv /usr/local/bin/safeenv
  echo "installed: /usr/local/bin/safeenv"
fi
