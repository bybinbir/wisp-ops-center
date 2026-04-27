#!/usr/bin/env bash
# wisp-ops-center web (Next.js) geliştirme modunda çalıştırır.
set -euo pipefail

cd "$(dirname "$0")/../apps/web"

if [ ! -d node_modules ]; then
  echo "[wisp-ops] node_modules yok, npm install çalıştırılıyor..."
  npm install
fi

NEXT_PUBLIC_API_BASE=${NEXT_PUBLIC_API_BASE:-http://localhost:8080} npm run dev
