"""Zero-config request auto-instrumentation for Flask.

    pip install flask vigil-sdk
    python flask_example.py
"""

import time

from flask import Flask, g, request
from vigil_sdk import VigilClient

app = Flask(__name__)
vigil = VigilClient(project_id="my-flask-app", vigil_url="http://localhost:8080")


@app.before_request
def _vigil_start_timer():
    g.vigil_start = time.monotonic()


@app.after_request
def _vigil_track_request(response):
    latency_ms = (time.monotonic() - g.vigil_start) * 1000
    vigil.track(latency_ms=latency_ms, status_code=response.status_code)
    return response


@app.route("/")
def index():
    return "ok"


if __name__ == "__main__":
    app.run()
