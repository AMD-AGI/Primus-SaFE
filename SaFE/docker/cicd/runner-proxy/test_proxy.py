#!/usr/bin/env python3

#  Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
#  See LICENSE for license information.

"""Unit tests for runner-proxy authentication."""

import os
import tempfile
import unittest
from unittest import mock

import requests

import proxy


class PodTokenAuthTest(unittest.TestCase):
    """The proxy authenticates with the pod's projected ServiceAccount token."""

    def _request(self):
        return requests.Request("GET", "http://apiserver.invalid/api/v1/workloads/w").prepare()

    def _write(self, path, content):
        with open(path, "w", encoding="utf-8") as f:
            f.write(content)

    def test_sends_the_token_as_the_bearer_credential(self):
        with tempfile.TemporaryDirectory() as d:
            path = os.path.join(d, "token")
            self._write(path, "token-1\n")
            req = proxy.PodTokenAuth(path)(self._request())
            self.assertEqual(req.headers["Authorization"], "Bearer token-1")

    def test_reads_the_file_again_for_every_request(self):
        # The kubelet replaces the file when it renews the token; a token read once would
        # expire in the middle of a job that runs for days.
        with tempfile.TemporaryDirectory() as d:
            path = os.path.join(d, "token")
            auth = proxy.PodTokenAuth(path)
            self._write(path, "old")
            self.assertEqual(auth(self._request()).headers["Authorization"], "Bearer old")
            self._write(path, "renewed")
            self.assertEqual(auth(self._request()).headers["Authorization"], "Bearer renewed")

    def test_session_signs_with_the_pod_token_and_ignores_an_api_key(self):
        env = {"ADMIN_CONTROL_PLANE": "10.0.0.1", "APISERVER_NODE_PORT": "32495", "USER_APIKEY": "ak-legacy"}
        with tempfile.TemporaryDirectory() as d:
            path = os.path.join(d, "token")
            self._write(path, "pod-token")
            with mock.patch.dict(os.environ, env), mock.patch.object(proxy, "POD_TOKEN_FILE", path):
                session, base_url = proxy.build_session()
        self.assertEqual(base_url, "http://10.0.0.1:32495")
        self.assertIsInstance(session.auth, proxy.PodTokenAuth)
        self.assertNotIn("Authorization", session.headers)
        self.assertEqual(session.auth.path, path)

    def test_session_without_the_token_file_is_an_error(self):
        env = {"ADMIN_CONTROL_PLANE": "10.0.0.1", "APISERVER_NODE_PORT": "32495"}
        with mock.patch.dict(os.environ, env), mock.patch.object(proxy, "POD_TOKEN_FILE", "/nonexistent/token"):
            with self.assertRaises(ValueError):
                proxy.build_session()


if __name__ == "__main__":
    unittest.main()
