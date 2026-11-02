"""Explicit online setup only: cache the adapter, base and tokenizer."""
import json
import sys


def emit(**event):
	print(json.dumps(event), flush=True)


def main(model):
	from huggingface_hub import snapshot_download
	from huggingface_hub.utils import tqdm
	from kev.checkpoint import Checkpoint, resolve_run
	from transformers import AutoTokenizer

	class JsonTqdm(tqdm):
		item = "adapter"

		def update(self, n=1):
			result = super().update(n)
			emit(event="progress", item=self.item, file=str(self.desc or ""),
				 done=int(self.n), total=int(self.total or 0))
			return result

	patterns = ["*.json", "*.safetensors", "*.pt", "*.txt", "*.jinja"]
	repo, _, revision = model.partition("@")
	emit(event="phase", item="adapter")
	# Fetch first: Checkpoint's resolver otherwise downloads without our callback.
	snapshot_download(repo, revision=revision or None, allow_patterns=patterns,
					  tqdm_class=JsonTqdm)
	ck = Checkpoint(model)
	emit(event="done", item="adapter")
	JsonTqdm.item = "base"
	emit(event="phase", item="base")
	snapshot_download(ck.meta.base, revision=ck.meta.base_revision,
					  allow_patterns=patterns, tqdm_class=JsonTqdm)
	resolve_run(f"{ck.meta.base}@{ck.meta.base_revision or ''}")
	AutoTokenizer.from_pretrained(ck.meta.base, revision=ck.meta.base_revision)
	emit(event="done", item="base")
	emit(event="done", item="model")


if __name__ == "__main__":
	try:
		main(sys.argv[1])
	except Exception as exc:
		emit(event="error", item="model", message=str(exc))
		raise
