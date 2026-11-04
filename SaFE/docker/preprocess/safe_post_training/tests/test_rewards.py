#
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#
import asyncio
import http.server
import threading
import types

import pytest

from safe_post_training import stages
from safe_post_training.frameworks.slime.plugins import custom_rm, remote_rm
from safe_post_training.tests.test_launch import FakeAdapter, recipe_for, run


def sample(label="4"):
    return types.SimpleNamespace(prompt="2+2?", response="4", label=label)


def test_custom_reward_file(monkeypatch, tmp_path):
    rm = tmp_path / "judge.py"
    rm.write_text("def custom_rm(args, sample):\n    return float(sample.response == sample.label)\n\n"
                  "async def score(args, sample):\n    return 0.5\n")
    for spec, want in ((str(rm), 1.0), (str(rm) + ":score", 0.5)):
        monkeypatch.setattr(custom_rm, "_function", None)
        monkeypatch.setenv(custom_rm.SPEC_ENV, spec)
        assert asyncio.run(custom_rm.custom_rm(None, sample())) == want
    with pytest.raises(FileNotFoundError):
        custom_rm.load(str(tmp_path / "missing.py"))
    with pytest.raises(AttributeError):
        custom_rm.load(str(rm) + ":nothing")


class FakeResponse:
    def __init__(self, status, body=None):
        self.status, self.body = status, body

    async def __aenter__(self):
        return self

    async def __aexit__(self, *exc):
        return False

    def raise_for_status(self):
        if self.status >= 400:
            raise RuntimeError("HTTP %d" % self.status)

    async def json(self):
        return self.body


class FakeSession:
    closed = False

    def __init__(self, *statuses):
        self.statuses, self.calls = list(statuses), []

    def post(self, url, json=None, headers=None):
        self.calls.append((url, json, headers))
        return FakeResponse(self.statuses.pop(0), {"score": 1})


async def no_sleep(seconds):
    return None


def test_remote_reward_with_key(monkeypatch, capsys):
    monkeypatch.setenv(remote_rm.KEY_ENV, "k-123")
    monkeypatch.setattr(remote_rm.asyncio, "sleep", no_sleep)
    session = FakeSession(503, 200)
    monkeypatch.setattr(remote_rm, "_get_session", lambda: session)
    args = types.SimpleNamespace(rm_url="http://rm.svc/score")
    assert asyncio.run(remote_rm.remote_rm(args, sample())) == {"score": 1}
    assert len(session.calls) == 2, "a server error is retried as upstream does"
    url, payload, headers = session.calls[0]
    assert url == "http://rm.svc/score" and payload == {"prompt": "2+2?", "response": "4", "label": "4"}
    assert headers == {"Authorization": "Bearer k-123"}

    session = FakeSession(401, 200)
    monkeypatch.setattr(remote_rm, "_get_session", lambda: session)
    with pytest.raises(remote_rm.RewardKeyRejected):
        asyncio.run(remote_rm.remote_rm(args, sample()))
    assert len(session.calls) == 1, "a rejected key is not retried"
    assert stages.REWARD_REJECTED_TEXT + " (HTTP 401)" in capsys.readouterr().out


class AuthHandler(http.server.BaseHTTPRequestHandler):
    """Answers 401 unless the request carries the expected bearer token."""

    def do_HEAD(self):
        self.send_response(200 if self.headers.get("Authorization") == "Bearer good" else 401)
        self.end_headers()

    def log_message(self, *args):
        pass


def check(recipe, tmp_path):
    stages.Runner(recipe, str(tmp_path), adapter=None, emitter=None).check_reward(timeout=5)


def test_check_reward(monkeypatch, tmp_path):
    server = http.server.HTTPServer(("127.0.0.1", 0), AuthHandler)
    threading.Thread(target=server.serve_forever, daemon=True).start()
    url = "http://127.0.0.1:%d/score" % server.server_port
    remote = {"trainType": "rl", "choices": {"rewardType": "remote", "rewardRemote": {"url": url, "secretId": "s"}}}
    try:
        monkeypatch.setenv(stages.REWARD_KEY_ENV, "good")
        check(remote, tmp_path)
        monkeypatch.setenv(stages.REWARD_KEY_ENV, "bad")
        with pytest.raises(stages.StageError) as e:
            check(remote, tmp_path)
        assert not e.value.retry and "rejected the key (HTTP 401)" in e.value.reason
        remote["choices"]["rewardRemote"]["secretId"] = ""
        check(remote, tmp_path)
    finally:
        server.shutdown()

    remote["choices"]["rewardRemote"]["url"] = "http://127.0.0.1:9/score"
    with pytest.raises(stages.StageError) as e:
        check(remote, tmp_path)
    assert e.value.retry and "can't reach the reward service" in e.value.reason

    custom = {"trainType": "rl", "choices": {"rewardType": "custom", "rewardHook": str(tmp_path / "rm.py:score")}}
    with pytest.raises(stages.StageError) as e:
        check(custom, tmp_path)
    assert not e.value.retry and "rm.py does not exist" in e.value.reason
    (tmp_path / "rm.py").write_text("def score(args, sample):\n    return 1\n")
    check(custom, tmp_path)


class RejectedAdapter(FakeAdapter):
    """Trains like the remote_rm plugin does when the reward service refuses the key."""

    def train_command(self, recipe, run_dir):
        cmd, env, files = super().train_command(recipe, run_dir)
        cmd[-1] = "import sys\nprint(%r)\nsys.exit(1)\n" % (stages.REWARD_REJECTED_TEXT + " (HTTP 401)")
        return cmd, env, files


def test_rejected_key_fails_without_retry(monkeypatch, tmp_path):
    code, _, ms = run(monkeypatch, recipe_for(tmp_path), RejectedAdapter())
    assert code == 1
    assert ms[-1][1]["reason"] == "the reward service rejected the key" and ms[-1][1]["retry"] is False
