#!/usr/bin/env bash
# build.sh -- 3dsync-agent'i, surum numarasini GIT TAG'INDEN otomatik
# okuyarak derler.
#
# AMAC (urunlestirme): daha once "version" komutu sabit bir metin
# ("dev") ya da elle verilen bir -ldflags degeriydi -- surum takibi icin
# yeterli degildi. Bu script, bir git deposundaysaniz `git describe` ile
# (orn. "v0.11.0" ya da bir etiketten sonraki commit'lerde
# "v0.11.0-3-gabc1234") otomatik bir surum uretir; git deposu/etiketi
# yoksa "dev" + (varsa) kisa commit hash'ine duser.
#
# KULLANIM:
#   ./build.sh                        # yerel OS/mimari icin derler
#   GOOS=linux GOARCH=amd64 ./build.sh  # capraz-derleme
#
# Surum numarasi atmak icin:
#   git tag v0.12.0 && git push --tags   (push, bu script'in isi degil)
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

if git rev-parse --git-dir >/dev/null 2>&1; then
  VERSION="$(git describe --tags --always --dirty 2>/dev/null || echo dev)"
else
  VERSION="dev"
fi

echo "==> Surum: $VERSION"
go build -ldflags="-s -w -X main.buildVersion=$VERSION" -o 3dsync-agent ./cmd/3dsync-agent
echo "==> Derlendi: $SCRIPT_DIR/3dsync-agent"
./3dsync-agent version
