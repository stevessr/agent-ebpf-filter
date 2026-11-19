#!/usr/bin/env python3
"""Synthetic fixtures exercise scoring correctness, not reported experiment data."""
import csv
import importlib.util
import json
import tempfile
from pathlib import Path
import unittest

SCRIPT = Path(__file__).resolve().parents[1] / "paper-attribution-reconcile.py"
spec = importlib.util.spec_from_file_location("paper_attribution_reconcile", SCRIPT)
scorer = importlib.util.module_from_spec(spec)
spec.loader.exec_module(scorer)


class AttributionJoinTests(unittest.TestCase):
    def test_event_ownership_and_no_event_separate_from_abstention(self):
        truth = {
            (2, 1): [
                {"agent_count": "2", "cycle": "1", "child_pid": "101",
                 "started_ns": "10000000000", "finished_ns": "10100000000",
                 "agent_id": "agent-A", "command": "git"},
                {"agent_count": "2", "cycle": "1", "child_pid": "102",
                 "started_ns": "10000000000", "finished_ns": "10100000000",
                 "agent_id": "agent-B", "command": "python"},
                {"agent_count": "2", "cycle": "1", "child_pid": "103",
                 "started_ns": "10000000000", "finished_ns": "10100000000",
                 "agent_id": "agent-B", "command": "node"},
            ]
        }
        with tempfile.TemporaryDirectory() as directory:
            folder = Path(directory)
            (folder / "events_agents2_cycle1.json").write_text(json.dumps({
                "agent_count": 2, "cycle": 1, "source": "memory", "events": [
                    {"eventId": "evt-1", "pid": 101, "comm": "git", "agentRunId": "agent-A",
                     "receivedAtMs": 10010, "type": "openat"},
                    {"eventId": "evt-2", "pid": 102, "comm": "python3", "agentRunId": "",
                     "receivedAtMs": 10020, "type": "openat"},
                    {"eventId": "unrelated", "pid": 199, "comm": "node",
                     "receivedAtMs": 10030},
                ]
            }), encoding="utf-8")
            observed, diagnostic = scorer.join_live_events(truth, folder)
            self.assertEqual(len(observed), 2)
            self.assertEqual(diagnostic["ground_truth_tools"], 3)
            self.assertEqual(diagnostic["tools_without_matched_events"], 1)
            self.assertEqual(diagnostic["unmatched_events"], 1)
            r = scorer.rates(observed)[2]
            self.assertEqual(r, {"events": 2, "correct": 1,
                                 "false_attribution": 0, "unattributed": 1})

    def test_mismatched_comm_or_pid_never_claims_ownership(self):
        truth = {(2, 1): [{"child_pid": "8", "agent_id": "true", "command": "node",
                           "started_ns": "1000000000", "finished_ns": "1100000000"}]}
        with tempfile.TemporaryDirectory() as tmp:
            (Path(tmp) / "events_agents2_cycle1.json").write_text(
                json.dumps({"agent_count": 2, "cycle": 1, "events": [
                    {"eventId": "x", "pid": 8, "comm": "python3", "agentRunId": "true", "receivedAtMs": 1010},
                    {"eventId": "y", "pid": 8, "comm": "node", "agentRunId": "true", "receivedAtMs": 8000},
                ]}), encoding="utf-8"
            )
            events, diag = scorer.join_live_events(truth, Path(tmp))
            self.assertFalse(events)
            self.assertEqual(diag["unmatched_events"], 2)

    def test_duplicate_event_id_with_conflicting_prediction_fails(self):
        # One duplicated event in two different cohort snapshots must not
        # silently be counted twice or reassigned to another Agent.
        truth = {
            (2, 1): [{"child_pid": "12", "agent_id": "A", "command": "git",
                      "started_ns": "1000000000", "finished_ns": "1200000000"}],
            (2, 2): [{"child_pid": "12", "agent_id": "B", "command": "git",
                      "started_ns": "1000000000", "finished_ns": "1200000000"}]
        }
        with tempfile.TemporaryDirectory() as tmp:
            for cycle in (1, 2):
                (Path(tmp) / f"events_agents2_cycle{cycle}.json").write_text(
                    json.dumps({"agent_count": 2, "cycle": cycle, "events": [
                        {"eventId": "same", "pid": 12, "comm": "git",
                         "receivedAtMs": 1050, "agentRunId": "A"},
                    ]}), encoding="utf-8"
                )
            with self.assertRaisesRegex(ValueError, "conflicting duplicated"):
                scorer.join_live_events(truth, Path(tmp))


if __name__ == "__main__":
    unittest.main()
