#!/usr/bin/env bash
# wisp-ops-center API'yi geliştirme modunda çalıştırır.
# Kullanım:
#   bash scripts/dev_run_api.sh
set -euo pipefail

cd "$(dirname "$0")/.."

export WISP_ENV=${WISP_ENV:-development}
export WISP_HTTP_ADDR=${WISP_HTTP_ADDR:-:8080}
export LOG_LEVEL=${LOG_LEVEL:-debug}
export LOG_FORMAT=${LOG_FORMAT:-text}

go run ./apps/api/cmd/api
