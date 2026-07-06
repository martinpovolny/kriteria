#!/usr/bin/env bash
set -euo pipefail

DEPLOY_DIR="/opt/kriteria"
SERVICE="kriteria"
NTFY="$HOME/dotfiles/bin/ntfy.sh"
COMMIT="${1:-unknown}"

notify() {
    [ -x "$NTFY" ] && "$NTFY" "$@" || true
}

trap 'notify --alert "Kriteria deploy FAILED — commit $COMMIT"' ERR

mv "$DEPLOY_DIR/kriteria.new" "$DEPLOY_DIR/kriteria"
sudo systemctl restart "$SERVICE"

notify --alert "Kriteria deployed — commit $COMMIT"
