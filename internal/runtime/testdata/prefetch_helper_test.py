"""Run the actual helper against dependency shims; never contact the Hub."""
import contextlib
import io
import json
import runpy
import sys
import types
import unittest
from pathlib import Path
from unittest.mock import patch


class HelperTests(unittest.TestCase):
	def test_disabled_progress_bar_has_no_description(self):
		class DisabledTqdm:
			# tqdm intentionally omits desc on its disabled/early-return path.
			def __init__(self, **kwargs):
				self.n = 0
				self.total = kwargs.get("total", 0)

			def update(self, n=1):
				self.n += n

		calls = []
		def download(repo, **kwargs):
			calls.append((repo, kwargs.get("revision")))
			kwargs["tqdm_class"](total=100).update(25)
			return "/unused/cache"

		class Checkpoint:
			def __init__(self, model):
				self.meta = types.SimpleNamespace(base="Qwen/base", base_revision="pinned")

		modules = {
			"huggingface_hub": types.SimpleNamespace(snapshot_download=download),
			"huggingface_hub.utils": types.SimpleNamespace(tqdm=DisabledTqdm),
			"kev.checkpoint": types.SimpleNamespace(Checkpoint=Checkpoint, resolve_run=lambda _: "/unused/cache"),
			"transformers": types.SimpleNamespace(AutoTokenizer=types.SimpleNamespace(from_pretrained=lambda *a, **kw: None)),
		}
		with patch.dict(sys.modules, modules), contextlib.redirect_stdout(io.StringIO()) as output:
			helper = runpy.run_path(str(Path(__file__).parents[1] / "prefetch.py"))
			helper["main"]("jaredpalmer/kev-0.8b@v1.0")
		events = [json.loads(line) for line in output.getvalue().splitlines()]
		self.assertEqual(events[-1], {"event": "done", "item": "model"})
		self.assertEqual(calls, [("jaredpalmer/kev-0.8b", "v1.0"), ("Qwen/base", "pinned")])
		self.assertEqual([p["done"] for p in events if p["event"] == "progress"], [25, 25])


if __name__ == "__main__":
	unittest.main()
