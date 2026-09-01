#!/usr/bin/env python3
#
# Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.
#
"""
Producer-side pins for the network preflight's failure states.

Everything here is a pure function fed a recorded output shape. The point is
the failure states specifically: this code's job is to decide which nodes are
bad, and every bug it has had was a wrong answer produced confidently -- a
parse miss read as zero bandwidth, an untested cluster read as a healthy one.
Those are the cases with no natural place to show up in a real run, because a
real run on healthy hardware never reaches them.

Run: python3 -m unittest discover -s tests -v      (from preflight/network)
"""
import os
import sys
import tempfile
import unittest
from unittest import mock

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

import binary_diagnose as bd
import extract_nodes as en
import ib_write_bw as ib


RCCL_HEADER = (
    "#                                                              out-of-place                       in-place          \n"
    "#       size         count      type   redop    root     time   algbw   busbw #wrong     time   algbw   busbw #wrong\n"
    "#        (B)    (elements)                               (us)  (GB/s)  (GB/s)            (us)  (GB/s)  (GB/s)       \n"
)


def rccl_row(size, algbw_in="238.71", wrong="0"):
    return (f"  {size}     {size // 4}     float     sum      -1   "
            f"4521.3  237.48  474.96      {wrong}   4498.1  {algbw_in}  477.42      {wrong}\n")


class TestParseAlgbw(unittest.TestCase):
    """A missing row and a zero measurement are different answers."""

    def test_reads_the_in_place_algbw_for_the_requested_size(self):
        text = RCCL_HEADER + rccl_row(8589934592)
        self.assertAlmostEqual(bd.parse_algbw(text, 8589934592), 238.71)

    def test_wrong_column_may_be_na_when_checking_is_off(self):
        text = RCCL_HEADER + rccl_row(8589934592, wrong="N/A")
        self.assertAlmostEqual(bd.parse_algbw(text, 8589934592), 238.71)

    def test_no_row_for_the_requested_size_is_None_not_zero(self):
        # RCCL_MAX_BYTES=3G: the -b 32M -f 2 ladder stops at 2G, so there is no
        # 3G row. Returning 0.0 here condemned every node in the group.
        text = RCCL_HEADER + rccl_row(2147483648)
        self.assertIsNone(bd.parse_algbw(text, bd.parse_size("3G")))

    def test_output_with_no_table_at_all_is_None(self):
        self.assertIsNone(bd.parse_algbw("mpiexec: command not found\n", 1024))

    def test_row_spliced_by_a_debug_line_is_None_not_a_wrong_number(self):
        # 11 fields whose first 11 all parse: the pre-existing `len > 10` test
        # accepted this and read slot 10 as algbw.
        spliced = "  8589934592     2147483648     float     sum      -1   4521.3  237.48  474.96      0   4498.1  238.71\n"
        self.assertIsNone(bd.parse_algbw(RCCL_HEADER + spliced, 8589934592))

    def test_a_genuine_zero_is_still_zero(self):
        text = RCCL_HEADER + rccl_row(8589934592, algbw_in="0.00")
        self.assertEqual(bd.parse_algbw(text, 8589934592), 0.0)


class TestLadderSteps(unittest.TestCase):
    """-e has to land on a size the test actually runs."""

    def test_ladder_is_the_start_size_doubling(self):
        steps = bd.ladder_steps(bd.parse_size("32M"), bd.parse_size("1G"))
        self.assertEqual([bd.format_size(x) for x in steps],
                         ["32M", "64M", "128M", "256M", "512M", "1G"])

    def test_the_sizes_that_produce_no_row_are_not_on_it(self):
        for bad in ("3G", "100M", "10G"):
            steps = bd.ladder_steps(bd.parse_size("32M"), bd.parse_size(bad))
            self.assertNotIn(bd.parse_size(bad), steps, bad)

    def test_the_sizes_that_do_are(self):
        for good in ("32M", "64M", "8G", "16G"):
            steps = bd.ladder_steps(bd.parse_size("32M"), bd.parse_size(good))
            self.assertIn(bd.parse_size(good), steps, good)


