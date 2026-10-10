#!/usr/bin/env bash
# Renders docs/banner.png out of two screenshots, so after docs/screenshots.sh.
#
#   docs/banner.sh
set -euo pipefail
cd "$(dirname "$0")/.."

python3 docs/banner.py \
    --product example/oak.yaml \
    --logo docs/logo.svg \
    --card docs/screenshots/report.png \
    --card docs/screenshots/run.png \
    --tagline "Build your own Arch Linux distribution. The installer is already written." \
    --cell 17
