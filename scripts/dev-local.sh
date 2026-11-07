#!/usr/bin/env bash
# Local-only version of dev-stack.sh: everything is reached through localhost
# on this computer and Tailscale is never touched.
#
#   ./scripts/dev-local.sh              start and prepare everything
#   ./scripts/dev-local.sh clean        ERASE everything, then recreate it
#   ./scripts/dev-local.sh clean --yes  the same, without asking for confirmation
#   ./scripts/dev-local.sh --new-pin    also generate a new room PIN
#
# Open Element Web at http://localhost:8081 (localhost counts as a secure
# context, so plain http works). A phone cannot reach it: use dev-stack.sh for that.
# It takes the same options as dev-stack.sh and shares its state and containers.
set -euo pipefail

exec "$(dirname "${BASH_SOURCE[0]}")/dev-stack.sh" --no-tailscale "$@"
