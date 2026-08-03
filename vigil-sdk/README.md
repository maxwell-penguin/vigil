# vigil-sdk

Python client for [vigil](https://github.com/) — batches events in memory and
flushes them to your vigil server every 10 seconds on a background thread.
`track()` never blocks the calling thread and never raises.

## Install

```
pip install vigil-sdk
```

## Usage

```python
from vigil_sdk import VigilClient

client = VigilClient(project_id="my-app", vigil_url="http://localhost:8080")

client.track(latency_ms=42, status_code=200)
client.track(latency_ms=830, status_code=500, error=True)
```

See `examples/flask_example.py` for zero-config request auto-instrumentation.
