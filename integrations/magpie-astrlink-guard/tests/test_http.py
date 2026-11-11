import http.client
import json
from http.server import ThreadingHTTPServer
from pathlib import Path
import sys
import threading
import unittest

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
from service import make_handler


class FakeScanner:
    backend, ep = 'cpu', 'CPUExecutionProvider'
    def detect(self, text):
        return [{'start': 0, 'end': 5, 'label': 'private_person', 'score': 0.9}] if text else []


class HttpTests(unittest.TestCase):
    def setUp(self):
        self.server = ThreadingHTTPServer(('127.0.0.1', 0), make_handler(FakeScanner(), 'secret'))
        self.thread = threading.Thread(target=self.server.serve_forever, daemon=True)
        self.thread.start()

    def tearDown(self):
        self.server.shutdown()
        self.server.server_close()
        self.thread.join(timeout=2)

    def request(self, method, path, body=None, token='secret'):
        conn = http.client.HTTPConnection('127.0.0.1', self.server.server_port)
        headers = {'Authorization': f'Bearer {token}'} if token else {}
        if body is not None:
            headers['Content-Type'] = 'application/json'
        conn.request(method, path, body, headers)
        response = conn.getresponse()
        result = response.status, json.loads(response.read())
        conn.close()
        return result

    def test_authentication_and_aggregates(self):
        self.assertEqual(self.request('POST', '/scan', '{"texts":["Alice"]}', token='wrong')[0], 401)
        status, result = self.request('POST', '/scan', '{"texts":["Alice"]}')
        self.assertEqual(status, 200)
        self.assertEqual(result['spans'][0][0]['label'], 'private_person')
        status, stats = self.request('GET', '/stats')
        self.assertEqual(stats['requests'], 1)
        self.assertEqual(stats['spansByLabel']['private_person'], 1)
        self.assertEqual(stats['texts'], 1)
        self.assertEqual(self.request('GET', '/stats', token='wrong')[0], 401)

    def test_invalid_payload(self):
        self.assertEqual(self.request('POST', '/scan', '{"texts":"not-a-list"}')[0], 422)

if __name__ == '__main__':
    unittest.main()
