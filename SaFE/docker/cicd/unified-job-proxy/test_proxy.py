#!/usr/bin/env python3

#  Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
#  See LICENSE for license information.

"""Unit tests for unified-job-proxy payload construction.

These tests focus on the invariant documented by the workload API
(WorkloadSpec): images and entryPoints must align by index with resources.
Regression coverage for issue #572, where multi-node UnifiedJob workloads were
created with a single image/entryPoint while resources were expanded into
Master + Worker entries, leaving the Worker container without an image.
"""

import base64
import os
import tempfile
import unittest
from unittest import mock

import requests

import proxy


class ConvertResourcesToArrayTest(unittest.TestCase):
    def test_single_replica_returns_one_entry(self):
        result = proxy.convert_resources_to_array({"replica": 1, "gpu": "8"})
        self.assertEqual(len(result), 1)
        self.assertEqual(result[0]["replica"], 1)

    def test_two_replicas_split_into_master_and_worker(self):
        result = proxy.convert_resources_to_array({"replica": 2, "gpu": "8"})
        self.assertEqual(len(result), 2)
        self.assertEqual(result[0]["replica"], 1)  # master
        self.assertEqual(result[1]["replica"], 1)  # worker

    def test_four_replicas_split_master_one_worker_rest(self):
        result = proxy.convert_resources_to_array({"replica": 4, "gpu": "8"})
        self.assertEqual(len(result), 2)
        self.assertEqual(result[0]["replica"], 1)  # master
        self.assertEqual(result[1]["replica"], 3)  # worker

    def test_string_replica_is_parsed(self):
        result = proxy.convert_resources_to_array({"replica": "2", "gpu": "8"})
        self.assertEqual(len(result), 2)

    def test_missing_replica_defaults_to_one(self):
        result = proxy.convert_resources_to_array({"gpu": "8"})
        self.assertEqual(len(result), 1)
        self.assertEqual(result[0]["replica"], 1)


class BuildPayloadAlignmentTest(unittest.TestCase):
    """images/entryPoints must always match the length of resources."""

    BASE_INPUT = {
        "model": "llama",
        "command": "python train.py",
        "image": "myregistry/trainer:v1",
    }

    def _input(self, **overrides):
        data = dict(self.BASE_INPUT)
        data.update(overrides)
        return data

    def _assert_aligned(self, payload):
        n = len(payload["resources"])
        self.assertEqual(len(payload["images"]), n)
        self.assertEqual(len(payload["entryPoints"]), n)

    def test_single_node_lengths_are_one(self):
        payload = proxy.build_payload_from_input(
            self._input(resources={"replica": 1, "gpu": "8"})
        )
        self.assertEqual(len(payload["resources"]), 1)
        self._assert_aligned(payload)

    def test_multi_node_arrays_expanded_to_match_resources(self):
        payload = proxy.build_payload_from_input(
            self._input(resources={"replica": 2, "gpu": "8"})
        )
        self.assertEqual(len(payload["resources"]), 2)
        self._assert_aligned(payload)
        # Master and Worker share the same image and entrypoint.
        self.assertEqual(payload["images"][0], payload["images"][1])
        self.assertEqual(payload["entryPoints"][0], payload["entryPoints"][1])

    def test_large_replica_still_aligned(self):
        payload = proxy.build_payload_from_input(
            self._input(resources={"replica": 4, "gpu": "8"})
        )
        self.assertEqual(len(payload["resources"]), 2)
        self._assert_aligned(payload)

    def test_missing_resources_keeps_arrays_non_empty(self):
        payload = proxy.build_payload_from_input(self._input())
        self.assertGreaterEqual(len(payload["resources"]), 1)
        self._assert_aligned(payload)
        self.assertTrue(payload["images"][0])
        self.assertTrue(payload["entryPoints"][0])

    def test_command_is_base64_encoded(self):
        payload = proxy.build_payload_from_input(
            self._input(resources={"replica": 2, "gpu": "8"})
        )
        decoded = base64.b64decode(payload["entryPoints"][0]).decode("utf-8")
        self.assertEqual(decoded, "python train.py")

    def test_already_base64_command_not_double_encoded(self):
        encoded = base64.b64encode(b"python train.py").decode("ascii")
        payload = proxy.build_payload_from_input(
            self._input(command=encoded, resources={"replica": 2, "gpu": "8"})
        )
        self.assertEqual(payload["entryPoints"][0], encoded)

    def test_image_value_propagated_to_all_entries(self):
        payload = proxy.build_payload_from_input(
            self._input(resources={"replica": 2, "gpu": "8"})
        )
        for img in payload["images"]:
            self.assertEqual(img, "myregistry/trainer:v1")

    def test_missing_required_fields_raise(self):
        for missing in ("model", "command", "image"):
            data = self._input(resources={"replica": 2})
            data.pop(missing)
            with self.assertRaises(ValueError):
                proxy.build_payload_from_input(data)


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
