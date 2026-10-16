#!/usr/bin/env python3
"""Local, synthetic-only WebSocket event feed for CodeCity display smoke tests.

No privileged collector or real user data is involved. Binds to loopback only.
"""
from __future__ import annotations

import argparse
import base64
import hashlib
import json
import socketserver
import struct
import time
from http import HTTPStatus

MAGIC = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"


def websocket_frame(payload: bytes) -> bytes:
    length = len(payload)
    if length < 126:
        return bytes((0x81, length)) + payload
    if length < 65536:
        return b"\x81\x7e" + struct.pack("!H", length) + payload
    return b"\x81\x7f" + struct.pack("!Q", length) + payload


def event_batch(seq: int) -> bytes:
    now = int(time.time() * 1000)
    pid = 421 if seq % 3 == 0 else 422
    kind = "file_write" if seq % 2 == 0 else "file_read"
    target = ["src/main.rs", "src/view.rs", "src/agent.rs"][seq % 3]
    events = [{
        "eventId": f"smoke-{seq}",
        "type": kind,
        "network": False,
        "target": f"/workspaces/codecity-demo/{target}",
        "rootAgentPid": 421,
        "pid": pid,
        "ppid": 421 if pid != 421 else 1,
        "agentRunId": "smoke-run",
        "receivedAtMs": now,
    }]
    if seq in (2, 8):
        events.append({
            "eventId": f"smoke-hook-{seq}",
            "type": "native_hook",
            "comm": "SubagentStart:Explore" if seq == 2 else "SubagentStop:Explore",
            "subagentId": "sha256:0123456789abcdef",
            "rootAgentPid": 421,
            "pid": 421,
            "agentRunId": "smoke-run",
            "receivedAtMs": now,
        })
    return json.dumps({"events": events}, separators=(",", ":")).encode("utf-8")


class Handler(socketserver.BaseRequestHandler):
    def handle(self) -> None:
        self.request.settimeout(8)
        buffer = b""
        while b"\r\n\r\n" not in buffer:
            chunk = self.request.recv(4096)
            if not chunk:
                return
            buffer += chunk
            if len(buffer) > 16384:
                return
        lines = buffer.split(b"\r\n")
        request = lines[0].decode("ascii", errors="replace")
        if not request.startswith("GET /ws/event-summaries "):
            self.request.sendall(b"HTTP/1.1 404 Not Found\r\nContent-Length: 0\r\n\r\n")
            return
        headers: dict[str, str] = {}
        for line in lines[1:]:
            if b":" in line:
                key, val = line.split(b":", 1)
                headers[key.decode("ascii", "ignore").strip().lower()] = val.decode(
                    "ascii", "ignore"
                ).strip()
        key = headers.get("sec-websocket-key")
        if not key or headers.get("upgrade", "").lower() != "websocket":
            self.request.sendall(b"HTTP/1.1 400 Bad Request\r\nContent-Length: 0\r\n\r\n")
            return
        digest = hashlib.sha1((key + MAGIC).encode("ascii")).digest()
        accept = base64.b64encode(digest).decode("ascii")
        self.request.sendall((
            "HTTP/1.1 101 Switching Protocols\r\n"
            "Upgrade: websocket\r\nConnection: Upgrade\r\n"
            f"Sec-WebSocket-Accept: {accept}\r\n\r\n"
        ).encode("ascii"))
        self.request.settimeout(None)
        for seq in range(240):
            try:
                self.request.sendall(websocket_frame(event_batch(seq)))
            except (BrokenPipeError, ConnectionResetError, OSError):
                break
            time.sleep(0.35)


class Server(socketserver.ThreadingTCPServer):
    allow_reuse_address = True
    daemon_threads = True


def main() -> None:
    p = argparse.ArgumentParser()
    p.add_argument("--port", type=int, default=18765)
    args = p.parse_args()
    with Server(("127.0.0.1", args.port), Handler) as server:
        print(f"synthetic CodeCity WS ready at 127.0.0.1:{args.port}", flush=True)
        server.serve_forever()


if __name__ == "__main__":
    main()
