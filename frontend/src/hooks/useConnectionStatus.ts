import { useState, useEffect, useRef, useCallback } from 'react';

export type ConnectionStatus = 'connected' | 'offline' | 'unavailable';

export function useConnectionStatus() {
  const [isOffline, setIsOffline] = useState(!navigator.onLine);
  const [isServerUnavailable, setIsServerUnavailable] = useState(false);
  const healthPollRef = useRef<ReturnType<typeof setInterval>>(undefined);

  const checkHealth = useCallback(async (): Promise<boolean> => {
    try {
      const res = await fetch('/api/v1/health');
      return res.ok;
    } catch {
      return false;
    }
  }, []);

  const startHealthPolling = useCallback(() => {
    if (healthPollRef.current) return;
    setIsServerUnavailable(true);
    healthPollRef.current = setInterval(async () => {
      const ok = await checkHealth();
      if (ok) {
        clearInterval(healthPollRef.current);
        healthPollRef.current = undefined;
        setIsServerUnavailable(false);
      }
    }, 5000);
  }, [checkHealth]);

  const stopHealthPolling = useCallback(() => {
    if (healthPollRef.current) {
      clearInterval(healthPollRef.current);
      healthPollRef.current = undefined;
    }
    setIsServerUnavailable(false);
  }, []);

  useEffect(() => {
    const handleOffline = () => setIsOffline(true);
    const handleOnline = () => {
      setIsOffline(false);
      checkHealth().then((ok) => {
        if (!ok) startHealthPolling();
        else stopHealthPolling();
      });
    };

    window.addEventListener('offline', handleOffline);
    window.addEventListener('online', handleOnline);

    return () => {
      window.removeEventListener('offline', handleOffline);
      window.removeEventListener('online', handleOnline);
      if (healthPollRef.current) {
        clearInterval(healthPollRef.current);
      }
    };
  }, [checkHealth, startHealthPolling, stopHealthPolling]);

  const status: ConnectionStatus = isOffline
    ? 'offline'
    : isServerUnavailable
      ? 'unavailable'
      : 'connected';

  return { status, startHealthPolling, stopHealthPolling };
}
