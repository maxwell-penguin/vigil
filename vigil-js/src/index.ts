const FLUSH_INTERVAL_MS = 10_000;
const RETRY_DELAY_MS = 1_000;

interface VigilEvent {
  timestamp: number;
  latency_ms: number;
  status_code: number;
  error: boolean;
}

/**
 * Batches track() calls and flushes them to a vigil server every 10s.
 * track() only ever pushes to an in-memory array — it never awaits network
 * I/O, so it never blocks the caller. Works in Node.js and browsers via the
 * global fetch (Node 18+).
 */
export class VigilClient {
  private events: VigilEvent[] = [];
  // Typed loosely: setInterval's return type differs between Node and DOM lib.
  private timer: any;

  constructor(private projectId: string, private vigilUrl: string) {
    this.timer = setInterval(() => this.flush(), FLUSH_INTERVAL_MS);
    // Don't keep a Node process alive just for this timer; no-op in browsers.
    if (typeof this.timer?.unref === "function") {
      this.timer.unref();
    }
  }

  track(latencyMs: number, statusCode: number, error = false): void {
    this.events.push({
      timestamp: Math.floor(Date.now() / 1000),
      latency_ms: latencyMs,
      status_code: statusCode,
      error,
    });
  }

  /** Stops the background flush timer, e.g. before process exit. */
  destroy(): void {
    clearInterval(this.timer);
  }

  private flush(): void {
    if (this.events.length === 0) return;
    const batch = this.events;
    this.events = [];

    this.post(batch).catch(() => {
      setTimeout(() => {
        this.post(batch).catch(() => {
          // second failure: drop silently
        });
      }, RETRY_DELAY_MS);
    });
  }

  private async post(events: VigilEvent[]): Promise<void> {
    const url = `${this.vigilUrl}/ingest?project_id=${encodeURIComponent(this.projectId)}`;
    const res = await fetch(url, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(events),
    });
    if (!res.ok) {
      throw new Error(`vigil ingest failed: ${res.status}`);
    }
  }
}
