// Owner: B
// The WebSocket connection logic, with no React in it, so it can be tested
// on its own (npm test). WebSocketContext.jsx is the thin React wrapper.
//
// What it does:
//   - one connection to /ws; opening it twice does nothing
//   - subscribe(type, handler): handlers receive the frame's `data`
//   - send(type, data): {"type": ..., "data": ...}, the same envelope as the server
//   - if the connection drops, it reconnects with a growing delay (1s, 2s, 4s ...
//     up to 30s) until disconnect() is called
//
// Synthetic event: "ws:reconnected" is delivered to subscribers every time the
// connection comes back after a drop. Messages and notifications that arrived
// while we were offline are NOT replayed, so subscribers should re-fetch then.

export const STATUS = {
  IDLE: "idle", // never connected
  CONNECTING: "connecting", // first attempt
  OPEN: "open",
  RECONNECTING: "reconnecting", // dropped, waiting or trying again
  CLOSED: "closed", // disconnect() was called
};

const WS_OPEN = 1; // WebSocket.OPEN

// ws://host/ws, or wss:// when the page itself uses https.
export function defaultSocketUrl(loc = globalThis.location) {
  return `${loc.protocol === "https:" ? "wss:" : "ws:"}//${loc.host}/ws`;
}

export function createSocketManager({
  url,
  WebSocketImpl = globalThis.WebSocket,
  baseDelay = 1000,
  maxDelay = 30000,
  // injectable so tests do not need real time
  setTimer = (fn, ms) => setTimeout(fn, ms),
  clearTimer = (id) => clearTimeout(id),
  random = Math.random,
} = {}) {
  const handlers = new Map(); // type -> Set of functions
  const statusListeners = new Set();

  let socket = null;
  let status = STATUS.IDLE;
  let wantOpen = false; // true between connect() and disconnect()
  let attempts = 0; // failed attempts in a row
  let hasOpened = false; // has this session been open before? (for "ws:reconnected")
  let timer = null;

  function setStatus(next) {
    if (next === status) return;
    status = next;
    for (const listener of [...statusListeners]) {
      try {
        listener(status);
      } catch (err) {
        console.error("websocket: status listener failed", err);
      }
    }
  }

  function emit(type, data) {
    for (const handler of [...(handlers.get(type) ?? [])]) {
      try {
        handler(data);
      } catch (err) {
        // one broken handler must not stop the others
        console.error(`websocket: handler for "${type}" failed`, err);
      }
    }
  }

  function open() {
    timer = null;
    setStatus(attempts > 0 || hasOpened ? STATUS.RECONNECTING : STATUS.CONNECTING);

    const sock = new WebSocketImpl(url ?? defaultSocketUrl());
    socket = sock;

    // Every callback first checks that it belongs to the CURRENT socket, so a
    // late event from an old, replaced socket cannot change anything.
    sock.onopen = () => {
      if (socket !== sock) return;
      attempts = 0;
      const wasOpenBefore = hasOpened;
      hasOpened = true;
      setStatus(STATUS.OPEN);
      if (wasOpenBefore) emit("ws:reconnected", null);
    };

    sock.onmessage = (event) => {
      if (socket !== sock) return;
      let frame;
      try {
        frame = JSON.parse(event.data);
      } catch {
        console.warn("websocket: ignored a frame that is not JSON");
        return;
      }
      if (!frame || typeof frame !== "object" || typeof frame.type !== "string") {
        console.warn("websocket: ignored a frame without a type");
        return;
      }
      emit(frame.type, frame.data);
    };

    sock.onclose = () => {
      if (socket !== sock) return;
      socket = null;
      if (!wantOpen) return;
      attempts += 1;
      setStatus(STATUS.RECONNECTING);
      // 1s, 2s, 4s ... capped, with some randomness so many tabs do not all
      // come back at the same moment.
      const delay = Math.min(maxDelay, baseDelay * 2 ** (attempts - 1)) * (0.5 + random() / 2);
      timer = setTimer(open, delay);
    };
    // onerror needs no handling: a failed connection always ends in onclose.
  }

  return {
    // Open the connection (does nothing if already wanted open).
    connect() {
      if (wantOpen) return;
      wantOpen = true;
      open();
    },

    // Close for good (logout). No reconnect. connect() can be called again later.
    disconnect() {
      wantOpen = false;
      attempts = 0;
      hasOpened = false;
      if (timer !== null) {
        clearTimer(timer);
        timer = null;
      }
      if (socket) {
        const old = socket;
        socket = null; // so old.onclose is ignored
        old.close();
      }
      setStatus(STATUS.CLOSED);
    },

    // Call handler(data) for every frame of this type. Returns an unsubscribe function.
    subscribe(type, handler) {
      if (!handlers.has(type)) handlers.set(type, new Set());
      handlers.get(type).add(handler);
      return () => handlers.get(type)?.delete(handler);
    },

    // Send a frame. Returns false (and sends nothing) if the connection is not open.
    send(type, data) {
      if (!socket || socket.readyState !== WS_OPEN) return false;
      try {
        socket.send(JSON.stringify({ type, data }));
        return true;
      } catch {
        return false;
      }
    },

    getStatus: () => status,

    // Call listener(status) on every status change. Returns an unsubscribe function.
    onStatusChange(listener) {
      statusListeners.add(listener);
      return () => statusListeners.delete(listener);
    },
  };
}