import os
import requests
import atexit
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

class AgentTracker:
    def __init__(self, backend_url=None):
        self.backend_url = _resolve_backend_url(backend_url)
        self.pid = os.getpid()
        self.registered = False

    def start(self):
        try:
            response = requests.post(f"{self.backend_url}/register", json={"pid": self.pid})
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
            response = requests.post(f"{self.backend_url}/unregister", json={"pid": self.pid})
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
