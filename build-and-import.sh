#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"

echo "=== Building logos-api ==="
docker build -t logos-api:latest .

echo "=== Importing image into k3s ==="
docker save logos-api:latest | sudo k3s ctr images import -

echo "=== Done! ==="
echo "Imported images:"
sudo k3s ctr images ls | grep logos
