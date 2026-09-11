# SPDX-License-Identifier: AGPL-3.0-only
"""Exercise actual Make/source checks without npm installation or Go compilation."""
import shutil
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

REPO = Path(__file__).resolve().parent.parent


class ReleaseArchiveTest(unittest.TestCase):
    def setUp(self):
        self.directory = tempfile.TemporaryDirectory()
        self.addCleanup(self.directory.cleanup)
        self.root = Path(self.directory.name)
        for name in ['scripts/source.mjs', 'scripts/check-release.py', 'go.mod',
                     'versions.yaml', 'Dockerfile', 'web/package.json', 'LICENSE',
                     'NOTICE', 'THIRD_PARTY_NOTICES.md', 'LICENSES/Apache-2.0.txt',
                     'LICENSES/shadcn-ui-MIT.txt', 'docs/provenance.md']:
            target = self.root / name
            target.parent.mkdir(parents=True, exist_ok=True)
            shutil.copyfile(REPO / name, target)
        # Keep the real Make dependency graph and release checker. Replace only
        # unrelated metadata tools, Vite compilation, and the final Go compiler.
        (self.root / 'test-targets.mk').write_text(
            'check-metadata:\n\t$(PYTHON) scripts/check-release.py\n'
            'web:\n\tnode scripts/source.mjs\n')
        compiler = self.root / 'compiler'
        compiler.write_text('#!/bin/sh\nprintf compiled > compiler-ran\n')
        compiler.chmod(0o755)

    def make(self, target):
        return subprocess.run(
            ['make', '-f', str(REPO / 'Makefile'), '-f', 'test-targets.mk', target,
             f'PYTHON={sys.executable}', f'GO={self.root / "compiler"}'],
            cwd=self.root, capture_output=True, text=True)

    def test_fresh_forbidden_archive_blocks_build_and_packaging(self):
        for target in ['build', 'artifacts', 'source', 'container']:
            with self.subTest(target=target):
                archive = self.root / 'internal/adminui/dist/source.tar.gz'
                archive.unlink(missing_ok=True)
                (self.root / 'docs/review-fixture.pem').write_text('synthetic test fixture\n')
                result = self.make(target)
                self.assertNotEqual(result.returncode, 0, result.stdout)
                self.assertIn('Private archive entry: docs/review-fixture.pem', result.stderr)
                self.assertFalse((self.root / 'compiler-ran').exists())

    def test_valid_old_archive_does_not_bypass_fresh_check(self):
        result = self.make('source')
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        (self.root / 'docs/review-fixture.pem').write_text('synthetic test fixture\n')
        result = self.make('build')
        self.assertNotEqual(result.returncode, 0, result.stdout)
        self.assertIn('Private archive entry: docs/review-fixture.pem', result.stderr)
        self.assertFalse((self.root / 'compiler-ran').exists())

    def test_valid_archive_allows_compilation(self):
        result = self.make('build')
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertTrue((self.root / 'compiler-ran').is_file())

    def test_required_archive_cannot_be_silently_absent(self):
        result = subprocess.run(
            [sys.executable, 'scripts/check-release.py', '--require-archive'],
            cwd=self.root, capture_output=True, text=True)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('Missing generated corresponding-source archive', result.stderr)


if __name__ == '__main__':
    unittest.main()
