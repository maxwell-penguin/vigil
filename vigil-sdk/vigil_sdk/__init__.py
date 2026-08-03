"""Minimal client for pushing events to a vigil server's /ingest endpoint."""

import json
import threading
import time
import urllib.error
import urllib.request

__all__ = ["VigilClient"]

FLUSH_INTERVAL_SECONDS = 10.0
RETRY_DELAY_SECONDS = 1.0
REQUEST_TIMEOUT_SECONDS = 5.0


class VigilClient:
    """Batches track() calls and flushes them to vigil every 10s.

    All network I/O happens on a background daemon thread — track() only
    ever appends to an in-memory list, so it never blocks the caller.
    """

    def __init__(self, project_id, vigil_url):
        self.project_id = project_id
        self.vigil_url = vigil_url.rstrip("/")
        self._lock = threading.Lock()
        self._events = []
        self._schedule_flush()

    def track(self, latency_ms, status_code, error=False):
        event = {
            "timestamp": int(time.time()),
            "latency_ms": latency_ms,
            "status_code": status_code,
            "error": error,
        }
        with self._lock:
            self._events.append(event)

    def _schedule_flush(self):
        # threading.Timer is one-shot, so each flush re-arms the next one —
        # that's what makes this a recurring 10s cadence.
        timer = threading.Timer(FLUSH_INTERVAL_SECONDS, self._flush_and_reschedule)
        timer.daemon = True
        timer.start()

    def _flush_and_reschedule(self):
        self._flush()
        self._schedule_flush()

    def _flush(self):
        with self._lock:
            if not self._events:
                return
            events, self._events = self._events, []

        if self._post(events):
            return
        time.sleep(RETRY_DELAY_SECONDS)
        self._post(events)  # drop silently on second failure

    def _post(self, events):
        url = "{}/ingest?project_id={}".format(self.vigil_url, self.project_id)
        body = json.dumps(events).encode("utf-8")
        req = urllib.request.Request(
            url, data=body, method="POST",
            headers={"Content-Type": "application/json"},
        )
        try:
            with urllib.request.urlopen(req, timeout=REQUEST_TIMEOUT_SECONDS) as resp:
                return 200 <= resp.status < 300
        except Exception:
            return False
