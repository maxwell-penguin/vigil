# vigil-sdk (JS/TS)

JavaScript/TypeScript client for [vigil](https://github.com/) — batches events
in memory and flushes them to your vigil server every 10 seconds. Works in
Node.js (18+) and browsers via the global `fetch`. `track()` never blocks and
never throws.

## Install

```
npm install vigil-sdk
```

## Usage

```ts
import { VigilClient } from "vigil-sdk";

const client = new VigilClient("my-app", "http://localhost:8080");

client.track(42, 200);
client.track(830, 500, true);
```
