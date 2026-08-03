const API_BASE = import.meta.env.VITE_API_BASE || "http://localhost:8080";

async function getJSON(path) {
  const res = await fetch(`${API_BASE}${path}`);
  if (!res.ok) throw new Error(`${path}: ${res.status}`);
  return res.json();
}

export function fetchSLO(projectId) {
  return getJSON(`/slo/${encodeURIComponent(projectId)}`);
}

export function fetchIncidents(projectId) {
  return getJSON(`/incidents/${encodeURIComponent(projectId)}`);
}

export function fetchMetrics(projectId) {
  return getJSON(`/metrics/${encodeURIComponent(projectId)}`);
}

export function badgeURL(projectId) {
  return `${API_BASE}/badge/${encodeURIComponent(projectId)}`;
}
