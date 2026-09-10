#!/bin/sh
# SPDX-License-Identifier: AGPL-3.0-only
set -eu
cd "$(dirname "$0")/.."
umask 077
mkdir -p .local
if [ ! -f deploy/.env ]; then
  python3 - <<'PY'
import os,secrets
from pathlib import Path
with Path('deploy/.env').open('x') as f:
    f.write(f'POSTGRES_PASSWORD={secrets.token_hex(24)}\nAUTH_PROXY_TOKEN_HMAC_KEY={secrets.token_hex(32)}\nAUTH_LOCAL_UID={os.getuid()}\nAUTH_LOCAL_GID={os.getgid()}\n')
PY
fi
docker compose --env-file deploy/.env -f deploy/compose.yaml build auth
docker compose --env-file deploy/.env -f deploy/compose.yaml up -d postgres
docker compose --env-file deploy/.env -f deploy/compose.yaml run --rm auth migrate
if [ ! -f .local/operators.json ]; then
  # Use a separate writable bootstrap mount; the service mount stays read-only.
  docker compose --env-file deploy/.env -f deploy/compose.yaml run --rm -v "$(pwd)/.local:/run/bootstrap:rw" auth bootstrap --token-file /run/bootstrap/operators.json --operator operator
fi
docker compose --env-file deploy/.env -f deploy/compose.yaml up -d auth
printf '%s\n' 'Open http://127.0.0.1:8082/admin/ and use the private .local/operators.json credentials.'
