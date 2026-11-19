#!/usr/bin/env python3

#  Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
#  See LICENSE for license information.

import unittest
from datetime import datetime, timedelta, timezone
from unittest import mock

import requests

import apikey_renew

ENV = {"IDENTITY_URL": "https://safe.example.com", "USER_APIKEY": "ak-test"}


def _rfc3339(dt: datetime) -> str:
    """Format dt the way identity does (seconds precision, Z)."""
    return dt.strftime("%Y-%m-%dT%H:%M:%SZ")


def _session(current_status=200, body=None, renew_status=200):
    """A mock session answering /current with body and /renew with renew_status."""
    session = mock.Mock()
    current = mock.Mock(status_code=current_status)
    current.json.return_value = body or {}
    session.get.return_value = current
    session.post.return_value = mock.Mock(status_code=renew_status)
    return session


class ParseExpirationTest(unittest.TestCase):
    def test_formats(self):
        want = datetime(2026, 11, 18, 10, 0, 0, tzinfo=timezone.utc)
        for text in (
            "2026-11-18T10:00:00Z",
            "2026-11-18T10:00:00",
            "2026-11-18T10:00:00.000Z",
            "2026-11-18T18:00:00+08:00",
            "2026-11-18T10:00:00.000000000Z",
        ):
            self.assertEqual(apikey_renew._parse_expiration(text), want, text)
        self.assertEqual(
            apikey_renew._parse_expiration("2026-11-18T10:00:00.123456789Z"),
            want.replace(microsecond=123456),
        )

    def test_invalid(self):
        for value in (None, "", "tomorrow", 123, "2026-13-40T10:00:00Z"):
            self.assertIsNone(apikey_renew._parse_expiration(value), value)


class EnsureUserAPIKeyTest(unittest.TestCase):
    def test_noop_without_identity_url(self):
        session = _session()
        with mock.patch.dict("os.environ", {}, clear=True):
            apikey_renew.ensure_user_apikey(session)
        session.get.assert_not_called()

    def test_renews_when_within_three_days(self):
        exp = _rfc3339(datetime.now(timezone.utc) + timedelta(days=2))
        session = _session(body={"id": 7, "expirationTime": exp})
        with mock.patch.dict("os.environ", ENV, clear=True):
            apikey_renew.ensure_user_apikey(session)
        args, kwargs = session.post.call_args
        self.assertEqual(args[0], "https://safe.example.com/api/v1/apikeys/7/renew")
        self.assertEqual(kwargs["json"], {"ttlDays": 365})

    def test_skips_renew_when_far_from_expiry(self):
        exp = _rfc3339(datetime.now(timezone.utc) + timedelta(days=30))
        session = _session(body={"id": 7, "expirationTime": exp})
        with mock.patch.dict("os.environ", ENV, clear=True):
            apikey_renew.ensure_user_apikey(session)
        session.post.assert_not_called()

    def test_failures_do_not_raise(self):
        near = _rfc3339(datetime.now(timezone.utc) + timedelta(days=1))
        past = _rfc3339(datetime.now(timezone.utc) - timedelta(days=1))
        cases = {
            "http error": _session(current_status=503),
            "rate limited": _session(current_status=429),
            "expired": _session(body={"id": 7, "expirationTime": past}),
            "non-numeric id": _session(body={"id": "seven", "expirationTime": near}),
            "renew error": _session(body={"id": 7, "expirationTime": near}, renew_status=500),
        }
        timeout = _session()
        timeout.get.side_effect = requests.Timeout("timed out")
        cases["timeout"] = timeout
        bad_json = _session()
        bad_json.get.return_value.json.side_effect = ValueError("not json")
        cases["bad json"] = bad_json
        with mock.patch.dict("os.environ", ENV, clear=True):
            for name, session in cases.items():
                with self.subTest(name):
                    apikey_renew.ensure_user_apikey(session)
        cases["non-numeric id"].post.assert_not_called()
        cases["expired"].post.assert_not_called()


if __name__ == "__main__":
    unittest.main()
