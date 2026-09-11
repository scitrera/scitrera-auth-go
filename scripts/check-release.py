#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-only
"""Check the standalone release's source and version boundaries."""
import argparse
import json
import re
import tarfile
from pathlib import Path

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--require-archive', action='store_true',
                    help='Fail if the generated corresponding-source archive is missing')
args = parser.parse_args()
root = Path(__file__).resolve().parent.parent
version = json.loads((root / 'web/package.json').read_text())['version']
assert (root / 'versions.yaml').is_file()
assert not (root / 'VERSION').exists(), 'versions.yaml is the canonical version manifest'
assert f'go 1.25.14\n' in (root / 'go.mod').read_text()
assert not re.search(r'^replace\b', (root / 'go.mod').read_text(), re.M)
assert 'golang:1.25.14-bookworm@sha256:' in (root / 'Dockerfile').read_text()
assert f'ARG VERSION={version}' in (root / 'Dockerfile').read_text()
for directory in ['cmd', 'internal', 'migrations', 'web/src', 'deploy']:
    for path in (root / directory).rglob('*'):
        if not path.is_file() or path.suffix not in {'.go', '.sql', '.tsx', '.ts', '.yaml', '.html', '.tmpl'}:
            continue
        if 'dist' in path.parts or path.name == '.env':
            continue
        text = path.read_text()
        for forbidden in ['/home/drew/', 'scitrera-aether3-go/oss-repo', 'beeaef61-22c3-4e35-a140-05d2b6615d37', 'BEGIN PRIVATE KEY']:
            assert forbidden not in text, f'{path}: forbidden release input {forbidden}'
for required in ['LICENSE', 'NOTICE', 'THIRD_PARTY_NOTICES.md', 'LICENSES/Apache-2.0.txt', 'LICENSES/shadcn-ui-MIT.txt', 'docs/provenance.md']:
    assert (root / required).is_file(), f'Missing {required}'
archive = root / 'internal/adminui/dist/source.tar.gz'
if args.require_archive and not archive.is_file():
    parser.exit(1, 'Missing generated corresponding-source archive: internal/adminui/dist/source.tar.gz\n')
if archive.exists():
    with tarfile.open(archive, 'r:gz') as source:
        for member in source.getmembers():
            path = Path(member.name)
            assert member.isfile() and not path.is_absolute() and '..' not in path.parts, f'Unsafe archive entry: {member.name}'
            assert not set(path.parts) & {'.git', '.local', '.omc', '.omo', '.codex', '.agents', 'node_modules', 'test-results', 'playwright-report', '__pycache__'}, f'Incidental archive entry: {member.name}'
            assert not path.name.startswith('.env') and path.suffix not in {'.env', '.pem', '.key', '.log'}, f'Private archive entry: {member.name}'
print(f'Release boundaries and version {version}: PASS')