class TestMaxBytesOverride(unittest.TestCase):
    """RCCL_MAX_BYTES reaches MAX_BYTES only when it can produce a result."""

    def setUp(self):
        self._saved = (bd.MAX_BYTES, bd.MAX_BYTES_PINNED)
        self.nodes_file = tempfile.NamedTemporaryFile("w", suffix=".ini", delete=False)
        self.nodes_file.write("10.0.0.1\n10.0.0.2\n")
        self.nodes_file.close()

    def tearDown(self):
        bd.MAX_BYTES, bd.MAX_BYTES_PINNED = self._saved
        os.unlink(self.nodes_file.name)

    def _parse(self, override):
        env = {k: v for k, v in os.environ.items()
               if k not in ("RCCL_MAX_BYTES", "RCCL_BUSBW_TARGET")}
        if override is not None:
            env["RCCL_MAX_BYTES"] = override
        argv = ["binary_diagnose.py", "--nodes_file", self.nodes_file.name]
        with mock.patch.dict(os.environ, env, clear=True), \
                mock.patch.object(sys, "argv", argv):
            bd.parse_args()
        return bd.MAX_BYTES, bd.MAX_BYTES_PINNED

    def test_unset_keeps_the_node_count_ladder(self):
        self.assertEqual(self._parse(None), ("1G", False))

    def test_a_size_on_the_ladder_is_honoured(self):
        self.assertEqual(self._parse("8G"), ("8G", True))

    def test_a_size_between_two_steps_is_refused(self):
        # 3G would run to 2G and leave nothing at 3G to read.
        for bad in ("3G", "100M", "10G"):
            self.assertEqual(self._parse(bad), ("1G", False), bad)

    def test_a_size_below_the_start_of_the_ladder_is_refused(self):
        self.assertEqual(self._parse("16M"), ("1G", False))

    def test_a_size_that_is_not_a_size_is_refused(self):
        self.assertEqual(self._parse("banana"), ("1G", False))


class TestBusbwTarget(unittest.TestCase):
    """The pass/fail bar must not be silently switchable to 'always'."""

    def _target(self, value):
        import importlib
        env = {k: v for k, v in os.environ.items() if k != "RCCL_BUSBW_TARGET"}
        if value is not None:
            env["RCCL_BUSBW_TARGET"] = value
        with mock.patch.dict(os.environ, env, clear=True):
            return importlib.reload(bd).ALLREDUCE_BUSBW_TARGET

    def tearDown(self):
        import importlib
        env = {k: v for k, v in os.environ.items() if k != "RCCL_BUSBW_TARGET"}
        with mock.patch.dict(os.environ, env, clear=True):
            importlib.reload(bd)

    def test_a_real_target_is_used(self):
        self.assertEqual(self._target("300"), 300.0)

    def test_zero_would_pass_everything_and_is_refused(self):
        self.assertEqual(self._target("0"), bd._BUSBW_TARGET_DEFAULT)

    def test_a_negative_is_refused(self):
        self.assertEqual(self._target("-1"), bd._BUSBW_TARGET_DEFAULT)

    def test_nan_would_fail_everything_and_is_refused(self):
        self.assertEqual(self._target("nan"), bd._BUSBW_TARGET_DEFAULT)

    def test_a_unit_suffix_is_refused_rather_than_crashing_at_import(self):
        self.assertEqual(self._target("350GB"), bd._BUSBW_TARGET_DEFAULT)


class TestExtractNodes(unittest.TestCase):
    """run.sh acts on this string; anything it cannot match is a dropped node."""

    def test_the_ib_write_bw_shape(self):
        self.assertEqual(
            en.extract_unhealthy_nodes("[RESULT] unhealthy nodes: ['10.0.0.7'], obtained through ib_write_bw"),
            "10.0.0.7")

    def test_the_binary_diagnose_shape_yields_the_nodes_file_token(self):
        self.assertEqual(
            en.extract_unhealthy_nodes("[RESULT] unhealthy nodes: [crsuse2-m2m-182(10.0.0.7)]"),
            "10.0.0.7")

    def test_a_bare_hostname_survives(self):
        self.assertEqual(en.extract_unhealthy_nodes("unhealthy nodes: ['node-a']"), "node-a")

    def test_an_empty_list_is_no_nodes(self):
        self.assertEqual(en.extract_unhealthy_nodes("unhealthy nodes: []"), "")

    def test_no_line_at_all_is_no_nodes(self):
        self.assertEqual(en.extract_unhealthy_nodes("everything was fine"), "")

    def test_a_parse_failure_reports_rather_than_answering_nothing_is_wrong(self):
        # The whole reason this file is not ast.literal_eval under a bare except.
        with mock.patch("sys.stderr") as err:
            self.assertEqual(en.extract_unhealthy_nodes("unhealthy nodes: [<mangled 10.0.0.7>]"), "")
            self.assertTrue(err.write.called)

    def test_duplicates_across_per_test_lines_collapse(self):
        text = ("unhealthy nodes: [host-a(10.0.0.7)]\n"
                "unhealthy nodes: [host-a(10.0.0.7), host-b(10.0.0.8)]\n")
        self.assertEqual(en.extract_unhealthy_nodes(text), "10.0.0.7,10.0.0.8")


