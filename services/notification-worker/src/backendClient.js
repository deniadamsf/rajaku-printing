// Thin HTTP client untuk backend /internal/notifications/*. Sengaja pakai
// fetch bawaan Node 20+ (no axios) — biar dependency minimal.
import { config } from './config.js';

const defaultHeaders = {
  'Content-Type': 'application/json',
  'X-Internal-Secret': config.internalSecret,
};

async function postJSON(path, body) {
  const url = `${config.backendURL}${path}`;
  const res = await fetch(url, {
    method: 'POST',
    headers: defaultHeaders,
    body: JSON.stringify(body || {}),
    // AbortController — worker jangan hang selamanya kalau backend lambat.
    signal: AbortSignal.timeout(15_000),
  });
  const text = await res.text();
  let json;
  try {
    json = text ? JSON.parse(text) : {};
  } catch {
    throw new Error(`backend ${path} returned non-JSON (${res.status}): ${text.slice(0, 200)}`);
  }
  if (!res.ok) {
    const msg = json?.error?.message || `HTTP ${res.status}`;
    const err = new Error(`backend ${path} failed: ${msg}`);
    err.status = res.status;
    err.body = json;
    throw err;
  }
  return json;
}

export async function claimJobs(limit) {
  const out = await postJSON('/api/v1/internal/notifications/claim', { limit });
  return out?.data?.jobs || [];
}

export async function markSent(jobId) {
  return postJSON(`/api/v1/internal/notifications/${jobId}/sent`);
}

export async function markFailed(jobId, errorMessage) {
  return postJSON(`/api/v1/internal/notifications/${jobId}/failed`, {
    error: errorMessage,
  });
}
