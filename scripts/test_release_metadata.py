# SPDX-License-Identifier: AGPL-3.0-only
import tempfile
import unittest
from pathlib import Path

from scitrera_repo_tools.version_sync.config import load_config
from release_metadata import release_outputs


class ReleaseMetadataTest(unittest.TestCase):
    def config(self, version='0.1.1', enabled=True):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / 'versions.yaml'
            path.write_text(f'scitrera-auth-go: {version}\nci:\n  github_release: {str(enabled).lower()}\n')
            return load_config(path)

    def test_matching_tag_publishes_stable_release(self):
        self.assertEqual(release_outputs(self.config(), 'push', 'tag', 'v0.1.1'),
                         {'version': '0.1.1', 'publish': 'true', 'prerelease': 'false'})

    def test_disabled_publication_still_prepares_artifacts(self):
        self.assertEqual(release_outputs(self.config(enabled=False), 'push', 'tag', 'v0.1.1')['publish'], 'false')

    def test_manual_and_branch_runs_never_publish(self):
        for event, ref_type, name in [('workflow_dispatch', 'branch', 'main'),
                                     ('workflow_dispatch', 'tag', 'v0.1.1'),
                                     ('push', 'branch', 'main')]:
            with self.subTest(event=event, ref_type=ref_type):
                self.assertEqual(release_outputs(self.config(), event, ref_type, name)['publish'], 'false')

    def test_mismatched_tags_fail_before_publication(self):
        for tag in ['v0.1.0', 'v0.1.1-rc.1', '0.1.1']:
            with self.subTest(tag=tag), self.assertRaisesRegex(ValueError, 'must match versions.yaml'):
                release_outputs(self.config(), 'push', 'tag', tag)

    def test_prerelease_tag_is_marked_as_prerelease(self):
        outputs = release_outputs(self.config('0.2.0-rc.1'), 'push', 'tag', 'v0.2.0-rc.1')
        self.assertEqual(outputs['publish'], 'true')
        self.assertEqual(outputs['prerelease'], 'true')


if __name__ == '__main__':
    unittest.main()
