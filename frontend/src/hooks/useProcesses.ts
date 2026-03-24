import { useState, useEffect, useCallback } from 'react';
import { api } from '../api/client';

export type ProcessInfo = {
  thread_id: number;
  thread_title: string;
  started_at: string;
  duration_sec: number;
};

export function useProcesses() {
  const [processes, setProcesses] = useState<ProcessInfo[]>([]);

  const poll = useCallback(async () => {
    try {
      const list = await api.listProcesses();
      setProcesses(list);
    } catch { /* ignore */ }
  }, []);

  useEffect(() => {
    poll();
    const id = setInterval(poll, 3000);
    return () => clearInterval(id);
  }, [poll]);

  const killProcess = useCallback(async (threadId: number) => {
    try {
      await api.killProcess(threadId);
      poll();
    } catch { /* ignore */ }
  }, [poll]);

  return { processes, poll, killProcess };
}
