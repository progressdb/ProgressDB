import { useEffect, useState } from 'react';
import { useProgressClient } from './client';

/**
 * Hook: basic health check.
 */
export function useHealthz() {
  const client = useProgressClient();
  const [data, setData] = useState<{ status: string } | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<any>(null);

  const fetch = async () => {
    setLoading(true);
    setError(null);
    try {
      const result = await client.healthz();
      setData(result);
    } catch (err) {
      setError(err);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetch();
  }, []);

  return { data, loading, error, refresh: fetch };
}

/**
 * Hook: readiness check with version info.
 */
export function useReadyz() {
  const client = useProgressClient();
  const [data, setData] = useState<{ status: string; version?: string } | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<any>(null);

  const fetch = async () => {
    setLoading(true);
    setError(null);
    try {
      const result = await client.readyz();
      setData(result);
    } catch (err) {
      setError(err);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetch();
  }, []);

  return { data, loading, error, refresh: fetch };
}