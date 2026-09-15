#!/usr/bin/env bash
set -euo pipefail
deploy_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
exec python3 "$deploy_dir/seal-secrets.py" "$@"
