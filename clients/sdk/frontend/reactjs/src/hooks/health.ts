import { useEffect, useState } from 'react';
import { useProgressClient } from './client';
import type { HealthzResponseType, ReadyzResponseType, ApiErrorResponseType } from '@progressdb/js';

/**
 * Hook: basic health check.
 */
export function useHealthz() {
  const client = useProgressClient();
  const [data, setData] = useState<HealthzResponseType | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<ApiErrorResponseType | null>(null);

  const fetch = async () => {
    setLoading(true);
    setError(null);
    try {
      const result = await client.healthz();
      setData(result);
    } catch (err) {
      setError(err as ApiErrorResponseType);
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
  const [data, setData] = useState<ReadyzResponseType | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<ApiErrorResponseType | null>(null);

  const fetch = async () => {
    setLoading(true);
    setError(null);
    try {
      const result = await client.readyz();
      setData(result);
    } catch (err) {
      setError(err as ApiErrorResponseType);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetch();
  }, []);

  return { data, loading, error, refresh: fetch };
}