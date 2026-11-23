// Owner: B — fetch wrapper: credentials "include", JSON, errors
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
    throw new Error(data?.error || `Request failed (${res.status})`);
  }
  return data;
}
