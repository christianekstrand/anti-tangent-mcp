import json
import os
import tempfile
import unittest

from check_progress_nudge import (
    COMPLETION_TOOL, DEFAULT_EDITS, PROGRESS_TOOL, SPEC_TOOL, claim, decide, own_transcript, scan, threshold)


def tool_use(name, call_id="t1"):
    return json.dumps({"type": "assistant", "message": {"role": "assistant", "content": [
        {"type": "tool_use", "id": call_id, "name": name, "input": {}}]}})


def user(text):
    return json.dumps({"type": "user", "message": {"role": "user", "content": text}})


def edits(n):
    return [tool_use("Edit", "e%d" % i) for i in range(n)]


class ThresholdTest(unittest.TestCase):
    def test_a_whole_number_above_zero_is_used(self):
        self.assertEqual(threshold("3"), 3)
        self.assertEqual(threshold(" 25 "), 25)

    def test_an_unusable_value_falls_back_to_the_default(self):
        for raw in (None, "", "abc", "0", "-4", "2.5"):
            self.assertEqual(threshold(raw), DEFAULT_EDITS, raw)


class ScanTest(unittest.TestCase):
    def test_no_spec_call_is_no_task(self):
        self.assertIsNone(scan(edits(12)))
        self.assertIsNone(scan([]))

    def test_counts_gated_edits_after_the_spec_call_only(self):
        lines = edits(4) + [tool_use(SPEC_TOOL, "spec1")] + [
            tool_use("Edit"), tool_use("Read"), tool_use("Write"), tool_use("NotebookEdit"), tool_use("Bash")]
        win = scan(lines)
        self.assertEqual(win.edits, 3)
        self.assertEqual(win.key, "spec1")
        self.assertFalse(win.checked)
        self.assertFalse(win.completing)

    def test_the_last_spec_call_opens_a_new_window(self):
        lines = ([tool_use(SPEC_TOOL, "spec1")] + edits(7) + [tool_use(PROGRESS_TOOL), tool_use(COMPLETION_TOOL)]
                 + [tool_use(SPEC_TOOL, "spec2")] + edits(2))
        win = scan(lines)
        self.assertEqual((win.key, win.edits, win.checked, win.completing), ("spec2", 2, False, False))

    def test_check_progress_and_validate_completion_are_seen(self):
        win = scan([tool_use(SPEC_TOOL)] + edits(2) + [tool_use(PROGRESS_TOOL)])
        self.assertTrue(win.checked)
        win = scan([tool_use(SPEC_TOOL)] + edits(2) + [tool_use(COMPLETION_TOOL)] + edits(1))
        self.assertTrue(win.completing)
        self.assertEqual(win.edits, 3)

    def test_text_naming_a_tool_is_not_a_call(self):
        lines = [user('call ' + SPEC_TOOL + ' via "tool_use"')] + edits(3)
        self.assertIsNone(scan(lines))
        win = scan([tool_use(SPEC_TOOL)] + [user('"tool_use" ' + PROGRESS_TOOL)])
        self.assertFalse(win.checked)

    def test_a_tool_result_is_not_a_call(self):
        result = json.dumps({"type": "user", "message": {"content": [
            {"type": "tool_result", "tool_use_id": "t1", "content": PROGRESS_TOOL}]}})
        win = scan([tool_use(SPEC_TOOL), result])
        self.assertFalse(win.checked)

    def test_several_tool_uses_in_one_entry_all_count(self):
        batch = json.dumps({"type": "assistant", "message": {"content": [
            {"type": "tool_use", "id": "a", "name": "Edit", "input": {}},
            {"type": "text", "text": "and"},
            {"type": "tool_use", "id": "b", "name": "Write", "input": {}}]}})
        self.assertEqual(scan([tool_use(SPEC_TOOL), batch]).edits, 2)

    def test_malformed_lines_and_entries_are_skipped(self):
        lines = ['not json "tool_use"', "{", tool_use(SPEC_TOOL),
                 json.dumps({"type": "assistant", "message": "tool_use"}),
                 json.dumps({"type": "assistant", "message": {"content": "tool_use"}}),
                 json.dumps(["tool_use"]),
                 tool_use("Edit")]
        self.assertEqual(scan(lines).edits, 1)

    def test_a_spec_call_without_an_id_is_keyed_by_its_line(self):
        entry = json.dumps({"type": "assistant", "message": {"content": [
            {"type": "tool_use", "name": SPEC_TOOL, "input": {}}]}})
        self.assertEqual(scan([user("x"), entry]).key, "line-1")


