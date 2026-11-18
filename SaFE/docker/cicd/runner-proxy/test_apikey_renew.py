#!/usr/bin/env python3

#  Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
#  See LICENSE for license information.

import unittest
from datetime import datetime, timedelta, timezone
from unittest import mock

import apikey_renew


class EnsureUserAPIKeyTest(unittest.TestCase):
    def test_noop_without_identity_url(self):
        with mock.patch.dict("os.environ", {}, clear=True):
            apikey_renew.ensure_user_apikey()

    def test_renews_when_within_three_days(self):
        exp = (datetime.now(timezone.utc) + timedelta(days=2)).strftime(apikey_renew.SAFE_TIME_LAYOUT)
        session = mock.Mock()
        current = mock.Mock(status_code=200)
        current.json.return_value = {"id": 7, "expirationTime": exp}
        renew = mock.Mock(status_code=200)
        renew.json.return_value = {}
        session.get.return_value = current
        session.post.return_value = renew
        env = {
            "IDENTITY_URL": "http://identity.svc:8080",
            "USER_APIKEY": "ak-test",
        }
        with mock.patch.dict("os.environ", env, clear=True):
            apikey_renew.ensure_user_apikey(session)
        session.post.assert_called_once()
        args, kwargs = session.post.call_args
        self.assertIn("/api/v1/apikeys/7/renew", args[0])
        self.assertEqual(kwargs["json"], {"ttlDays": 365})

    def test_skips_renew_when_far_from_expiry(self):
        exp = (datetime.now(timezone.utc) + timedelta(days=30)).strftime(apikey_renew.SAFE_TIME_LAYOUT)
        session = mock.Mock()
        current = mock.Mock(status_code=200)
        current.json.return_value = {"id": 7, "expirationTime": exp}
        session.get.return_value = current
        env = {
            "IDENTITY_URL": "http://identity.svc:8080",
            "USER_APIKEY": "ak-test",
        }
        with mock.patch.dict("os.environ", env, clear=True):
            apikey_renew.ensure_user_apikey(session)
        session.post.assert_not_called()

    def test_expired_raises(self):
        exp = (datetime.now(timezone.utc) - timedelta(days=1)).strftime(apikey_renew.SAFE_TIME_LAYOUT)
        session = mock.Mock()
        current = mock.Mock(status_code=200)
        current.json.return_value = {"id": 7, "expirationTime": exp}
        session.get.return_value = current
        env = {
            "IDENTITY_URL": "http://identity.svc:8080",
            "USER_APIKEY": "ak-test",
        }
        with mock.patch.dict("os.environ", env, clear=True):
            with self.assertRaisesRegex(RuntimeError, "expired"):
                apikey_renew.ensure_user_apikey(session)


if __name__ == "__main__":
    unittest.main()
