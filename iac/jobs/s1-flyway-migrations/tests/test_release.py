import os
from pathlib import Path
import subprocess
import tempfile
import unittest

import yaml

ROOT = Path(__file__).resolve().parents[4]
MODULE = ROOT / 'iac/jobs/s1-flyway-migrations'
IMAGE = 'ghcr.io/umu-art/s1-flyway-migrations@sha256:' + 'a' * 64


def load(path):
    return yaml.load(path.read_text(), Loader=yaml.BaseLoader)


class MigrationJob(unittest.TestCase):
    def run_job(self, scenario, namespace='system1-stage'):
        self.assertTrue((MODULE / 'run.sh').is_file(), 'missing migration runner')
        with tempfile.TemporaryDirectory() as directory:
            tmp = Path(directory)
            kubectl = tmp / 'kubectl'
            kubectl.write_text('''#!/usr/bin/env bash
set -euo pipefail
printf '%s\\n' "$*" >> "$CALLS"
case "$3" in
  apply|create)
    cat >> "$MANIFESTS"
    if [[ "$3" == create ]]; then printf 'job.batch/s1-flyway-migrations-abcde\\n'; fi
    ;;
  wait) [[ "$SCENARIO" != no-secret ]] ;;
  get)
    case "$SCENARIO" in
      complete) printf 'Complete=True\\n' ;;
      failed) printf 'Failed=True\\n' ;;
    esac
    ;;
  logs) printf 'Flyway log\\n' ;;
  describe|delete) : ;;
  *) exit 2 ;;
esac
''')
            kubectl.chmod(0o755)
            calls = tmp / 'calls'
            manifests = tmp / 'manifests'
            result = subprocess.run(
                ['bash', str(MODULE / 'run.sh'), namespace, IMAGE, '123-2'],
                env={**os.environ, 'PATH': f'{tmp}:{os.environ["PATH"]}',
                     'SCENARIO': scenario, 'CALLS': str(calls), 'MANIFESTS': str(manifests),
                     'FLYWAY_JOB_WAIT_SECONDS': '0' if scenario == 'timeout' else '10'},
                capture_output=True, text=True, timeout=10,
            )
            return result, calls.read_text(), manifests.read_text()

    def test_scopes_credentials_and_runs_job_after_external_secret(self):
        for namespace in ('system1-stage', 'system1'):
            with self.subTest(namespace=namespace):
                result, calls, manifests = self.run_job('complete', namespace)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertLess(calls.index('wait'), calls.index('create'))
                self.assertIn(f'--namespace {namespace} create', calls)
                self.assertIn(f'key: {namespace}/s1-flyway-migrations', manifests)
                self.assertIn(f'image: {IMAGE}', manifests)
                self.assertIn('generateName: s1-flyway-migrations-', manifests)
                self.assertIn('restartPolicy: Never', manifests)
                self.assertIn('ttlSecondsAfterFinished:', manifests)

    def test_missing_external_secret_stops_before_job(self):
        result, calls, _ = self.run_job('no-secret')
        self.assertNotEqual(result.returncode, 0)
        self.assertNotIn(' create ', calls)

    def test_failed_job_returns_failure_and_logs(self):
        result, calls, _ = self.run_job('failed')
        self.assertNotEqual(result.returncode, 0)
        self.assertIn(' logs job.batch/s1-flyway-migrations-abcde', calls)
        self.assertIn('Flyway log', result.stdout + result.stderr)

    def test_timeout_returns_failure_and_cancels_job(self):
        result, calls, _ = self.run_job('timeout')
        self.assertNotEqual(result.returncode, 0)
        self.assertIn(' logs job.batch/s1-flyway-migrations-abcde', calls)
        self.assertIn(' delete job.batch/s1-flyway-migrations-abcde', calls)


class ReleaseWorkflow(unittest.TestCase):
    def test_push_release_is_sequential_and_uses_its_own_images_and_revision(self):
        workflow = load(ROOT / '.github/workflows/entrypoint.yml')
        self.assertIn('release', workflow['jobs'], 'missing release gate')
        release = workflow['jobs']['release']
        self.assertEqual(set(release['needs']), {'build', 'build-migrations'})
        self.assertIn("github.event_name == 'push'", release['if'])
        steps = {step['name']: step for step in release['steps']}
        names = list(steps)
        self.assertLess(names.index('Apply database migrations'), names.index('Sync ArgoCD'))
        self.assertLess(names.index('Sync ArgoCD'), names.index('Restart updated Deployments'))
        self.assertIn('needs.build-migrations.outputs.image', str(steps['Apply database migrations']))
        self.assertEqual(steps['Sync ArgoCD']['with']['revision'], '${{ github.sha }}')
        self.assertEqual(workflow['concurrency']['cancel-in-progress'], 'false')
        self.assertIn('prod', workflow['concurrency']['group'])
        self.assertIn('stage', workflow['concurrency']['group'])
        self.assertNotIn('sync-iac', workflow['jobs'])
        reusable = load(ROOT / '.github/workflows/_build-module.yml')
        self.assertNotIn('deploy_to_stand', str(reusable))


if __name__ == '__main__':
    unittest.main()
