#!/usr/bin/env bash
# Faz 1 — basit migration uygulayıcı.
# Henüz schema_migrations tablosu kullanılmaz; yalnızca SQL dosyalarını
# psql üzerinden sıralı çalıştırır. Faz 2'de checksum + state tablosuyla
# değiştirilecek.
set -euo pipefail

: "${DATABASE_URL:?DATABASE_URL ortam değişkeni gerekli}"

cd "$(dirname "$0")/.."

for f in $(ls migrations/*.sql | sort); do
  echo "[migrate] $f"
  psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f "$f"
done

echo "[migrate] tamamlandı"
