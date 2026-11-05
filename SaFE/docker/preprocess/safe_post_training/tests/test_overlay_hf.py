#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#
import json
import os

import pytest

torch = pytest.importorskip("torch")
from safetensors import safe_open  # noqa: E402
from safetensors.torch import save_file  # noqa: E402

from safe_post_training.frameworks.slime import overlay_hf  # noqa: E402


def write_hf(path, shards):
    """Write an HF directory from {shard file: {tensor name: tensor}}, with its index and a config."""
    path.mkdir(parents=True)
    weight_map = {}
    for name, tensors in shards.items():
        save_file(tensors, str(path / name), metadata={"format": "pt"})
        weight_map.update({k: name for k in tensors})
    (path / overlay_hf.INDEX).write_text(json.dumps({"metadata": {"total_size": 0}, "weight_map": weight_map}))
    (path / "config.json").write_text(json.dumps({"vocab_size": 8}))
    return path


def read(path, name):
    """Return one tensor of a safetensors file."""
    with safe_open(str(path), framework="pt") as f:
        return f.get_tensor(name)


def base_and_trained(tmp_path):
    """Return a two-shard base model and the converted checkpoint that trained its tensor b."""
    base = write_hf(tmp_path / "base", {
        "model-1.safetensors": {"a": torch.arange(4, dtype=torch.bfloat16), "b": torch.ones(2, 2, dtype=torch.bfloat16)},
        "model-2.safetensors": {"c": torch.zeros(3, dtype=torch.float32)}})
    (base / "tokenizer.json").write_text("{}")
    trained = write_hf(tmp_path / "trained", {"t.safetensors": {"b": torch.full((2, 2), 3.0)}})
    return base, trained


def test_the_trained_tensors_replace_their_base_tensors_only(tmp_path):
    base, trained = base_and_trained(tmp_path)
    out = tmp_path / "out"
    assert overlay_hf.overlay(str(base), str(trained), str(out)) == 1
    assert torch.equal(read(out / "model-1.safetensors", "a"), torch.arange(4, dtype=torch.bfloat16))
    b = read(out / "model-1.safetensors", "b")
    assert b.dtype == torch.bfloat16 and bool(b.eq(3).all()), "a trained tensor keeps the base dtype"
    assert os.stat(out / "model-2.safetensors").st_ino == os.stat(base / "model-2.safetensors").st_ino, \
        "a shard without trained tensors is a hard link of the base"
    assert json.loads((out / overlay_hf.INDEX).read_text()) == json.loads((base / overlay_hf.INDEX).read_text())
    assert (out / "tokenizer.json").exists() and (out / "config.json").exists()
    assert not (tmp_path / "out.partial").exists()


def test_shards_are_copied_where_links_fail(monkeypatch, tmp_path):
    base, trained = base_and_trained(tmp_path)

    def refuse(src, dst):
        raise OSError("cross-device link")

    monkeypatch.setattr(os, "link", refuse)
    out = tmp_path / "out"
    overlay_hf.overlay(str(base), str(trained), str(out))
    assert os.stat(out / "model-2.safetensors").st_ino != os.stat(base / "model-2.safetensors").st_ino
    assert (out / "model-2.safetensors").read_bytes() == (base / "model-2.safetensors").read_bytes()


def test_a_checkpoint_that_does_not_fit_the_base_is_refused(tmp_path):
    base = write_hf(tmp_path / "base", {"m.safetensors": {"a": torch.zeros(2)}})
    unknown = write_hf(tmp_path / "unknown", {"t.safetensors": {"z": torch.zeros(2)}})
    with pytest.raises(ValueError, match="1 tensors the base model lacks, e.g. z"):
        overlay_hf.overlay(str(base), str(unknown), str(tmp_path / "out"))
    shaped = write_hf(tmp_path / "shaped", {"t.safetensors": {"a": torch.zeros(3)}})
    with pytest.raises(ValueError, match=r"a has shape \(3,\) in the checkpoint and \(2,\) in the base model"):
        overlay_hf.overlay(str(base), str(shaped), str(tmp_path / "out"))
    assert overlay_hf.main(["--base", str(base), "--trained", str(unknown), "--out", str(tmp_path / "out")]) == 1
    assert not (tmp_path / "out").exists()
