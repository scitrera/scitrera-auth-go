#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-only
"""Sync build metadata not covered by repo-tools' manifest strategies.

The version argument comes from repo-tools sync-versions --print-version.
Use make versions / make check-metadata rather than editing these mirrors.
"""
import argparse
import json
import re
from pathlib import Path

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--check', action='store_true')
parser.add_argument('version')
args = parser.parse_args()
if not re.fullmatch(r'\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?', args.version):
    parser.error('expected a semantic version from versions.yaml')
root = Path(__file__).resolve().parent.parent
updates = {}
for filename, pattern, replacement in [
    ('Dockerfile', r'^ARG VERSION=\S+$', f'ARG VERSION={args.version}'),
    ('deploy/compose.yaml', r'^    image: scitrera-auth-go:\S+$', f'    image: scitrera-auth-go:{args.version}'),
]:
    old = (root / filename).read_text()
    new, count = re.subn(pattern, replacement, old, flags=re.M)
    if count != 1:
        parser.error(f'{filename}: expected exactly one version declaration')
    updates[filename] = new

lock = json.loads((root / 'web/package-lock.json').read_text())
lock['version'] = args.version
lock['packages']['']['version'] = args.version
updates['web/package-lock.json'] = json.dumps(lock, indent=2, ensure_ascii=False) + '\n'

drift = [name for name, content in updates.items() if (root / name).read_text() != content]
if args.check and drift:
    parser.exit(1, 'Build metadata drift; run make versions: ' + ', '.join(drift) + '\n')
for name in drift:
    (root / name).write_text(updates[name])
print(f'Build metadata {args.version}: {"updated" if drift else "OK"}')
