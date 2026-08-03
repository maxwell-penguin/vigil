import { useEffect, useRef, useState } from "react";
import { fetchIncidents, fetchMetrics, fetchSLO } from "../api";

const POLL_MS = 30_000;

// ponytail: one hook fetching all three endpoints together, since the
// dashboard always needs them in lockstep — no separate hook per endpoint.
export function useDashboardData(projectId) {
  const [slo, setSLO] = useState(null);
  const [incidents, setIncidents] = useState([]);
  const [metrics, setMetrics] = useState([]);
  const [error, setError] = useState(null);
  const [loading, setLoading] = useState(true);
  const projectIdRef = useRef(projectId);
  projectIdRef.current = projectId;

  useEffect(() => {
    let cancelled = false;

    async function load() {
      try {
        const [sloRes, incidentsRes, metricsRes] = await Promise.all([
          fetchSLO(projectId),
          fetchIncidents(projectId),
          fetchMetrics(projectId),
        ]);
        if (cancelled || projectIdRef.current !== projectId) return;
        setSLO(sloRes);
        setIncidents(incidentsRes || []);
        setMetrics(metricsRes || []);
        setError(null);
      } catch (e) {
        if (!cancelled) setError(e.message);
      } finally {
        if (!cancelled) setLoading(false);
      }
    }

    setLoading(true);
    load();
    const id = setInterval(load, POLL_MS);
    return () => {
      cancelled = true;
      clearInterval(id);
    };
  }, [projectId]);

  return { slo, incidents, metrics, error, loading };
}
