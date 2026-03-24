import { useEffect, useRef, useCallback } from 'react';
import { useSettings } from '../context/SettingsContext';

let audioCtx: AudioContext | null = null;

function playNotificationSound() {
  try {
    if (!audioCtx) audioCtx = new AudioContext();
    const osc = audioCtx.createOscillator();
    const gain = audioCtx.createGain();
    osc.connect(gain);
    gain.connect(audioCtx.destination);
    osc.frequency.value = 880;
    osc.type = 'sine';
    gain.gain.setValueAtTime(0.15, audioCtx.currentTime);
    gain.gain.exponentialRampToValueAtTime(0.001, audioCtx.currentTime + 0.3);
    osc.start(audioCtx.currentTime);
    osc.stop(audioCtx.currentTime + 0.3);
  } catch { /* audio not available */ }
}

function getBaseTitle(): string {
  return document.title.replace(/^\(\d+\)\s*/, '');
}

export function useNotifications() {
  const { settings } = useSettings();
  const unreadCount = useRef(0);
  const tabFocused = useRef(!document.hidden);

  useEffect(() => {
    const onVisibility = () => {
      tabFocused.current = !document.hidden;
      if (tabFocused.current) {
        unreadCount.current = 0;
        document.title = getBaseTitle();
      }
    };
    document.addEventListener('visibilitychange', onVisibility);
    return () => document.removeEventListener('visibilitychange', onVisibility);
  }, []);

  const notifyResponse = useCallback((content: string) => {
    if (tabFocused.current) return;

    unreadCount.current += 1;
    document.title = `(${unreadCount.current}) ${getBaseTitle()}`;

    if (settings.notificationsEnabled && 'Notification' in window && Notification.permission === 'granted') {
      const snippet = content.length > 100 ? content.slice(0, 100) + '...' : content;
      const n = new Notification('Botka Chat', {
        body: snippet,
        tag: 'botka-response',
      });
      n.onclick = () => {
        window.focus();
        n.close();
      };
    }

    if (settings.notificationSound) {
      playNotificationSound();
    }
  }, [settings.notificationsEnabled, settings.notificationSound]);

  return { notifyResponse };
}
