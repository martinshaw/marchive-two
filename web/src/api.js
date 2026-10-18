async function read(response) {
  const type = response.headers.get("Content-Type") || "";
  const data = type.includes("json") ? await response.json().catch(() => ({})) : {};
  if (!response.ok) throw new Error(data.error || "Could not save that change.");
  return data;
}

export function getState() {
  return fetch("/api/state", { cache: "no-store", headers: { Accept: "application/json" } }).then(read);
}

export function send(method, path, body) {
  return fetch(path, {
    method,
    headers: { Accept: "application/json", "Content-Type": "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body),
  }).then(read);
}
