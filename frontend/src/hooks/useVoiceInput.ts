import { useState, useRef, useCallback, useEffect } from 'react';
import { api } from '../api/client';

export type VoiceState = 'idle' | 'recording' | 'transcribing' | 'unsupported' | 'denied';
export type VoiceMode = 'native' | 'whisper' | null;

const RECORDING_TIMEOUT_MS = 2 * 60 * 1000;

interface SpeechRecognitionEvent {
  results: SpeechRecognitionResultList;
  resultIndex: number;
}

interface SpeechRecognitionErrorEvent {
  error: string;
}

interface SpeechRecognitionInstance extends EventTarget {
  continuous: boolean;
  interimResults: boolean;
  lang: string;
  start(): void;
  stop(): void;
  abort(): void;
  onresult: ((event: SpeechRecognitionEvent) => void) | null;
  onerror: ((event: SpeechRecognitionErrorEvent) => void) | null;
  onend: (() => void) | null;
}

declare global {
  interface Window {
    SpeechRecognition?: new () => SpeechRecognitionInstance;
    webkitSpeechRecognition?: new () => SpeechRecognitionInstance;
  }
}

const getSpeechRecognition = (): (new () => SpeechRecognitionInstance) | null => {
  return window.SpeechRecognition || window.webkitSpeechRecognition || null;
};

const hasMediaRecorder = (): boolean => {
  return typeof MediaRecorder !== 'undefined';
};

function detectMode(): VoiceMode {
  if (getSpeechRecognition()) return 'native';
  if (hasMediaRecorder()) return 'whisper';
  return null;
}

