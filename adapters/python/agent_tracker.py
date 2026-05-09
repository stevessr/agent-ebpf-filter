import os
import requests
import atexit
import hashlib
from pathlib import Path


def _resolve_backend_url(explicit_url=None):
    if explicit_url:
        return explicit_url

    env_url = os.getenv("AGENT_BACKEND_URL")
    if env_url:
        return env_url

    try:
        port_file = Path(__file__).resolve().parents[2] / "backend" / ".port"
        if port_file.exists():
            port = port_file.read_text(encoding="utf-8").strip()
            if port.isdigit():
                return f"http://127.0.0.1:{port}"
    except Exception:
        pass

    return "http://127.0.0.1:8080"


def _first_env(*keys):
    for key in keys:
        value = os.getenv(key, "").strip()
        if value:
            return value
    return ""


def _env_number(*keys):
    raw = _first_env(*keys)
    if not raw:
        return 0
    try:
        value = float(raw)
        return value if value > 0 else 0
    except ValueError:
        return 0


def _build_digest(*parts):
    normalized = [str(part).strip() for part in parts if str(part).strip()]
    if not normalized:
        return ""
    return hashlib.sha256("\x00".join(normalized).encode("utf-8")).hexdigest()


def _resolve_api_token(context=None):
    context = context or {}
    explicit = str(context.get("access_token") or context.get("api_token") or "").strip()
    if explicit:
        return explicit
    return _first_env("AGENT_API_KEY", "AGENT_EBPF_ACCESS_TOKEN")

class AgentTracker:
    def __init__(self, backend_url=None, context=None):
        self.backend_url = _resolve_backend_url(backend_url)
        self.pid = os.getpid()
        self.registered = False
        self.context = context or {}

    def _build_payload(self):
        return {
            "pid": self.pid,
            "root_agent_pid": self.context.get("root_agent_pid") or int(_env_number("AGENT_EBPF_ROOT_AGENT_PID", "ROOT_AGENT_PID")),
            "agent_run_id": self.context.get("agent_run_id") or _first_env("AGENT_EBPF_AGENT_RUN_ID", "AGENT_RUN_ID"),
            "task_id": self.context.get("task_id") or _first_env("AGENT_EBPF_TASK_ID", "AGENT_TASK_ID"),
            "conversation_id": self.context.get("conversation_id") or _first_env("AGENT_EBPF_CONVERSATION_ID", "AGENT_CONVERSATION_ID"),
            "turn_id": self.context.get("turn_id") or _first_env("AGENT_EBPF_TURN_ID", "AGENT_TURN_ID"),
            "tool_call_id": self.context.get("tool_call_id") or _first_env("AGENT_EBPF_TOOL_CALL_ID", "AGENT_TOOL_CALL_ID"),
            "tool_name": self.context.get("tool_name") or _first_env("AGENT_EBPF_TOOL_NAME", "AGENT_TOOL_NAME"),
            "trace_id": self.context.get("trace_id") or _first_env("AGENT_EBPF_TRACE_ID", "TRACE_ID"),
            "span_id": self.context.get("span_id") or _first_env("AGENT_EBPF_SPAN_ID", "SPAN_ID"),
            "decision": self.context.get("decision") or _first_env("AGENT_EBPF_DECISION", "AGENT_DECISION"),
            "risk_score": self.context.get("risk_score") or _env_number("AGENT_EBPF_RISK_SCORE", "AGENT_RISK_SCORE"),
            "container_id": self.context.get("container_id") or _first_env("AGENT_EBPF_CONTAINER_ID", "CONTAINER_ID"),
            "cwd": self.context.get("cwd") or _first_env("AGENT_EBPF_CWD", "PWD") or os.getcwd(),
            "argv_digest": self.context.get("argv_digest") or _build_digest(
                self.context.get("tool_name") or _first_env("AGENT_EBPF_TOOL_NAME", "AGENT_TOOL_NAME"),
                self.context.get("tool_call_id") or _first_env("AGENT_EBPF_TOOL_CALL_ID", "AGENT_TOOL_CALL_ID"),
                self.context.get("agent_run_id") or _first_env("AGENT_EBPF_AGENT_RUN_ID", "AGENT_RUN_ID"),
            ),
        }

    def start(self):
        try:
            api_token = _resolve_api_token(self.context)
            headers = {}
            if api_token:
                headers["X-API-KEY"] = api_token
                headers["Authorization"] = f"Bearer {api_token}"
            response = requests.post(f"{self.backend_url}/register", json=self._build_payload(), headers=headers)
            if response.status_code == 200:
                print(f"AgentTracker: successfully registered PID {self.pid}")
                self.registered = True
                atexit.register(self.stop)
            else:
                print(f"AgentTracker: failed to register PID {self.pid}. Status: {response.status_code}")
        except Exception as e:
            print(f"AgentTracker: Error connecting to backend - {e}")

    def stop(self):
        if not self.registered:
            return
        try:
            api_token = _resolve_api_token(self.context)
            headers = {}
            if api_token:
                headers["X-API-KEY"] = api_token
                headers["Authorization"] = f"Bearer {api_token}"
            response = requests.post(f"{self.backend_url}/unregister", json={"pid": self.pid}, headers=headers)
            if response.status_code == 200:
                print(f"AgentTracker: successfully unregistered PID {self.pid}")
                self.registered = False
        except Exception as e:
            pass

if __name__ == "__main__":
    import time
    tracker = AgentTracker()
    tracker.start()
    print("Doing some work... creating files")
    with open("/tmp/agent_test.txt", "w") as f:
        f.write("test")
    time.sleep(2)
    print("Done")
