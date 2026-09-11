# SPDX-License-Identifier: AGPL-3.0-only
"""Release graph and native GHCR publication contract checks."""
from pathlib import Path
import unittest

import yaml
from scitrera_repo_tools.ci_gen_gha.templates import build_build_docker
from scitrera_repo_tools.version_sync.config import load_config

from container_workflow import render_workflow

ROOT = Path(__file__).resolve().parent.parent


class ContainerWorkflowTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.config = load_config(ROOT / 'versions.yaml')
        cls.rendered = render_workflow(cls.config)
        cls.workflow = yaml.safe_load(cls.rendered)

    def test_only_callable_and_preserves_repo_tools_jobs(self):
        # PyYAML's YAML 1.1 loader treats the unquoted GitHub 'on' key as True.
        self.assertEqual(self.workflow[True], {'workflow_call': None})
        generated = yaml.safe_load(build_build_docker(self.config, self.config.ci))
        self.assertEqual(self.workflow['jobs'], generated['jobs'])
        self.assertEqual((ROOT / '.github/workflows/publish-container.yml').read_text(), self.rendered)

    def test_native_builds_push_digests_and_merge_both_platforms(self):
        jobs = self.workflow['jobs']
        self.assertNotIn('qemu', self.rendered.lower())
        image = 'ghcr.io/scitrera/scitrera-auth-go'
        build_ids = []
        for arch, runner in [('amd64', 'ubuntu-latest'), ('arm64', 'ubuntu-24.04-arm')]:
            job_id = f'build-scitrera-auth-go-linux-{arch}'
            build_ids.append(job_id)
            job = jobs[job_id]
            self.assertEqual(job['runs-on'], runner)
            self.assertEqual(job['permissions']['packages'], 'write')
            build = next(step for step in job['steps'] if step.get('id') == 'build')['with']
            self.assertEqual(build['platforms'], f'linux/{arch}')
            self.assertIn(f'name={image},push-by-digest=true', build['outputs'])
            self.assertIn('push=true', build['outputs'])
            self.assertIn('VERSION=${{ steps.imgver.outputs.version }}', build['build-args'])
            self.assertIn('REVISION=${{ github.sha }}', build['build-args'])
        merge = jobs['merge-scitrera-auth-go']
        self.assertEqual(set(merge['needs']), set(build_ids))
        command = merge['steps'][-1]['run']
        for job_id in build_ids:
            self.assertIn(f'{image}@${{{{ needs.{job_id}.outputs.digest }}}}', command)

    def test_publication_waits_for_release_checks_and_skips_manual_runs(self):
        release = yaml.safe_load((ROOT / '.github/workflows/release.yml').read_text())
        jobs = release['jobs']
        publish = jobs['container']
        self.assertEqual(publish['uses'], './.github/workflows/publish-container.yml')
        self.assertEqual(publish['if'], "github.event_name == 'push' && github.ref_type == 'tag'")
        self.assertEqual(set(publish['needs']), {'metadata', 'artifacts'})
        self.assertEqual(set(jobs['artifacts']['needs']), {'go', 'npm', 'integration'})
        for gate in ['go', 'npm', 'integration']:
            self.assertEqual(jobs[gate]['needs'], 'metadata')
        self.assertEqual(publish['permissions']['packages'], 'write')
        self.assertIn('container', jobs['github-release']['needs'])


if __name__ == '__main__':
    unittest.main()
