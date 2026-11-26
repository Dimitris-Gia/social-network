// Owner: B
// Run with: npm test   (Node's built-in test runner, no extra packages)
import { test, beforeEach } from "node:test";
import assert from "node:assert/strict";
import { createSocketManager, defaultSocketUrl, STATUS } from "./socketManager.js";

// A fake WebSocket the tests control by hand.
class FakeSocket {
  static instances = [];
  readyState = 0;
  sent = [];
  closed = false;
  constructor(url) {
    this.url = url;
    FakeSocket.instances.push(this);
  }
  send(text) {
    this.sent.push(JSON.parse(text));
  }
  close() {
    this.closed = true;
    this.readyState = 3;
  }
  // --- helpers the tests call ---
  serverOpens() {
    this.readyState = 1;
    this.onopen?.();
  }
  serverSends(frame) {
    this.onmessage?.({ data: typeof frame === "string" ? frame : JSON.stringify(frame) });
  }
  serverDrops() {
    this.readyState = 3;
    this.onclose?.();
  }
}

// A fake clock: timers only run when the test says so.
let timers;
const setTimer = (fn, ms) => {
  timers.push({ fn, ms, cleared: false });
  return timers.length - 1;
};
const clearTimer = (id) => {
  timers[id].cleared = true;
};
const pendingTimers = () => timers.filter((t) => !t.cleared && !t.ran);
function runNextTimer() {
  const t = pendingTimers()[0];
  t.ran = true;
  t.fn();
  return t.ms;
}

function newManager(extra = {}) {
  return createSocketManager({
    url: "ws://test/ws",
    WebSocketImpl: FakeSocket,
    setTimer,
    clearTimer,
    random: () => 1, // no jitter: delays are exact
    ...extra,
  });
}
const lastSocket = () => FakeSocket.instances.at(-1);

beforeEach(() => {
  FakeSocket.instances = [];
  timers = [];
});

test("connect opens one socket, even when called twice", () => {
  const m = newManager();
  const seen = [];
  m.onStatusChange((s) => seen.push(s));

  m.connect();
  m.connect();
  assert.equal(FakeSocket.instances.length, 1);
  assert.equal(lastSocket().url, "ws://test/ws");
  assert.equal(m.getStatus(), STATUS.CONNECTING);

  lastSocket().serverOpens();
  assert.deepEqual(seen, [STATUS.CONNECTING, STATUS.OPEN]);
});

test("frames reach the subscribers of their type, with only the data", () => {
  const m = newManager();
  m.connect();
  lastSocket().serverOpens();

  const a = [];
  const b = [];
  const other = [];
  m.subscribe("private_message", (d) => a.push(d));
  const offB = m.subscribe("private_message", (d) => b.push(d));
  m.subscribe("notification", (d) => other.push(d));

  lastSocket().serverSends({ type: "private_message", data: { id: 1 } });
  assert.deepEqual(a, [{ id: 1 }]);
  assert.deepEqual(b, [{ id: 1 }]);
  assert.deepEqual(other, []);

  offB(); // unsubscribe
  lastSocket().serverSends({ type: "private_message", data: { id: 2 } });
  assert.deepEqual(a, [{ id: 1 }, { id: 2 }]);
  assert.deepEqual(b, [{ id: 1 }]);
});

test("a handler that throws does not stop the others", (t) => {
  t.mock.method(console, "error", () => {});
  const m = newManager();
  m.connect();
  lastSocket().serverOpens();

  const got = [];
  m.subscribe("x", () => {
    throw new Error("bug");
  });
  m.subscribe("x", (d) => got.push(d));
  lastSocket().serverSends({ type: "x", data: 1 });
  assert.deepEqual(got, [1]);
});

test("frames that are not valid are ignored", (t) => {
  t.mock.method(console, "warn", () => {});
  const m = newManager();
  m.connect();
  lastSocket().serverOpens();
  const got = [];
  m.subscribe("ok", (d) => got.push(d));

  lastSocket().serverSends("this is not json");
  lastSocket().serverSends({ data: 1 }); // no type
  lastSocket().serverSends(JSON.stringify(null));
  lastSocket().serverSends(JSON.stringify([1, 2]));
  lastSocket().serverSends({ type: 5, data: 1 }); // type is not a string
  lastSocket().serverSends({ type: "ok", data: "still works" });
  assert.deepEqual(got, ["still works"]);
});

