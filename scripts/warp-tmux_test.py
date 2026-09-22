#!/usr/bin/env python3
"""Check patched emitters: python3 scripts/warp-tmux_test.py CLAUDE_ROOT CODEX_ROOT."""

import errno
import json
import os
from pathlib import Path
import pty
import select
import shlex
import shutil
import signal
import subprocess
import sys
import tempfile
import time


def terminal_output(command, env, ready=None):
    pid, fd = pty.fork()
    if pid == 0:
        os.execvpe(command[0], command, env)
    output = b""
    try:
        if ready:
            ready()
        deadline = time.monotonic() + 5
        while time.monotonic() < deadline:
            if not select.select([fd], [], [], 0.1)[0]:
                continue
            try:
                chunk = os.read(fd, 65536)
            except OSError as error:
                if error.errno != errno.EIO:
                    raise
                break
            if not chunk:
                break
            output += chunk
        else:
            raise AssertionError("emitter timed out")
        _, status = os.waitpid(pid, 0)
        pid = 0
        assert status == 0, (status, output)
        return output
    finally:
        os.close(fd)
        if pid:
            os.kill(pid, signal.SIGKILL)
            os.waitpid(pid, 0)


def through_tmux(root, env, agent):
    tmux = shutil.which("tmux")
    assert tmux, "tmux >= 3.3 is required"
    with tempfile.TemporaryDirectory(prefix="dbw-") as directory:
        command = [tmux, "-S", str(Path(directory) / "s"), "-f", "/dev/null"]
        raw_probe = "\x1b]777;notify;warp://cli-agent;raw-probe\x07"
        events = []
        emit = [shlex.join(command + ["wait-for", "ready"]), "printf %s " + shlex.quote(raw_probe)]
        for event in ("session_start", "prompt_submit", "permission_request", "tool_complete", "stop"):
            body = json.dumps({"v": 1, "agent": agent, "event": event, "session_id": "transport-test"})
            events.append(f"\x1b]777;notify;warp://cli-agent;{body}\x07".encode())
            call = shlex.join(["bash", str(root / "scripts/warp-notify.sh"), "warp://cli-agent", body])
            emit.append(call + (" | jq -rj .terminalSequence" if agent == "claude" else ""))
        # Let tmux drain the pane output before detaching this test client.
        emit.extend(["sleep 0.2", shlex.join(command + ["detach-client", "-s", "smoke"]), "exec sleep 10"])
        env = dict(env, TERM="xterm-256color", TMUX="", CLAUDE_CODE_VERSION="2.1.141")
        try:
            subprocess.run(command + ["new-session", "-d", "-s", "smoke", shlex.join(["bash", "-o", "pipefail", "-c", "; ".join(emit)])], env=env, check=True)
            subprocess.run(command + ["set-option", "-g", "allow-passthrough", "on"], check=True)

            def ready():
                deadline = time.monotonic() + 3
                while time.monotonic() < deadline:
                    if subprocess.check_output(command + ["list-clients"]):
                        subprocess.run(command + ["wait-for", "-S", "ready"], check=True)
                        return
                    time.sleep(0.01)
                raise AssertionError("tmux client did not attach")

            output = terminal_output(command + ["attach-session", "-t", "smoke"], env, ready)
            assert raw_probe.encode() not in output, "raw OSC unexpectedly passed through"
            for event in events:
                assert event in output, (agent, "tmux dropped event", event, output)
        finally:
            subprocess.run(command + ["kill-server"], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)


def main():
    claude, codex = sys.argv[1:]
    scripts = Path(__file__).resolve().parent
    env = dict(os.environ, WARP_CLI_AGENT_PROTOCOL_VERSION="1", WARP_CLIENT_VERSION="v999")
    with tempfile.TemporaryDirectory(prefix="warp-emitters-") as directory:
        for agent, source in (("claude", claude), ("codex", codex)):
            root = Path(directory) / agent
            shutil.copytree(Path(source) / "scripts", root / "scripts")
            patch = scripts / f"warp-{agent}-tmux.patch"
            subprocess.run(["patch", "--batch", "--forward", "-p1", "-d", str(root), "-i", str(patch)], check=True)
            for tmux in ("", "/tmp/test-socket,123,0"):
                for version in (("2.1.140", "2.1.141", "") if agent == "claude" else ("",)):
                    for event in ("session_start", "prompt_submit", "permission_request", "tool_complete", "stop"):
                        body = json.dumps({"v": 1, "agent": agent, "event": event, "session_id": "transport-test"})
                        raw = f"\x1b]777;notify;warp://cli-agent;{body}\x07".encode()
                        expected = b"\x1bPtmux;" + raw.replace(b"\x1b", b"\x1b\x1b") + b"\x1b\\" if tmux else raw
                        env.update(TMUX=tmux, CLAUDE_CODE_VERSION=version)
                        output = terminal_output(["bash", str(root / "scripts/warp-notify.sh"), "warp://cli-agent", body], env)
                        if agent == "claude" and version == "2.1.141":
                            output = json.loads(output)["terminalSequence"].encode()
                        assert output == expected, (agent, tmux, version, event, output)
            through_tmux(root, env, agent)
    print("PASS: both plugins, five events, native/real tmux, Claude tty and JSON delivery")


if __name__ == "__main__":
    main()
