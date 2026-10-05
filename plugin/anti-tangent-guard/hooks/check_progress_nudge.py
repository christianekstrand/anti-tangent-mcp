"""PostToolUse body: once per task, ask for a check_progress call when the
task reaches its Nth edit without one.

Reads the hook payload as JSON on stdin; the wrapper consumes the hook's own
stdin and re-feeds it here, so nothing else in this process may read stdin.
Exit 0 = nothing to ask, 2 = ask (the message is on stderr), 3 = not decided
(no task in the transcript, a task already in its completion loop, no readable
transcript, an ungated tool, no place to record the ask). The wrapper maps
every other exit to allow. One word for the trace line goes to stdout.

The transcript read is the session's own; own_transcript says how it is found.

A task is the window after the LAST validate_task_spec call. The edit that
triggered this hook has already happened, so exit 2 undoes nothing: it hands
the message to the model.
"""
import hashlib
import json
import os
import sys

# The wrapper runs this under python3 -I, which leaves the script's own
# directory off sys.path, so the sibling module is put back by hand.
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from check_task_start import as_list, subagent_transcript  # noqa: E402

SPEC_TOOL = "mcp__anti-tangent__validate_task_spec"
PROGRESS_TOOL = "mcp__anti-tangent__check_progress"
COMPLETION_TOOL = "mcp__anti-tangent__validate_completion"
GATED_TOOLS = ("Edit", "Write", "NotebookEdit")
DEFAULT_EDITS = 10

NUDGE_MESSAGE = """CHECKPOINT DUE: call check_progress

This task has made {edits} edits since validate_task_spec and has not called
check_progress. Call mcp__anti-tangent__check_progress now with the session_id
validate_task_spec returned, a one-sentence working_on, and changed_files
holding every file changed so far. Act on its findings, then carry on.

The edit you just made is kept. This is asked once per task.

(Disable: ANTI_TANGENT_PROGRESS_GUARD=0. Threshold: ANTI_TANGENT_PROGRESS_EDITS,
default {default}. Trace: {trace})
"""


def threshold(raw):
    """Return the edit count that triggers the ask: raw when it is a whole
    number above zero, else the default."""
    try:
        n = int(str(raw).strip())
    except (TypeError, ValueError):
        return DEFAULT_EDITS
    return n if n > 0 else DEFAULT_EDITS


class Window:
    """What the transcript shows since the last validate_task_spec call."""

    def __init__(self):
        self.key = ""
        self.edits = 0
        self.checked = False
        self.completing = False


def scan(lines):
    """Return the Window after the last validate_task_spec tool_use in an
    iterable of transcript lines, or None when there is no such call.

    Only assistant tool_use parts count; text naming a tool is not a call. A
    line without a quoted "tool_use" is skipped before it is parsed, which
    keeps a long transcript cheap: this runs after every edit, and the quotes
    leave out the far more common tool_use_id of a tool result. A malformed
    line or entry is skipped rather than raised on.
    """
    win = None
    for number, line in enumerate(lines):
        if '"tool_use"' not in line:
            continue
        try:
            entry = json.loads(line)
        except Exception:
            continue
        if not isinstance(entry, dict) or entry.get("type") != "assistant":
            continue
        msg = entry.get("message")
        if not isinstance(msg, dict):
            continue
        for part in as_list(msg.get("content")):
            if not isinstance(part, dict) or part.get("type") != "tool_use":
                continue
            name = part.get("name")
            if name == SPEC_TOOL:
                win = Window()
                call_id = part.get("id")
                win.key = call_id if isinstance(call_id, str) and call_id else "line-%d" % number
            elif win is None:
                continue
            elif name == PROGRESS_TOOL:
                win.checked = True
            elif name == COMPLETION_TOOL:
                win.completing = True
            elif name in GATED_TOOLS:
                win.edits += 1
    return win


def own_transcript(data):
    """Return the path of the transcript of the session the payload came from,
    or "" when it cannot be told.

    With no agent_id the payload is the main session's and transcript_path is
    its transcript. Inside a subagent, transcript_path is the parent's and the
    subagent's own sits beside it at <parent stem>/subagents/agent-<id>.jsonl.
    When that file is absent but transcript_path itself names an agent
    transcript, the host has handed over the subagent's own path and it is
    used as it is. Anything else is "": reading the parent's transcript for a
    subagent would judge the wrong session.
    """
    parent = data.get("transcript_path")
    if not isinstance(parent, str) or not parent:
        return ""
    if not data.get("agent_id"):
        return parent
    derived = subagent_transcript(data)
    if derived and os.path.isfile(derived):
        return derived
    if os.path.basename(parent).startswith("agent-"):
        return parent
    return ""


def claim(directory, transcript, key):
    """Record that this task has been asked, and return True when this call is
    the one that recorded it. False means an earlier call already asked. Raises
    OSError when the record cannot be written.

    The exclusive create is what makes the ask happen once: edits sent in one
    assistant turn run their hooks at the same time and all read the same
    count, and only one of them creates the file.
    """
    digest = hashlib.sha256(("%s\0%s" % (transcript, key)).encode("utf-8")).hexdigest()[:16]
    stamp = os.path.join(directory, "progress-asked-%s" % digest)
    flags = os.O_WRONLY | os.O_CREAT | os.O_EXCL | getattr(os, "O_NOFOLLOW", 0)
    try:
        os.close(os.open(stamp, flags, 0o600))
    except FileExistsError:
        return False
    return True


def decide(win, limit, try_claim):
    """Return (exit code, trace word) for a Window. try_claim() is called only
    when the task is due, and follows claim()'s contract."""
    if win is None:
        return 3, "no-task"
    if win.completing:
        return 3, "completing"
    detail = "edits=%d" % win.edits
    if win.checked or win.edits < limit:
        return 0, detail
    try:
        first = try_claim()
    except OSError:
        # An ask that cannot be recorded would repeat after every edit.
        return 3, "no-state"
    return (2, detail) if first else (0, detail)


def main():
    try:
        data = json.loads(sys.stdin.read())
    except Exception:
        return 3
    if not isinstance(data, dict) or (data.get("tool_name") or "") not in GATED_TOOLS:
        return 3
    path = own_transcript(data)
    if not path:
        return 3
    try:
        with open(path, encoding="utf-8", errors="replace") as fh:
            win = scan(fh)
    except OSError:
        return 3
    trace_log = os.environ.get("ATG_TRACE_LOG", "")
    limit = threshold(os.environ.get("ANTI_TANGENT_PROGRESS_EDITS"))

    def try_claim():
        if not trace_log:
            raise OSError("no state directory")
        return claim(os.path.dirname(trace_log), path, win.key)

    code, detail = decide(win, limit, try_claim)
    sys.stdout.write(detail)
    if code == 2:
        sys.stderr.write(NUDGE_MESSAGE.format(edits=win.edits, default=DEFAULT_EDITS, trace=trace_log))
    return code


if __name__ == "__main__":
    sys.exit(main())
