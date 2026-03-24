import { useEffect, type RefObject } from 'react';

export function useAutoResize(ref: RefObject<HTMLTextAreaElement | null>) {
  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    const resize = () => {
      el.style.height = 'auto';
      el.style.height = Math.min(el.scrollHeight, 200) + 'px';
    };
    el.addEventListener('input', resize);
    resize();
    return () => el.removeEventListener('input', resize);
  }, [ref]);
}
