import { useCallback, useRef } from 'react';

const MAX_HISTORY = 100;

export function useInputHistory() {
  const historyRef = useRef<string[]>([]);
  const indexRef = useRef(-1); // -1 means "not navigating"
  const draftRef = useRef('');

  const push = useCallback((text: string) => {
    const trimmed = text.trim();
    if (!trimmed) return;
    // Avoid consecutive duplicates
    const history = historyRef.current;
    if (history.length > 0 && history[history.length - 1] === trimmed) {
      indexRef.current = -1;
      return;
    }
    history.push(trimmed);
    if (history.length > MAX_HISTORY) {
      history.shift();
    }
    indexRef.current = -1;
  }, []);

  const navigateUp = useCallback((currentValue: string): string | null => {
    const history = historyRef.current;
    if (history.length === 0) return null;

    if (indexRef.current === -1) {
      // Starting navigation — save draft
      draftRef.current = currentValue;
      indexRef.current = history.length - 1;
      return history[indexRef.current];
    }

    if (indexRef.current > 0) {
      indexRef.current--;
      return history[indexRef.current];
    }

    // Already at oldest — do nothing
    return null;
  }, []);

  const navigateDown = useCallback((): string | null => {
    if (indexRef.current === -1) return null; // Not navigating

    const history = historyRef.current;

    if (indexRef.current < history.length - 1) {
      indexRef.current++;
      return history[indexRef.current];
    }

    // Past the end — restore draft
    indexRef.current = -1;
    return draftRef.current;
  }, []);

  const reset = useCallback(() => {
    indexRef.current = -1;
    draftRef.current = '';
  }, []);

  return { push, navigateUp, navigateDown, reset };
}
