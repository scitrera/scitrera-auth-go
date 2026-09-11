#!/usr/bin/env python3
# SPDX-License-Identifier: AGPL-3.0-only
"""Validate release tags and expose versions.yaml settings to GitHub Actions."""
import os
from pathlib import Path

from scitrera_repo_tools.version_sync.config import load_config


def release_outputs(config, event, ref_type, ref_name):
    version = config.project_versions['scitrera-auth-go']
    if ref_type == 'tag' and ref_name != f'v{version}':
        raise ValueError(f'Tag {ref_name!r} must match versions.yaml (v{version})')
    publish = event == 'push' and ref_type == 'tag' and config.ci.github_release
    return {
        'version': version,
        'publish': str(publish).lower(),
        'prerelease': str('-' in version.split('+', 1)[0]).lower(),
    }


def main():
    config = load_config(Path(__file__).resolve().parent.parent / 'versions.yaml')
    try:
        outputs = release_outputs(config, os.getenv('GITHUB_EVENT_NAME'),
                                  os.getenv('GITHUB_REF_TYPE'), os.getenv('GITHUB_REF_NAME'))
    except ValueError as exc:
        raise SystemExit(str(exc)) from exc
    text = ''.join(f'{name}={value}\n' for name, value in outputs.items())
    print(text, end='')
    if output := os.getenv('GITHUB_OUTPUT'):
        with Path(output).open('a') as file:
            file.write(text)


if __name__ == '__main__':
    main()