export function useVoiceInput(onTranscript: (text: string) => void) {
  const [mode] = useState<VoiceMode>(() => detectMode());
  const modeRef = useRef<VoiceMode>(mode);
  const [state, setState] = useState<VoiceState>(() =>
    mode ? 'idle' : 'unsupported'
  );
  const [whisperAvailable, setWhisperAvailable] = useState<boolean | null>(null);

  const recognitionRef = useRef<SpeechRecognitionInstance | null>(null);
  const finalTranscriptRef = useRef('');
  const recorderRef = useRef<MediaRecorder | null>(null);
  const chunksRef = useRef<Blob[]>([]);
  const timeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    if (modeRef.current !== 'whisper') return;
    api.getTranscribeStatus().then(
      (status) => {
        setWhisperAvailable(status.enabled);
        if (!status.enabled) {
          setState('unsupported');
          modeRef.current = null;
        }
      },
      () => {
        setWhisperAvailable(false);
        setState('unsupported');
        modeRef.current = null;
      },
    );
  }, []);

  const startNative = useCallback(() => {
    const SpeechRecognition = getSpeechRecognition();
    if (!SpeechRecognition) {
      setState('unsupported');
      return;
    }

    if (recognitionRef.current) {
      recognitionRef.current.abort();
      recognitionRef.current = null;
    }

    const recognition = new SpeechRecognition();
    recognition.continuous = true;
    recognition.interimResults = true;
    recognition.lang = navigator.language || 'en-US';
    finalTranscriptRef.current = '';

    recognition.onresult = (event: SpeechRecognitionEvent) => {
      let finalText = '';
      for (let i = 0; i < event.results.length; i++) {
        const result = event.results[i];
        if (result?.isFinal) {
          finalText += result[0]!.transcript;
        }
      }
      if (finalText && finalText !== finalTranscriptRef.current) {
        const newText = finalText.slice(finalTranscriptRef.current.length);
        finalTranscriptRef.current = finalText;
        if (newText.trim()) {
          onTranscript(newText);
        }
      }
    };

    recognition.onerror = (event: SpeechRecognitionErrorEvent) => {
      if (event.error === 'not-allowed') {
        setState('denied');
      } else if (event.error !== 'aborted' && event.error !== 'no-speech') {
        setState('idle');
      }
      recognitionRef.current = null;
    };

    recognition.onend = () => {
      setState((s) => (s === 'recording' ? 'idle' : s));
      recognitionRef.current = null;
    };

    recognitionRef.current = recognition;
    try {
      recognition.start();
      setState('recording');
    } catch {
      setState('idle');
      recognitionRef.current = null;
    }
  }, [onTranscript]);

  const stopNative = useCallback(() => {
    if (recognitionRef.current) {
      recognitionRef.current.stop();
      recognitionRef.current = null;
    }
    setState((s) => (s === 'recording' ? 'idle' : s));
  }, []);

  const startWhisper = useCallback(async () => {
    try {
      const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
      const mimeType = MediaRecorder.isTypeSupported('audio/webm;codecs=opus')
        ? 'audio/webm;codecs=opus'
        : MediaRecorder.isTypeSupported('audio/ogg;codecs=opus')
          ? 'audio/ogg;codecs=opus'
          : 'audio/webm';

      const recorder = new MediaRecorder(stream, { mimeType });
      chunksRef.current = [];

      recorder.ondataavailable = (e) => {
        if (e.data.size > 0) chunksRef.current.push(e.data);
      };

      recorder.onstop = async () => {
        stream.getTracks().forEach((t) => t.stop());

        if (timeoutRef.current) {
          clearTimeout(timeoutRef.current);
          timeoutRef.current = null;
        }

        if (chunksRef.current.length === 0) {
          setState('idle');
          return;
        }

        const blob = new Blob(chunksRef.current, { type: mimeType });
        chunksRef.current = [];

        setState('transcribing');
        try {
          const lang = navigator.language?.split('-')[0] || undefined;
          const text = await api.transcribe(blob, lang);
          if (text.trim()) {
            onTranscript(text);
          }
        } catch (err) {
          console.error('Transcription failed:', err);
        }
        setState('idle');
      };

      recorderRef.current = recorder;
      recorder.start();
      setState('recording');

      timeoutRef.current = setTimeout(() => {
        if (recorderRef.current?.state === 'recording') {
          recorderRef.current.stop();
          recorderRef.current = null;
        }
      }, RECORDING_TIMEOUT_MS);
    } catch (err) {
      console.error('Mic access failed:', err);
      setState('denied');
    }
  }, [onTranscript]);

  const stopWhisper = useCallback(() => {
    if (recorderRef.current?.state === 'recording') {
      recorderRef.current.stop();
      recorderRef.current = null;
    }
  }, []);

  const cancelWhisper = useCallback(() => {
    if (recorderRef.current) {
      chunksRef.current = [];
      if (recorderRef.current.state === 'recording') {
        recorderRef.current.stream.getTracks().forEach((t) => t.stop());
        recorderRef.current.stop();
      }
      recorderRef.current = null;
    }
    if (timeoutRef.current) {
      clearTimeout(timeoutRef.current);
      timeoutRef.current = null;
    }
    setState('idle');
  }, []);

  const toggle = useCallback(() => {
    if (state === 'recording') {
      if (modeRef.current === 'native') stopNative();
      else stopWhisper();
    } else if (state === 'idle' || state === 'denied') {
      if (modeRef.current === 'native') startNative();
      else if (modeRef.current === 'whisper') startWhisper();
    }
  }, [state, startNative, stopNative, startWhisper, stopWhisper]);

  const cancel = useCallback(() => {
    if (modeRef.current === 'native') stopNative();
    else cancelWhisper();
  }, [stopNative, cancelWhisper]);

  useEffect(() => {
    return () => {
      if (recognitionRef.current) {
        recognitionRef.current.abort();
        recognitionRef.current = null;
      }
      if (recorderRef.current?.state === 'recording') {
        recorderRef.current.stream.getTracks().forEach((t) => t.stop());
        recorderRef.current.stop();
        recorderRef.current = null;
      }
      if (timeoutRef.current) {
        clearTimeout(timeoutRef.current);
        timeoutRef.current = null;
      }
    };
  }, []);

  const isSupported = mode !== null && (mode === 'native' || whisperAvailable !== false);

  return {
    state,
    toggle,
    cancel,
    isSupported,
    mode,
  };
}