test("send uses the {type, data} envelope and refuses when not open", () => {
  const m = newManager();
  assert.equal(m.send("private_message", {}), false); // never connected

  m.connect();
  assert.equal(m.send("private_message", {}), false); // still connecting
  lastSocket().serverOpens();

  assert.equal(m.send("private_message", { to: 5, content: "Hi 😀" }), true);
  assert.deepEqual(lastSocket().sent, [{ type: "private_message", data: { to: 5, content: "Hi 😀" } }]);

  lastSocket().serverDrops();
  assert.equal(m.send("private_message", {}), false); // dropped
});

test("after a drop it reconnects with a growing delay, capped, and resets after success", () => {
  const m = newManager({ baseDelay: 1000, maxDelay: 5000 });
  m.connect();
  lastSocket().serverOpens();

  lastSocket().serverDrops();
  assert.equal(m.getStatus(), STATUS.RECONNECTING);
  const delays = [];
  for (let i = 0; i < 5; i++) {
    delays.push(runNextTimer()); // time passes, a new socket is tried
    lastSocket().serverDrops(); // and fails again
  }
  assert.deepEqual(delays, [1000, 2000, 4000, 5000, 5000]); // doubles, then capped

  runNextTimer();
  lastSocket().serverOpens(); // success
  assert.equal(m.getStatus(), STATUS.OPEN);

  lastSocket().serverDrops();
  assert.equal(runNextTimer(), 1000); // back to the short delay
});

test("randomness only shortens the delay (between 50% and 100%)", () => {
  const m = newManager({ random: () => 0 });
  m.connect();
  lastSocket().serverDrops();
  assert.equal(runNextTimer(), 500);
});

test('"ws:reconnected" is sent when the connection comes back, not on the first open', () => {
  const m = newManager();
  let count = 0;
  m.subscribe("ws:reconnected", () => count++);

  m.connect();
  lastSocket().serverOpens();
  assert.equal(count, 0);

  lastSocket().serverDrops();
  runNextTimer();
  lastSocket().serverOpens();
  assert.equal(count, 1);

  lastSocket().serverDrops();
  runNextTimer();
  lastSocket().serverDrops(); // fails again: no event
  runNextTimer();
  lastSocket().serverOpens();
  assert.equal(count, 2);
});

test("disconnect closes for good: no reconnect, timer cleared, late events ignored", () => {
  const m = newManager();
  m.connect();
  const first = lastSocket();
  first.serverOpens();

  first.serverDrops();
  assert.equal(pendingTimers().length, 1); // a reconnect is planned
  m.disconnect();
  assert.equal(pendingTimers().length, 0); // ... and cancelled
  assert.equal(m.getStatus(), STATUS.CLOSED);

  // an open socket being closed on purpose
  const m2 = newManager();
  m2.connect();
  const sock = lastSocket();
  sock.serverOpens();
  m2.disconnect();
  assert.equal(sock.closed, true);
  sock.onclose?.(); // the browser reports the close afterwards
  assert.equal(pendingTimers().length, 0); // still no reconnect
  assert.equal(m2.getStatus(), STATUS.CLOSED);

  // late frames from the closed socket reach nobody
  const got = [];
  m2.subscribe("x", (d) => got.push(d));
  sock.serverSends({ type: "x", data: 1 });
  assert.deepEqual(got, []);
});

test("connect after disconnect works (logout then login) and counts as a first open", () => {
  const m = newManager();
  let reconnected = 0;
  m.subscribe("ws:reconnected", () => reconnected++);

  m.connect();
  lastSocket().serverOpens();
  m.disconnect();

  m.connect();
  assert.equal(FakeSocket.instances.length, 2);
  assert.equal(m.getStatus(), STATUS.CONNECTING);
  lastSocket().serverOpens();
  assert.equal(reconnected, 0); // a new session, not a reconnect
  assert.equal(m.getStatus(), STATUS.OPEN);
});

test("status listeners can unsubscribe", () => {
  const m = newManager();
  const seen = [];
  const off = m.onStatusChange((s) => seen.push(s));
  m.connect();
  off();
  lastSocket().serverOpens();
  assert.deepEqual(seen, [STATUS.CONNECTING]);
});

test("defaultSocketUrl follows the page's protocol", () => {
  assert.equal(defaultSocketUrl({ protocol: "http:", host: "localhost:5173" }), "ws://localhost:5173/ws");
  assert.equal(defaultSocketUrl({ protocol: "https:", host: "example.com" }), "wss://example.com/ws");
});