class DecideTest(unittest.TestCase):
    def win(self, lines):
        return scan(lines)

    def never(self):
        raise AssertionError("claim must not be tried")

    def test_no_task_skips(self):
        self.assertEqual(decide(None, 10, self.never), (3, "no-task"))

    def test_a_task_in_its_completion_loop_skips(self):
        win = self.win([tool_use(SPEC_TOOL)] + edits(12) + [tool_use(COMPLETION_TOOL)])
        self.assertEqual(decide(win, 10, self.never), (3, "completing"))

    def test_below_the_threshold_passes(self):
        win = self.win([tool_use(SPEC_TOOL)] + edits(9))
        self.assertEqual(decide(win, 10, self.never), (0, "edits=9"))

    def test_a_checked_task_passes_however_many_edits(self):
        win = self.win([tool_use(SPEC_TOOL)] + edits(5) + [tool_use(PROGRESS_TOOL)] + edits(20))
        self.assertEqual(decide(win, 10, self.never), (0, "edits=25"))

    def test_the_threshold_edit_asks_when_the_claim_is_new(self):
        win = self.win([tool_use(SPEC_TOOL)] + edits(10))
        self.assertEqual(decide(win, 10, lambda: True), (2, "edits=10"))

    def test_an_edit_past_the_threshold_still_asks_when_nothing_asked_yet(self):
        # Edits sent in one turn can carry the count past the threshold
        # without any hook having seen it exactly.
        win = self.win([tool_use(SPEC_TOOL)] + edits(13))
        self.assertEqual(decide(win, 10, lambda: True), (2, "edits=13"))

    def test_a_task_already_asked_passes(self):
        win = self.win([tool_use(SPEC_TOOL)] + edits(11))
        self.assertEqual(decide(win, 10, lambda: False), (0, "edits=11"))

    def test_an_ask_that_cannot_be_recorded_is_not_made(self):
        win = self.win([tool_use(SPEC_TOOL)] + edits(10))

        def broken():
            raise OSError("read-only")
        self.assertEqual(decide(win, 10, broken), (3, "no-state"))


class OwnTranscriptTest(unittest.TestCase):
    def test_the_main_session_reads_its_own_transcript(self):
        self.assertEqual(own_transcript({"transcript_path": "/p/s1.jsonl"}), "/p/s1.jsonl")

    def test_a_subagent_reads_the_file_beside_the_parent(self):
        with tempfile.TemporaryDirectory() as tmp:
            parent = os.path.join(tmp, "s1.jsonl")
            sub = os.path.join(tmp, "s1", "subagents", "agent-a1.jsonl")
            os.makedirs(os.path.dirname(sub))
            open(sub, "w").close()
            self.assertEqual(own_transcript({"transcript_path": parent, "agent_id": "a1"}), sub)

    def test_a_subagent_handed_its_own_transcript_reads_it(self):
        path = "/p/s1/subagents/agent-a1.jsonl"
        self.assertEqual(own_transcript({"transcript_path": path, "agent_id": "a1"}), path)

    def test_a_subagent_with_no_transcript_of_its_own_never_reads_the_parent(self):
        self.assertEqual(own_transcript({"transcript_path": "/p/s1.jsonl", "agent_id": "a1"}), "")

    def test_a_payload_without_a_usable_path_is_refused(self):
        for data in ({}, {"transcript_path": ""}, {"transcript_path": 5}, {"transcript_path": "/p/s1.jsonl", "agent_id": "../x"}):
            self.assertEqual(own_transcript(data), "", data)


class ClaimTest(unittest.TestCase):
    def test_only_the_first_claim_for_a_task_wins(self):
        with tempfile.TemporaryDirectory() as tmp:
            self.assertTrue(claim(tmp, "/p/s1.jsonl", "spec1"))
            self.assertFalse(claim(tmp, "/p/s1.jsonl", "spec1"))
            self.assertTrue(claim(tmp, "/p/s1.jsonl", "spec2"), "a new task in the same transcript")
            self.assertTrue(claim(tmp, "/p/s2.jsonl", "spec1"), "the same call id in another transcript")

    def test_a_missing_directory_raises(self):
        with tempfile.TemporaryDirectory() as tmp:
            with self.assertRaises(OSError):
                claim(os.path.join(tmp, "absent"), "/p/s1.jsonl", "spec1")

    def test_a_symlink_at_the_stamp_path_is_not_followed(self):
        if not hasattr(os, "O_NOFOLLOW"):
            self.skipTest("no O_NOFOLLOW on this platform")
        with tempfile.TemporaryDirectory() as tmp:
            self.assertTrue(claim(tmp, "/p/s1.jsonl", "spec1"))
            stamp = os.path.join(tmp, os.listdir(tmp)[0])
            os.unlink(stamp)
            target = os.path.join(tmp, "target")
            os.symlink(target, stamp)
            self.assertFalse(claim(tmp, "/p/s1.jsonl", "spec1"))
            self.assertFalse(os.path.exists(target))


if __name__ == "__main__":
    unittest.main()
