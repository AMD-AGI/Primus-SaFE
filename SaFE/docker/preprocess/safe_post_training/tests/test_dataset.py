#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#
import json

import pytest

from safe_post_training import dataset


def write_jsonl(path, rows):
    path.write_text("\n".join(json.dumps(r) for r in rows) + "\n")


def read_jsonl(path):
    return [json.loads(l) for l in path.read_text().splitlines()]


def test_sft_alpaca_to_messages(tmp_path):
    src = tmp_path / "train.jsonl"
    write_jsonl(src, [{"instruction": "Add", "input": "1+1", "output": "2"}, {"instruction": "Hi", "output": "Hello"}])
    stats = dataset.prepare(str(src), str(tmp_path / "out" / "train.jsonl"), "sft", {})
    assert stats == {"rows": 2, "written": 2}
    rows = read_jsonl(tmp_path / "out" / "train.jsonl")
    assert rows[0] == {"messages": [{"role": "user", "content": "Add\n\n1+1"}, {"role": "assistant", "content": "2"}]}
    assert rows[1]["messages"][0]["content"] == "Hi"


def test_sft_messages_and_directories(tmp_path):
    d = tmp_path / "ds"
    d.mkdir()
    (d / "a.json").write_text(json.dumps([{"conv": [{"role": "user", "content": "q"}]}]))
    write_jsonl(d / "b.jsonl", [{"conv": [{"role": "user", "content": "r"}]}])
    (d / "notes.txt").write_text("ignored")
    stats = dataset.prepare(str(d), str(tmp_path / "t.jsonl"), "sft", {"messages": "conv"})
    assert stats["written"] == 2


def test_rl_prompt_and_label(tmp_path):
    src = tmp_path / "rl.jsonl"
    write_jsonl(src, [{"question": "2+2?", "answer": 4}])
    dataset.prepare(str(src), str(tmp_path / "t.jsonl"), "rl", {"prompt": "question", "label": "answer"})
    assert read_jsonl(tmp_path / "t.jsonl") == [{"prompt": "2+2?", "label": "4"}]


def test_missing_field_names_the_field_and_rows(tmp_path):
    src = tmp_path / "train.jsonl"
    write_jsonl(src, [{"instruction": "a", "output": "b"}, {"instruction": "c"}, {"instruction": "d"}])
    with pytest.raises(dataset.DatasetError, match="field 'output' missing in 2 of 3 rows"):
        dataset.prepare(str(src), str(tmp_path / "t.jsonl"), "sft", {})
    assert not (tmp_path / "t.jsonl").exists()


def test_validation_split_is_reproducible(tmp_path):
    src = tmp_path / "train.jsonl"
    write_jsonl(src, [{"instruction": str(i), "output": "x"} for i in range(1000)])
    runs = []
    for n in range(2):
        stats = dataset.prepare(str(src), str(tmp_path / ("t%d.jsonl" % n)), "sft", {},
                                split_dst=str(tmp_path / ("e%d.jsonl" % n)), split=0.1, split_limit=50)
        runs.append((tmp_path / ("e%d.jsonl" % n)).read_text())
    assert stats["split"] == 50 and stats["written"] + 50 < 1000
    assert runs[0] == runs[1], "a fixed seed picks the same rows on a retry"


def test_limit_and_errors(tmp_path):
    src = tmp_path / "train.jsonl"
    write_jsonl(src, [{"instruction": str(i), "output": "x"} for i in range(5)])
    assert dataset.prepare(str(src), str(tmp_path / "e.jsonl"), "sft", {}, limit=2)["written"] == 2
    with pytest.raises(dataset.DatasetError, match="does not exist"):
        dataset.prepare(str(tmp_path / "missing"), str(tmp_path / "t.jsonl"), "sft", {})
    (tmp_path / "bad.jsonl").write_text("{not json}\n")
    with pytest.raises(dataset.DatasetError, match="line 1 is not JSON"):
        dataset.prepare(str(tmp_path / "bad.jsonl"), str(tmp_path / "t.jsonl"), "sft", {})
