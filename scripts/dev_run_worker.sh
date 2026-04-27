#!/usr/bin/env bash
# wisp-ops-center worker'ı geliştirme modunda çalıştırır.
set -euo pipefail

cd "$(dirname "$0")/.."

export WISP_ENV=${WISP_ENV:-development}
export LOG_LEVEL=${LOG_LEVEL:-debug}
export LOG_FORMAT=${LOG_FORMAT:-text}

go run ./apps/worker/cmd/worker
