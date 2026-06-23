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
import unittest

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


if __name__ == "__main__":
    unittest.main()