class TestParseIbBandwidth(unittest.TestCase):
    def test_reads_the_average_column(self):
        out = " 16777216   50               0.00               24.19        0.180312\n"
        self.assertAlmostEqual(ib.parse_ib_bandwidth(out), 24.19)

    def test_no_row_is_None(self):
        self.assertIsNone(ib.parse_ib_bandwidth("---------- header only ----------\n"))


class TestIbVerdict(unittest.TestCase):
    """Every branch of the pair verdict, including the ones that mean 'unknown'."""

    HEADER = "#bytes  #iterations   BW peak[Gb/sec]  BW average[Gb/sec]  MsgRate[Mpps]\n"
    ROW = " 16777216   50               0.00               24.19        0.180312\n"

    def test_a_good_pair_passes_and_reports_the_number(self):
        ok, detail = ib.ib_verdict(0, self.HEADER + self.ROW)
        self.assertTrue(ok)
        self.assertIn("24.19", detail)

    def test_a_non_zero_exit_fails(self):
        ok, _ = ib.ib_verdict(1, self.HEADER + self.ROW)
        self.assertFalse(ok)

    def test_the_header_alone_is_not_a_measurement(self):
        # "Gb/sec" is in the header, so the old criterion passed a run that
        # printed it and then died. Exit code 0 with no row is now explicit.
        ok, detail = ib.ib_verdict(0, self.HEADER)
        self.assertTrue(ok)
        self.assertIn("no result row parsed", detail)

    def test_a_perftest_failure_marker_fails_the_pair(self):
        for marker in ib._IB_FAILURE_MARKERS:
            ok, detail = ib.ib_verdict(0, self.HEADER + self.ROW + marker + "\n")
            self.assertFalse(ok, marker)
            self.assertIn(marker, detail)

    def test_a_zero_bandwidth_row_fails(self):
        zero = " 16777216   50               0.00               0.00        0.000000\n"
        ok, detail = ib.ib_verdict(0, self.HEADER + zero)
        self.assertFalse(ok)
        self.assertIn("no bandwidth measured", detail)

    def test_a_floor_is_enforced(self):
        with mock.patch.dict(os.environ, {"IB_BW_MIN_GBPS": "100"}):
            ok, detail = ib.ib_verdict(0, self.HEADER + self.ROW)
        self.assertFalse(ok)
        self.assertIn("IB_BW_MIN_GBPS", detail)

    def test_a_floor_that_would_switch_the_check_off_is_ignored(self):
        for bad in ("0", "-5", "nan", "fast"):
            with mock.patch.dict(os.environ, {"IB_BW_MIN_GBPS": bad}):
                ok, _ = ib.ib_verdict(0, self.HEADER + self.ROW)
            self.assertTrue(ok, bad)


class TestPinnedEnv(unittest.TestCase):
    """Which knobs a site may retune, and which one it may not."""

    def test_the_tuning_knobs_are_overridable(self):
        with mock.patch.dict(os.environ, {"NCCL_CROSS_NIC": "1",
                                          "NCCL_NET_GDR_LEVEL": "0"}):
            env = bd.build_env_vars()
        self.assertEqual(env["NCCL_CROSS_NIC"], "1")
        self.assertEqual(env["NCCL_NET_GDR_LEVEL"], "0")

    def test_ib_disable_is_pinned_because_it_changes_the_transport(self):
        # An ambient 1 would run the RDMA preflight over TCP, put every pair
        # far under the busbw bar and "confirm" every node faulty.
        with mock.patch.dict(os.environ, {"NCCL_IB_DISABLE": "1"}):
            env = bd.build_env_vars()
        self.assertEqual(env["NCCL_IB_DISABLE"], "0")

    def test_scratch_reclaim_stays_pinned(self):
        with mock.patch.dict(os.environ, {"HSA_NO_SCRATCH_RECLAIM": "1"}):
            env = bd.build_env_vars()
        self.assertEqual(env["HSA_NO_SCRATCH_RECLAIM"], "0")


