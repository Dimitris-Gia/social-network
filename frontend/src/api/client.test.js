// Owner: B
// Run with: npm test   (Node's built-in test runner, no extra packages)
import { test, beforeEach, afterEach } from "node:test";
import assert from "node:assert/strict";
import { request, setUnauthorizedHandler } from "./client.js";

// fetch is replaced by a fake that answers with the given status and body.
const realFetch = globalThis.fetch;
function answer(status, body) {
  globalThis.fetch = async () => ({
    ok: status >= 200 && status < 300,
    status,
    json: async () => {
      if (body === undefined) throw new SyntaxError("no body");
      return body;
    },
  });
}

let unauthorizedCalls;
beforeEach(() => {
  unauthorizedCalls = 0;
  setUnauthorizedHandler(() => unauthorizedCalls++);
});
afterEach(() => {
  globalThis.fetch = realFetch;
  setUnauthorizedHandler(() => {});
});

test("a 401 from a protected route calls the handler and throws with the status", async () => {
  answer(401, { error: "not logged in" });
  await assert.rejects(request("/api/posts"), (err) => {
    assert.equal(err.status, 401);
    assert.equal(err.message, "not logged in");
    return true;
  });
  assert.equal(unauthorizedCalls, 1);
});

test("a 401 from /api/login (wrong password) does NOT log out", async () => {
  answer(401, { error: "invalid email or password" });
  await assert.rejects(request("/api/login", { method: "POST", body: {} }), { status: 401 });
  assert.equal(unauthorizedCalls, 0);
});

test("other errors keep their status and do not call the handler", async () => {
  for (const status of [400, 403, 404, 500]) {
    answer(status, { error: "nope" });
    await assert.rejects(request("/api/x"), { status, message: "nope" });
  }
  answer(502, undefined); // no JSON body (e.g. the proxy answered)
  await assert.rejects(request("/api/x"), { status: 502, message: "Request failed (502)" });
  assert.equal(unauthorizedCalls, 0);
});

test("a handler that throws does not hide the request's own error", async (t) => {
  t.mock.method(console, "error", () => {});
  setUnauthorizedHandler(() => {
    throw new Error("boom");
  });
  answer(401, { error: "not logged in" });
  await assert.rejects(request("/api/me"), { status: 401, message: "not logged in" });
});

test("success still returns the data, and 204 returns null", async () => {
  answer(200, { id: 1 });
  assert.deepEqual(await request("/api/me"), { id: 1 });
  answer(204);
  assert.equal(await request("/api/logout", { method: "POST" }), null);
  assert.equal(unauthorizedCalls, 0);
});
