#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"

VERSION="${1:-$(git rev-parse --short HEAD)}"
IMAGE="logos-api:${VERSION}"

echo "=== Building ${IMAGE} ==="
docker build -t "${IMAGE}" .

echo "=== Importing image into k3s ==="
docker save "${IMAGE}" | sudo k3s ctr images import -

echo "=== Done! ==="
echo "Image: ${IMAGE}"
echo ""
echo "Update the deployment manifest tag:"
echo "  image: ${IMAGE}"
sudo k3s ctr images ls | grep logos-api
