#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#
"""Prepare stage: convert a workspace dataset into the JSONL the training framework reads.

SFT rows become {"messages": [...]}, from an alpaca-style instruction/input/output mapping or an existing
messages field; RL rows become {"prompt": ..., "label": ...}. Inputs are .jsonl, .json and, when pyarrow is in the
image, .parquet files, or a directory of them.
"""

import json
import os

DATA_EXTENSIONS = (".jsonl", ".json", ".parquet")


class DatasetError(Exception):
    """A dataset problem that a retry would hit again."""


def input_files(path):
    """List the data files of a dataset path: the file itself, or the supported files of a directory, sorted."""
    if os.path.isfile(path):
        return [path]
    if not os.path.isdir(path):
        raise DatasetError("dataset path %s does not exist" % path)
    files = []
    for root, _, names in os.walk(path):
        for name in names:
            if name.endswith(DATA_EXTENSIONS) and not name.startswith("."):
                files.append(os.path.join(root, name))
    if not files:
        raise DatasetError("dataset path %s holds no .jsonl, .json or .parquet file" % path)
    return sorted(files)


def read_rows(path):
    """Yield the rows of one data file as dicts."""
    if path.endswith(".parquet"):
        try:
            import pyarrow.parquet as pq
        except ImportError as e:
            raise DatasetError("reading %s needs pyarrow, which the image lacks" % path) from e
        for batch in pq.ParquetFile(path).iter_batches():
            yield from batch.to_pylist()
        return
    with open(path, encoding="utf-8") as f:
        if path.endswith(".json"):
            data = json.load(f)
            rows = data if isinstance(data, list) else data.get("data", [])
            yield from rows
            return
        for n, line in enumerate(f, 1):
            line = line.strip()
            if not line:
                continue
            try:
                yield json.loads(line)
            except json.JSONDecodeError as e:
                raise DatasetError("%s line %d is not JSON: %s" % (path, n, e)) from e


def sft_row(row, field_map):
    """Convert one row to slime's SFT messages format, or return the name of the missing field."""
    messages_field = field_map.get("messages")
    if messages_field:
        messages = row.get(messages_field)
        return ({"messages": messages}, None) if isinstance(messages, list) and messages else (None, messages_field)
    instruction, output = field_map.get("instruction", "instruction"), field_map.get("output", "output")
    for field in (instruction, output):
        if row.get(field) in (None, ""):
            return None, field
    prompt = str(row[instruction])
    extra = row.get(field_map.get("input", "input"))
    if extra:
        prompt += "\n\n" + str(extra)
    return {"messages": [{"role": "user", "content": prompt},
                         {"role": "assistant", "content": str(row[output])}]}, None


def rl_row(row, field_map):
    """Convert one row to slime's prompt and label format, or return the name of the missing field."""
    prompt, label = field_map.get("prompt", "prompt"), field_map.get("label", "label")
    for field in (prompt, label):
        if row.get(field) in (None, ""):
            return None, field
    value = row[prompt]
    if not isinstance(value, (str, list)):
        value = str(value)
    return {"prompt": value, "label": str(row[label])}, None


def prepare(src, dst, train_type, field_map, limit=None):
    """Convert the dataset at src into dst and return its statistics. A field missing from any row fails the
    stage, naming the field and how many rows lack it; limit keeps only the first rows (validation sets)."""
    convert = rl_row if train_type == "rl" else sft_row
    missing = {}
    total = written = 0
    os.makedirs(os.path.dirname(dst), exist_ok=True)
    tmp = dst + ".tmp"
    with open(tmp, "w", encoding="utf-8") as out:
        for path in input_files(src):
            for row in read_rows(path):
                total += 1
                if not isinstance(row, dict):
                    missing["<row is not an object>"] = missing.get("<row is not an object>", 0) + 1
                    continue
                converted, field = convert(row, field_map or {})
                if field:
                    missing[field] = missing.get(field, 0) + 1
                    continue
                if limit is None or written < limit:
                    out.write(json.dumps(converted, ensure_ascii=False) + "\n")
                    written += 1
    if missing:
        os.remove(tmp)
        field, count = max(missing.items(), key=lambda kv: kv[1])
        raise DatasetError("field '%s' missing in %d of %d rows" % (field, count, total))
    if written == 0:
        os.remove(tmp)
        raise DatasetError("the dataset has no rows")
    os.replace(tmp, dst)
    return {"rows": total, "written": written}
