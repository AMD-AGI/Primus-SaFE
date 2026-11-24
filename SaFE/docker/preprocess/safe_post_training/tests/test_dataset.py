#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#
import errno
import json
import os

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
    assert stats["split"] == 50 and stats["written"] + 50 == 1000, "rows past the split limit go to training"
    assert runs[0] == runs[1], "a fixed seed picks the same rows on a retry"
    assert len(read_jsonl(tmp_path / "t0.jsonl")) == 950 and len(read_jsonl(tmp_path / "e0.jsonl")) == 50


def test_every_row_goes_to_training_or_validation(tmp_path):
    src = tmp_path / "train.jsonl"
    write_jsonl(src, [{"instruction": str(i), "output": "x"} for i in range(10000)])
    stats = dataset.prepare(str(src), str(tmp_path / "t.jsonl"), "sft", {}, split_dst=str(tmp_path / "e.jsonl"),
                            split=0.1, split_limit=50)
    assert stats == {"rows": 10000, "written": 9950, "split": 50}
    seen = read_jsonl(tmp_path / "t.jsonl") + read_jsonl(tmp_path / "e.jsonl")
    assert sorted(int(r["messages"][0]["content"]) for r in seen) == list(range(10000))


def test_limit_and_errors(tmp_path):
    src = tmp_path / "train.jsonl"
    write_jsonl(src, [{"instruction": str(i), "output": "x"} for i in range(5)])
    assert dataset.prepare(str(src), str(tmp_path / "e.jsonl"), "sft", {}, limit=2)["written"] == 2
    with pytest.raises(dataset.DatasetError, match="does not exist"):
        dataset.prepare(str(tmp_path / "missing"), str(tmp_path / "t.jsonl"), "sft", {})
    (tmp_path / "bad.jsonl").write_text("{not json}\n")
    with pytest.raises(dataset.DatasetError, match="line 1 is not JSON"):
        dataset.prepare(str(tmp_path / "bad.jsonl"), str(tmp_path / "t.jsonl"), "sft", {})


@pytest.mark.parametrize("name, content, message", [
    ("syntax.json", b"[{\"instruction\": ", "is not JSON"),
    ("scalar.json", b"3", "holds neither a list of rows nor a data list"),
    ("latin1.jsonl", b"{\"instruction\": \"caf\xe9\", \"output\": \"x\"}\n", "is not UTF-8 text"),
    ("list.jsonl", b"[1, 2]\n", "row 1 is not an object"),
    ("rows.json", b"[\"a\"]", "row 1 is not an object"),
])
def test_malformed_files_are_dataset_errors(tmp_path, name, content, message):
    (tmp_path / name).write_bytes(content)
    with pytest.raises(dataset.DatasetError, match=message):
        dataset.prepare(str(tmp_path / name), str(tmp_path / "out" / "t.jsonl"), "sft", {},
                        split_dst=str(tmp_path / "out" / "e.jsonl"), split=0.5)
    assert os.listdir(tmp_path / "out") == [], "no partial output is left behind"


def test_a_broken_parquet_file_is_a_dataset_error(tmp_path):
    pytest.importorskip("pyarrow")
    (tmp_path / "train.parquet").write_bytes(b"not parquet")
    with pytest.raises(dataset.DatasetError, match="is not a readable Parquet file"):
        dataset.prepare(str(tmp_path / "train.parquet"), str(tmp_path / "t.jsonl"), "sft", {})


def test_a_storage_error_reading_parquet_is_not_a_dataset_error(monkeypatch, tmp_path):
    """A read error of the storage may pass on the next attempt; what pyarrow says of a damaged file does not."""
    pq = pytest.importorskip("pyarrow.parquet")
    path = str(tmp_path / "train.parquet")
    errors = iter([OSError(errno.EIO, "Input/output error"), OSError("Corrupt snappy compressed data")])

    def parquet_file(_):
        raise next(errors)

    monkeypatch.setattr(pq, "ParquetFile", parquet_file)
    with pytest.raises(OSError) as e:
        list(dataset.read_rows(path))
    assert e.value.errno == errno.EIO
    with pytest.raises(dataset.DatasetError, match="is not a readable Parquet file"):
        list(dataset.read_rows(path))
