#!/usr/bin/env python3
"""Exercise CLI smoke checks without accessing a user's application state."""
import os
from pathlib import Path
import subprocess
import tempfile
import unittest


SMOKE = Path(__file__).with_name("smoke-megafloyd.sh")


class SmokeTest(unittest.TestCase):
    def run_fixture(self, body, *, name="megafloyd", dependency=False):
        with tempfile.TemporaryDirectory(prefix="megafloyd-smoke-test-") as directory:
            root = Path(directory)
            command = root / name
            command.write_text("#!/bin/sh\nset -eu\n" + body)
            command.chmod(0o755)
            if dependency:
                (root / "installed-version").write_text("MegaFloyd version 2026.10.31\n")
            env = dict(os.environ)
            env.pop("MEGAFLOYD_BIN", None)
            return subprocess.run(["bash", str(SMOKE), str(command)], env=env,
                                  capture_output=True, text=True, timeout=15)

    def test_installed_launcher_retains_its_relative_files(self):
        result = self.run_fixture('cat "$(dirname -- "$0")/installed-version"\n', dependency=True)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn("smoke: OK (MegaFloyd version 2026.10.31)", result.stdout)

    def test_renamed_artifact_gets_canonical_identity_and_legacy_brand_works(self):
        result = self.run_fixture('[ "$(basename -- "$0")" = megafloyd ]\n'
                                  'echo "megafloyd version 2026.10.31"\n', name="host-build")
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def test_bad_identity_empty_output_and_failed_commands_are_rejected(self):
        cases = [('echo "megafloyd-other version 1"\n', 1),
                 ('exit 0\n', 1),
                 ('echo "MegaFloyd version 2026.10.31"\n'
                  '[ "${1:-}" != --help ] || exit 37\n', 37)]
        for body, expected in cases:
            with self.subTest(body=body):
                result = self.run_fixture(body)
                self.assertEqual(result.returncode, expected, result.stdout + result.stderr)
                self.assertIn("FAIL:", result.stderr)


if __name__ == "__main__":
    unittest.main()
