import importlib.util
import pathlib
import unittest

spec = importlib.util.spec_from_file_location(
    "source_gate", pathlib.Path(__file__).with_name("check-source.py")
)
gate = importlib.util.module_from_spec(spec)
spec.loader.exec_module(gate)


class SourceGateTests(unittest.TestCase):
    def test_plain_text(self):
        self.assertEqual(gate.scan(b"ordinary source and secrets.NAME references"), [])

    def test_private_key_marker(self):
        data = b"-----BEGIN " + b"PRIVATE KEY-----"
        self.assertEqual(gate.scan(data)[0], ("private key", 1))

    def test_generated_credentials(self):
        for prefix, length, label in [
            (b"ghp_", 36, "GitHub credential"),
            (b"mnet_", 32, "Mnet node credential"),
            (b"AIza", 35, "Google API credential"),
            (b"AKIA", 16, "AWS access key"),
        ]:
            with self.subTest(label=label):
                self.assertEqual(gate.scan(b"first line\n" + prefix + b"X" * length)[0], (label, 2))

    def test_unicode_dash(self):
        for code in (0x2013, 0x2014):
            self.assertEqual(gate.scan(("first\n" + chr(code)).encode()), [("forbidden dash", 2)])


if __name__ == "__main__":
    unittest.main()
