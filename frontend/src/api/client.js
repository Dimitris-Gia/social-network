// Owner: B — fetch wrapper: credentials "include", JSON, errors
//
// Session expiry: the backend answers 401 to every protected route once the
// session is gone (expired, or logged out in another tab). request() reports
// that to ONE handler, set by AuthContext, which forgets the user; then
// ProtectedRoute sends them to /login. Without this, the page would stay open
// and every action would just fail with "not logged in".

// Paths where a 401 does NOT mean "your session ended":
// /api/login answers 401 for a wrong email or password.
const NOT_A_SESSION_401 = new Set(["/api/login"]);

let onUnauthorized = () => {};

// AuthContext calls this once. Pass a no-op to remove it.
export function setUnauthorizedHandler(fn) {
  onUnauthorized = fn;
}

export async function request(path, { method = "GET", body } = {}) {
  const isForm = body instanceof FormData;
  const res = await fetch(path, {
    method,
    credentials: "include",
    headers: body && !isForm ? { "Content-Type": "application/json" } : undefined,
    body: body ? (isForm ? body : JSON.stringify(body)) : undefined,
  });
  const data = res.status === 204 ? null : await res.json().catch(() => null);
  if (!res.ok) {
    if (res.status === 401 && !NOT_A_SESSION_401.has(path)) {
      try {
        onUnauthorized();
      } catch (err) {
        console.error("request: unauthorized handler failed", err);
      }
    }
    const err = new Error(data?.error || `Request failed (${res.status})`);
    err.status = res.status; // lets callers tell 401/403/404 apart
    throw err;
  }
  return data;
}
