import { useCallback, useEffect, useState } from 'react';

export type ApiResourceState<T> = {
  data?: T;
  error?: unknown;
  loading: boolean;
  refetch: () => void;
};

export type ApiResourceOptions = {
  intervalMs?: number;
};

export function useApiResource<T>(
  load: (signal: AbortSignal) => Promise<T>,
  options: ApiResourceOptions = {},
): ApiResourceState<T> {
  const [data, setData] = useState<T | undefined>();
  const [error, setError] = useState<unknown>();
  const [loading, setLoading] = useState(true);
  const [reloadCount, setReloadCount] = useState(0);
  const refetch = useCallback(() => {
    setLoading(true);
    setError(undefined);
    setReloadCount((count) => count + 1);
  }, []);

  useEffect(() => {
    const controller = new AbortController();

    load(controller.signal)
      .then((result) => {
        setData(result);
      })
      .catch((err: unknown) => {
        if (!controller.signal.aborted) {
          setError(err);
        }
      })
      .finally(() => {
        if (!controller.signal.aborted) {
          setLoading(false);
        }
      });

    return () => controller.abort();
  }, [load, reloadCount]);

  useEffect(() => {
    if (!options.intervalMs) {
      return undefined;
    }

    const timer = window.setInterval(refetch, options.intervalMs);

    return () => window.clearInterval(timer);
  }, [options.intervalMs, refetch]);

  return {
    data,
    error,
    loading,
    refetch,
  };
}
