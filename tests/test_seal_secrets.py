import importlib.util
import json
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location("seal_secrets", Path(__file__).resolve().parents[1] / "deploy/seal-secrets.py")
seal = importlib.util.module_from_spec(spec)
spec.loader.exec_module(seal)


class SealSecretsTests(unittest.TestCase):
    def setUp(self):
        self.directory = tempfile.TemporaryDirectory()
        self.addCleanup(self.directory.cleanup)
        self.root = Path(self.directory.name)
        self.source = self.root / "secrets.env"

    def test_values_are_data_not_shell(self):
        value = '$(touch /must-never-run); `whoami` # = : "literal"'
        self.source.write_text("# comment\nDATABASE_URL=\nDATABASE_PASSWORD='" + value + "'\n")
        self.assertEqual(seal.read_values(self.source), {"DATABASE_PASSWORD": value})

    def test_rejects_unknown_duplicate_and_malformed_entries(self):
        for text in ["TUNNEL_TOKEN=no", "DATABASE_USER=a\nDATABASE_USER=b", "source other.env", 'DATABASE_PASSWORD="unfinished']:
            with self.subTest(text=text):
                self.source.write_text(text)
                with self.assertRaises(ValueError):
                    seal.read_values(self.source)

    def test_empty_source_preserves_existing_secret(self):
        self.source.write_text("DATABASE_PASSWORD=\n")
        target = self.root / "sealed-secret.json"
        target.write_text("previous")
        with patch.object(seal.subprocess, "run") as run:
            with self.assertRaises(ValueError):
                seal.seal(self.source, self.root / "cert", self.root)
            run.assert_not_called()
        self.assertEqual(target.read_text(), "previous")

    def test_plaintext_only_reaches_stdin_and_encrypted_output_is_saved(self):
        self.source.write_text("DATABASE_PASSWORD=test-secret-value\n")
        output = {"kind": "SealedSecret", "spec": {"encryptedData": {"DATABASE_PASSWORD": "encrypted-value"}}}
        with patch.object(seal.subprocess, "run", return_value=subprocess.CompletedProcess([], 0, json.dumps(output), "")) as run:
            seal.seal(self.source, self.root / "cert", self.root / "out")
        self.assertNotIn("test-secret-value", str(run.call_args.args))
        plain = json.loads(run.call_args.kwargs["input"])
        self.assertEqual(plain["metadata"]["namespace"], "maceclubheemskerk")
        self.assertEqual(plain["stringData"]["DATABASE_PASSWORD"], "test-secret-value")
        saved = (self.root / "out/sealed-secret.json").read_text()
        self.assertNotIn("test-secret-value", saved)
        self.assertIn("encrypted-value", saved)
        self.assertIn("sealed-secret.json", (self.root / "out/kustomization.yaml").read_text())

    def test_tool_failure_does_not_overwrite_or_echo_secret(self):
        self.source.write_text("DATABASE_PASSWORD=test-secret-value\n")
        target = self.root / "sealed-secret.json"
        target.write_text("previous")
        with patch.object(seal.subprocess, "run", return_value=subprocess.CompletedProcess([], 1, "", "test-secret-value")):
            with self.assertRaises(ValueError) as error:
                seal.seal(self.source, self.root / "cert", self.root)
        self.assertNotIn("test-secret-value", str(error.exception))
        self.assertEqual(target.read_text(), "previous")


if __name__ == "__main__":
    unittest.main()
