#!/usr/bin/env bash
# Renders docs/screenshots/ out of the example, by hand after a visible change.
# Needs chromium, imagemagick, python-pyte and python-yaml.
#
#   docs/screenshots.sh
set -euo pipefail
cd "$(dirname "$0")/.."

make example
python3 docs/screenshots.py --product example
