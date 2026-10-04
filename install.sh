#!/bin/sh
set -eu

go build -o safeenv .

if install -m 755 safeenv /usr/local/bin/safeenv 2>/dev/null; then
  :
else
  sudo install -m 755 safeenv /usr/local/bin/safeenv
fi

echo "installed: /usr/local/bin/safeenv"
