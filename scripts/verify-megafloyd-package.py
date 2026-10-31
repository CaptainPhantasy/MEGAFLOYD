#!/usr/bin/env python3
"""Direct install, corruption, and rollback checks using the actual canonical release archive."""
import argparse
import contextlib
import importlib.util
import io
import json
import os
import pathlib
import subprocess
import tempfile


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--installer", required=True)
    parser.add_argument("--manifest", required=True)
    parser.add_argument("--workspace", required=True)
    args = parser.parse_args()
    installer = pathlib.Path(args.installer).resolve()
    manifest = pathlib.Path(args.manifest).resolve()
    release = json.loads(manifest.read_text())
    spec = importlib.util.spec_from_file_location("canonical_release", installer)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    workspace = pathlib.Path(args.workspace).resolve()
    workspace.mkdir(parents=True, exist_ok=True)
    results = []
    with tempfile.TemporaryDirectory(prefix="package-proof-", dir=workspace) as directory:
        root = pathlib.Path(directory)
        prefix = root / "installed"
        prefix.mkdir()
        sentinel = prefix / "owner-file"
        sentinel.write_text("preserve this unrelated file\n")
        with contextlib.redirect_stderr(io.StringIO()) as errors:
            code = module.install(argparse.Namespace(prefix=None))
        assert code == 2 and errors.getvalue().strip() == module.DESTINATION_ERROR
        results.append("missing destination returns the required error and exit2")
        command = ["python3", str(installer), "install", "--prefix", str(prefix), "--manifest", str(manifest)]
        completed = subprocess.run(command, text=True, capture_output=True, timeout=180)
        assert completed.returncode == 0, completed.stdout + completed.stderr
        expected_link = "releases/" + release["release_id"]
        assert os.readlink(prefix / "current") == expected_link
        assert sentinel.read_text() == "preserve this unrelated file\n"
        version = subprocess.check_output([str(prefix / "bin/megafloyd"), "--version"], text=True).strip()
        assert version in ("MegaFloyd version " + release["version"],
                           "megafloyd version " + release["version"])
        assert os.readlink(prefix / "MEGAFLOYD.app") == "current/MEGAFLOYD.app"
        results.append("actual package installs all components and launcher reports the release version")
        receipt_before = (prefix / "install-receipt.json").read_bytes()
        launcher_before = (prefix / "bin/megafloyd").read_bytes()
        bad_archive = root / "corrupt.tar.gz"
        bad_archive.write_bytes(b"deliberately truncated package")
        corrupted = subprocess.run(command + ["--archive", str(bad_archive)], text=True, capture_output=True, timeout=20)
        assert corrupted.returncode == 1 and "checksum or size mismatch" in corrupted.stderr
        assert os.readlink(prefix / "current") == expected_link
        assert (prefix / "install-receipt.json").read_bytes() == receipt_before
        assert (prefix / "bin/megafloyd").read_bytes() == launcher_before
        results.append("corrupted package cannot change current release, launcher, or receipt")
        # Use a different prior current target to prove rollback, while sharing the real verified package.
        prior = prefix / "releases/prior-release"
        prior.symlink_to(release["release_id"])
        staged_prior = prefix / ".prior-current"
        staged_prior.symlink_to("releases/prior-release")
        os.replace(staged_prior, prefix / "current")
        original_run = module.run
        reached_activation = False

        def fail_activation(arguments, **kwargs):
            nonlocal reached_activation
            if arguments[0] == str(prefix / "bin/megafloyd"):
                reached_activation = True
                raise OSError("controlled post-switch executable failure")
            return original_run(arguments, **kwargs)

        module.run = fail_activation
        install_args = argparse.Namespace(prefix=str(prefix), manifest=str(manifest), archive=None,
                                          cdn=module.CDN, check=False, verify_only=False)
        try:
            with contextlib.redirect_stdout(io.StringIO()):
                module.install(install_args)
        except OSError as error:
            assert str(error) == "controlled post-switch executable failure"
        else:
            raise AssertionError("expected the controlled activation failure")
        assert reached_activation
        assert os.readlink(prefix / "current") == "releases/prior-release"
        assert (prefix / "install-receipt.json").read_bytes() == receipt_before
        assert (prefix / "bin/megafloyd").read_bytes() == launcher_before
        assert sentinel.read_text() == "preserve this unrelated file\n"
        results.append("failure after current-link switch restores the prior target, launcher, and receipt")
        checked = subprocess.run([str(prefix / "bin/megafloyd"), "update", "--check", "--manifest", str(manifest)],
                                 text=True, capture_output=True, timeout=20)
        assert checked.returncode == 0, checked.stderr
        assert json.loads(checked.stdout)["available"] == release["release_id"]
        results.append("installed megafloyd update command uses the package installer")
    receipt = {"passed": len(results), "results": results, "release_id": release["release_id"],
               "archive_sha256": release["platforms"][module.host_target()]["sha256"], "model_calls": 0}
    (workspace / "package-verification-receipt.json").write_text(json.dumps(receipt, indent=2) + "\n")
    print(json.dumps(receipt, indent=2))


if __name__ == "__main__":
    main()