class TestRailProbeBudget(unittest.TestCase):
    """An unprobeable node must not cost one ssh timeout per lookup."""

    def setUp(self):
        ib._rail_cache.clear()
        ib._rail_probe_failures.clear()
        ib._pairing_cache.clear()

    tearDown = setUp

    def test_a_failing_probe_is_retried_then_given_up_on(self):
        calls = []

        def fake_run(cmd, **kw):
            calls.append(cmd)
            return mock.Mock(returncode=255, stdout="", stderr="ssh: timed out")

        with mock.patch.object(ib.subprocess, "run", fake_run):
            # test_node_group asks once per (HCA index, peer): 8 x 15 on a full
            # group. Ten lookups is already past the old cost model.
            for _ in range(10):
                self.assertEqual(ib.get_rail_map("10.0.0.9", "10.0.0.1", ["ionic_0"], ["ssh"]), {})
        self.assertEqual(len(calls), ib._RAIL_PROBE_MAX_ATTEMPTS)
        self.assertTrue(ib.rail_map_is_settled("10.0.0.9"))

    def test_a_transient_failure_still_gets_its_retry(self):
        results = [mock.Mock(returncode=255, stdout="", stderr="hiccup"),
                   mock.Mock(returncode=0, stdout="ionic_0 fe81:0001\n", stderr="")]
        with mock.patch.object(ib.subprocess, "run", lambda cmd, **kw: results.pop(0)):
            self.assertEqual(ib.get_rail_map("10.0.0.9", "10.0.0.1", ["ionic_0"], ["ssh"]), {})
            self.assertFalse(ib.rail_map_is_settled("10.0.0.9"))
            self.assertEqual(ib.get_rail_map("10.0.0.9", "10.0.0.1", ["ionic_0"], ["ssh"]),
                             {"ionic_0": "fe81:0001"})
        self.assertTrue(ib.rail_map_is_settled("10.0.0.9"))

    def test_the_static_fallback_is_cached_once_the_probes_settle(self):
        hcas = ["ionic_0", "ionic_1"]
        with mock.patch.object(ib, "get_rail_map", return_value={}):
            for _ in range(5):
                ib.resolve_pairing("10.0.0.1", "10.0.0.2", "10.0.0.1", hcas, ["ssh"])
        # Unsettled probes must stay uncached so a retry is still possible.
        self.assertNotIn(("10.0.0.1", "10.0.0.2"), ib._pairing_cache)

        ib._rail_probe_failures["10.0.0.1"] = ib._RAIL_PROBE_MAX_ATTEMPTS
        ib._rail_probe_failures["10.0.0.2"] = ib._RAIL_PROBE_MAX_ATTEMPTS
        with mock.patch.object(ib, "get_rail_map", return_value={}):
            ib.resolve_pairing("10.0.0.1", "10.0.0.2", "10.0.0.1", hcas, ["ssh"])
        self.assertIn(("10.0.0.1", "10.0.0.2"), ib._pairing_cache)


class TestRailPairing(unittest.TestCase):
    """All-or-nothing: anything the rails do not settle keeps the static table."""

    HCAS = ["ionic_0", "ionic_1"]

    def test_a_clean_bijection_is_used(self):
        client = {"ionic_0": "a", "ionic_1": "b"}
        server = {"ionic_0": "b", "ionic_1": "a"}
        self.assertEqual(ib.rail_pairing(self.HCAS, client, server), ["ionic_1", "ionic_0"])

    def test_no_rail_map_at_all_is_None(self):
        self.assertIsNone(ib.rail_pairing(self.HCAS, {}, {"ionic_0": "a"}))
        self.assertIsNone(ib.rail_pairing(self.HCAS, {"ionic_0": "a"}, {}))

    def test_a_client_card_with_no_rail_is_None(self):
        self.assertIsNone(ib.rail_pairing(
            self.HCAS, {"ionic_0": "a"}, {"ionic_0": "a", "ionic_1": "b"}))

    def test_a_peer_with_no_card_on_the_rail_is_None(self):
        self.assertIsNone(ib.rail_pairing(
            self.HCAS, {"ionic_0": "a", "ionic_1": "b"}, {"ionic_0": "a", "ionic_1": "c"}))

    def test_subnets_that_do_not_tell_the_cards_apart_are_None(self):
        # IPv4-mapped GIDs: every card shares a prefix.
        same = {"ionic_0": "a", "ionic_1": "a"}
        self.assertIsNone(ib.rail_pairing(self.HCAS, same, same))


if __name__ == "__main__":
    unittest.main(verbosity=2)
