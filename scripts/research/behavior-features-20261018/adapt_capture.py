"""Map existing archived Event / TLS JSONL to the canonical offline contract.

No invented FD resolution, process-start identity, HTTP requests from IP/SNI,
or labels from decisions. Unsupported records are counted, never echoed.
"""
import argparse
import json
import re
import sys
from collections import Counter

SYSCALLS = set("read write readv writev pread64 pwrite64 preadv pwritev preadv2 pwritev2 open openat openat2 creat exec execve execveat unlink unlinkat rmdir rename renameat renameat2 chmod fchmod fchmodat chown fchown lchown fchownat".split())


def adapt(record, fmt):
    e = record.get("event", record) if fmt == "archive" else record
    if not isinstance(e, dict):
        return None, "invalid_event"
    out = {"source": str(e.get("capture_source", "unknown")), "context": {
        "run_id": str(e.get("agent_run_id", "")), "tgid": e.get("tgid", 0),
    }}
    if fmt == "archive" and "kernel_timestamp_ns" in e:
        out["context"].update(event_time_ns=int(e["kernel_timestamp_ns"]), clock=str(e.get("kernel_clock", "")))
    if "kernel_dropped_since_last" in e:
        out["capture_loss"] = int(e["kernel_dropped_since_last"]) > 0
    name = e.get("type", "")
    if fmt == "archive" and name in SYSCALLS:
        s = {"name": name, "relative_base_unknown": True}
        if isinstance(e.get("path_truncated"), bool):
            s["path_truncated"] = e["path_truncated"]
        for src, dst in [("path", "path"), ("extra_path", "target_path"), ("cwd", "cwd")]:
            if isinstance(e.get(src), str) and e[src]:
                s[dst] = e[src]
        if "retval" in e:
            s["return"] = int(e["retval"])
        if name in {"read", "write"} and "bytes" in e:
            s["requested_bytes"] = int(e["bytes"])
        if name == "open":
            m = re.fullmatch(r"flags=0x([0-9a-fA-F]+) mode=0[0-7]+", str(e.get("extra_info", "")))
            if m:
                s["open_flags"] = int(m[1], 16)
        mode = str(e.get("mode", ""))
        if re.fullmatch(r"0[0-7]+", mode):
            s["permission_mode"] = int(mode, 8)
        # Event.path for read/write is not a proven FD->inode/path resolution.
        # No fd_path is fabricated, and absent proto3 return fields stay absent.
        out["syscall"] = s
        return out, "syscall"
    method = e.get("method" if fmt == "tls" else "http_method", "")
    target = e.get("request_path" if fmt == "tls" else "http_path")
    if fmt == "tls" and not target:
        target = e.get("url")
    if method or target:
        h = {"method": str(method)}
        if isinstance(target, str) and target:
            h["target"] = target
        host = e.get("host" if fmt == "tls" else "http_host")
        if isinstance(host, str) and host:
            h["host"] = host
        if e.get("dst_ip"):
            h["destination_ip"] = e["dst_ip"]
        direction = e.get("direction" if fmt == "tls" else "net_direction")
        if direction in ("outgoing", "incoming"):
            h["direction"] = direction
        if fmt == "tls":
            headers = e.get("headers")
            if isinstance(headers, dict):
                h.update(headers=[{"name": k, "value": v} for k, v in headers.items()],
                         headers_present=True, headers_preserve_duplicates=False)
            if isinstance(e.get("body"), str):
                h["body"] = e["body"]
            if isinstance(e.get("truncated"), bool):
                h["truncated"] = e["truncated"]
        out["http"] = h
        return out, "http_request"
    return None, "no_syscall_or_request_evidence"


def main():
    p = argparse.ArgumentParser()
    p.add_argument("--format", choices=["archive", "tls"], required=True)
    args = p.parse_args()
    counts = Counter()
    for line in sys.stdin:
        if len(line) > 512 * 1024:
            counts["oversized"] += 1
            continue
        try:
            record, status = adapt(json.loads(line), args.format)
            counts[status] += 1
            if record is not None:
                print(json.dumps(record, ensure_ascii=True))
        except (ValueError, TypeError, AttributeError, OverflowError):
            counts["invalid"] += 1
    print(json.dumps({"audit": dict(counts)}), file=sys.stderr)


if __name__ == "__main__":
    main()
