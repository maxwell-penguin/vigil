"""Self-check for VigilClient batching/retry. Run with: python -m unittest"""

import json
import threading
import unittest
from http.server import BaseHTTPRequestHandler, HTTPServer

from vigil_sdk import VigilClient


def _start_server(handler_cls):
    server = HTTPServer(("127.0.0.1", 0), handler_cls)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    return server, thread


class TestFlushSendsBatch(unittest.TestCase):
    def test_batch_is_sent_with_error_field(self):
        received = []

        class Handler(BaseHTTPRequestHandler):
            def do_POST(self):
                length = int(self.headers["Content-Length"])
                body = self.rfile.read(length)
                received.append((self.path, json.loads(body)))
                self.send_response(202)
                self.end_headers()

            def log_message(self, *args):
                pass

        server, thread = _start_server(Handler)
        try:
            client = VigilClient("proj", f"http://127.0.0.1:{server.server_port}")
            client.track(latency_ms=42, status_code=200)
            client.track(latency_ms=900, status_code=500, error=True)
            client._flush()  # avoid waiting on the real 10s timer

            self.assertEqual(len(received), 1)
            path, batch = received[0]
            self.assertIn("project_id=proj", path)
            self.assertEqual(len(batch), 2)
            self.assertEqual(batch[1]["error"], True)
            self.assertEqual(batch[1]["status_code"], 500)
        finally:
            server.shutdown()
            thread.join()


class TestRetryThenDropSilently(unittest.TestCase):
    def test_retries_once_then_gives_up(self):
        attempts = []

        class Handler(BaseHTTPRequestHandler):
            def do_POST(self):
                attempts.append(1)
                self.send_response(500)
                self.end_headers()

            def log_message(self, *args):
                pass

        server, thread = _start_server(Handler)
        try:
            client = VigilClient("proj", f"http://127.0.0.1:{server.server_port}")
            client.track(latency_ms=1, status_code=200)
            client._flush()  # should try, sleep 1s, retry once, then drop

            self.assertEqual(len(attempts), 2)
        finally:
            server.shutdown()
            thread.join()


if __name__ == "__main__":
    unittest.main()
