import os
import requests
import atexit

class AgentTracker:
    def __init__(self, backend_url="http://localhost:8080"):
        self.backend_url = backend_url
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
