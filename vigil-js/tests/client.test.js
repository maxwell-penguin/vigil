// Self-check for VigilClient batching/retry.
// Uses Node's built-in test runner and http server — zero extra deps.
// Run with: npm run build && npm test

import assert from "node:assert/strict";
import http from "node:http";
import test from "node:test";
import { VigilClient } from "../dist/index.js";

function startServer(onRequest) {
  const server = http.createServer((req, res) => {
    let body = "";
    req.on("data", (chunk) => (body += chunk));
    req.on("end", () => onRequest(req, res, body));
  });
  return new Promise((resolve) => {
    server.listen(0, "127.0.0.1", () => resolve(server));
  });
}

test("flush sends the batch with the error field", async () => {
  const received = [];
  const server = await startServer((req, res, body) => {
    received.push({ url: req.url, batch: JSON.parse(body) });
    res.writeHead(202);
    res.end();
  });

  try {
    const { port } = server.address();
    const client = new VigilClient("proj", `http://127.0.0.1:${port}`);
    client.track(42, 200);
    client.track(900, 500, true);
    client["flush"](); // private, but this is the unit under test — fire-and-forget by design
    await new Promise((r) => setTimeout(r, 100)); // let the loopback POST land

    assert.equal(received.length, 1);
    assert.ok(received[0].url.includes("project_id=proj"));
    assert.equal(received[0].batch.length, 2);
    assert.equal(received[0].batch[1].error, true);
    assert.equal(received[0].batch[1].status_code, 500);
    client.destroy();
  } finally {
    server.close();
  }
});

test("retries once then drops silently on repeated failure", async () => {
  let attempts = 0;
  const server = await startServer((req, res, body) => {
    attempts++;
    res.writeHead(500);
    res.end();
  });

  try {
    const { port } = server.address();
    const client = new VigilClient("proj", `http://127.0.0.1:${port}`);
    client.track(1, 200);
    client["flush"]();
    // the retry is scheduled 1s out — wait for it
    await new Promise((r) => setTimeout(r, 1200));

    assert.equal(attempts, 2);
    client.destroy();
  } finally {
    server.close();
  }
});
