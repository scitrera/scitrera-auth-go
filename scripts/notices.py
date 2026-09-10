#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-only
"""Collect dependency license files from resolved Go modules and installed npm packages."""
import json
import os
import shutil
import subprocess
from pathlib import Path

root = Path(__file__).resolve().parent.parent
target = root / 'LICENSES/third-party'
if target.exists(): shutil.rmtree(target)  # Regenerate only this tool-owned inventory.
target.mkdir(parents=True, exist_ok=True)
decoder = json.JSONDecoder()
raw = subprocess.check_output([os.environ.get('GO', 'go'), 'list', '-buildvcs=false', '-deps', '-json', './cmd/scitrera-auth-proxy'], cwd=root, text=True)
modules = {}
while raw.strip():
    obj, end = decoder.raw_decode(raw.lstrip())
    raw = raw.lstrip()[end:]
    mod = obj.get('Module', {})
    if mod.get('Version'):
        modules[mod['Path']] = (mod['Version'], Path(mod['Dir']))
for relative in json.loads((root / 'web/package-lock.json').read_text())['packages']:
    if not relative.startswith('node_modules/'): continue
    package = root / 'web' / relative / 'package.json'
    if not package.exists(): continue  # Optional packages for another OS/architecture.
    info = json.loads(package.read_text())
    if info.get('name') and info.get('version'):
        modules['npm:' + info['name']] = (info['version'], package.parent)
rows = []
missing = []
for name, (version, directory) in sorted(modules.items()):
    paths = []
    # Platform binary packages distribute the parent project's license.
    for prefix, parent in [('npm:@esbuild/', 'esbuild'), ('npm:@rollup/rollup-', 'rollup'), ('npm:@tailwindcss/oxide-', '@tailwindcss/oxide'), ('npm:lightningcss-', 'lightningcss')]:
        if name.startswith(prefix): directory = root / 'web/node_modules' / parent
    for file in sorted(directory.iterdir()):
        if file.is_file() and file.suffix not in {'.go','.js','.ts','.c','.h','.json'} and file.name.upper().startswith(('LICENSE', 'LICENCE', 'NOTICE', 'COPYING', 'OFL', 'PATENTS')):
            dest = target / (name.replace('/', '_').replace(':', '_') + '@' + version) / file.name
            dest.parent.mkdir(parents=True, exist_ok=True)
            shutil.copyfile(file, dest)
            paths.append(f'[{file.name}]({dest.relative_to(root)})')
    if not paths and name.startswith('npm:@radix-ui/'):
        file = root / 'LICENSES/radix-ui-MIT.txt'
        if file.exists(): paths.append('[MIT](LICENSES/radix-ui-MIT.txt)')
    if not paths: missing.append(f'{name}@{version}')
    rows.append(f'| `{name}` | `{version}` | {", ".join(paths)} |')
if missing: raise RuntimeError('Missing upstream license texts: ' + ', '.join(missing))
goroot = Path(subprocess.check_output([os.environ.get('GO', 'go'), 'env', 'GOROOT'], text=True).strip())
for name in ['LICENSE', 'PATENTS']:
    shutil.copyfile(goroot / name, target / ('Go-' + name))
(root / 'THIRD_PARTY_NOTICES.md').write_text('''# Third-party notices

Scitrera Auth first-party source is AGPL-3.0-only. Dependencies retain the licenses
below. This inventory includes linked Go modules and installed frontend build/test
dependencies. Public locked versions/checksums are in go.mod/go.sum and the web
lockfile. Refresh with `python3 scripts/notices.py` after installing dependencies.

Aether's Apache license also covers adapted definitions in migrations/001_proxy.sql.
The Scitrera UI adaptations are described in NOTICE; the underlying shadcn/ui
conventions retain the [MIT notice](LICENSES/shadcn-ui-MIT.txt).
The Go runtime retains its [license](LICENSES/third-party/Go-LICENSE) and
[patent notice](LICENSES/third-party/Go-PATENTS). Container base-image packages
retain their own licenses; consult the pinned images' system documentation.

| Dependency | Version | License/notice files |
|---|---|---|
''' + '\n'.join(rows) + '\n')
print(f'Collected notices for {len(rows)} dependencies.')
