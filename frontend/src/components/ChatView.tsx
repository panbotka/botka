import { useState, useEffect, useRef, useCallback, type DragEvent } from 'react';
import type { Message, Thread, ThreadDetail, Attachment, ForkPoint } from '../types';
import { api, interruptThread, streamChat, streamRegenerate, streamEdit, streamBranch, streamSubscribe, fetchSessionHealth, toggleMessageHidden } from '../api/client';
import type { SessionHealthData } from '../api/client';
import type { StreamChunk } from '../api/client';
import { useSSEManager, useSSESession } from '../context/SSEContext';
import type { ActiveToolCall } from '../context/SSEContext';
import MessageBubble from './MessageBubble';
import ChatInput, { isAllowedFile, getFileExtension, MAX_FILE_SIZE } from './ChatInput';
import type { ChatInputHandle } from './ChatInput';
import ToolCallPanel from './ToolCallPanel';
import AskUserPanel from './AskUserPanel';
import StreamErrorBlock from './StreamErrorBlock';
import Lightbox from './Lightbox';
import { COMMANDS } from './SlashCommandMenu';
import { downloadExport } from '../utils/exportThread';
import type { ExportFormat } from '../utils/exportThread';
import { useNotifications } from '../hooks/useNotifications';
import { useConnectionStatus } from '../hooks/useConnectionStatus';
import { useChatSync } from '../hooks/useChatSync';
import { useSettings } from '../context/SettingsContext';
import { getThreadBackground } from '../utils/threadColors';

interface Props {
  threadId: number | null;
  thread?: Thread;
  onTitleUpdate?: (threadId: number, title: string) => void;
  onNewThread?: () => void;
  onOpenSearch?: () => void;
  pendingStarterMessage?: string | null;
  onStarterMessageConsumed?: () => void;
  onStreamingChange?: (threadId: number | null) => void;
}

export default function ChatView({ threadId, thread, onTitleUpdate, onNewThread, onOpenSearch, pendingStarterMessage, onStarterMessageConsumed, onStreamingChange }: Props) {
  // --- State ---
  const [messages, setMessages] = useState<Message[]>([]);
  const [loading, setLoading] = useState(false);
  const [commandFeedback, setCommandFeedback] = useState<{ type: 'info' | 'error'; text: string } | null>(null);
  const [clearConfirm, setClearConfirm] = useState(false);
  const [lightbox, setLightbox] = useState<{ attachment: Attachment; allImages: Attachment[] } | null>(null);
  const [dragOver, setDragOver] = useState(false);
  const [queuedIds, setQueuedIds] = useState<Set<number>>(new Set());
  const [streamError, setStreamError] = useState<string | null>(null);
  const [streamErrorRaw, setStreamErrorRaw] = useState<string | null>(null);
  const [memorySuggestions, setMemorySuggestions] = useState<string[]>([]);
  const [forkPoints, setForkPoints] = useState<Record<string, ForkPoint>>({});
  const [usageInfo, setUsageInfo] = useState<{ cost_usd?: number; input_tokens?: number; output_tokens?: number } | null>(null);
  const [sessionHealth, setSessionHealth] = useState<SessionHealthData | null>(null);
  const [branchFromId, setBranchFromId] = useState<number | null>(null);
  const [planMode, setPlanMode] = useState(false);

  // --- Refs ---
  const dragCounterRef = useRef(0);
  const chatInputRef = useRef<ChatInputHandle>(null);
  const bottomRef = useRef<HTMLDivElement>(null);
  const scrollContainerRef = useRef<HTMLDivElement>(null);
  const userSentRef = useRef(false);
  const isAtBottomRef = useRef(true);
  const feedbackTimer = useRef<ReturnType<typeof setTimeout>>(undefined);
  const messageQueueRef = useRef<{ id: number; content: string; files?: File[] }[]>([]);
  const currentThreadIdRef = useRef(threadId);
  currentThreadIdRef.current = threadId;

  // --- Settings ---
  const { resolvedTheme } = useSettings();

  // --- SSE Manager ---
  const sseManager = useSSEManager();
  const sseSession = useSSESession(threadId);

  // Derived streaming values from the persistent SSE session
  const streamingContent = sseSession?.content ?? '';
  const streamingThinking = sseSession?.thinking ?? '';
  const thinkingDurationMs = sseSession?.thinkingDurationMs ?? null;
  const activeToolCalls: ActiveToolCall[] = sseSession?.toolCalls ?? [];
  const reconnecting = sseSession?.reconnecting ?? null;
  const retryInfo = sseSession?.retryInfo ?? null;
  const isStreaming = sseSession?.isStreaming ?? false;
  const isStreamingThisThread = isStreaming;

  // --- Hooks ---
  const { notifyResponse } = useNotifications();
  const { status: connectionStatus, startHealthPolling, stopHealthPolling } = useConnectionStatus();

  // --- Cross-tab message sync ---
  const handleSyncNewMessage = useCallback((message: Message) => {
    setMessages(prev => {
      if (prev.some(m => m.id === message.id)) return prev;
      return [...prev, message];
    });
  }, []);

  const handleSyncThreadUpdated = useCallback(() => {
    if (!threadId) return;
    api.getThread(threadId).then((data: ThreadDetail) => {
      if (currentThreadIdRef.current === threadId) {
        setMessages(data.messages);
        setForkPoints(data.fork_points || {});
      }
    }).catch(() => {});
  }, [threadId]);

  const { broadcastNewMessage, broadcastThreadUpdated } = useChatSync({
    threadId,
    onNewMessage: handleSyncNewMessage,
    onThreadUpdated: handleSyncThreadUpdated,
  });

  // Stable callback refs (used in .then() callbacks to avoid stale closures)
  const onStreamingChangeRef = useRef(onStreamingChange);
  onStreamingChangeRef.current = onStreamingChange;
  const onTitleUpdateRef = useRef(onTitleUpdate);
  onTitleUpdateRef.current = onTitleUpdate;

  // Track completion context for the current stream
  const completionContextRef = useRef<{ isBranching: boolean; isEdit: boolean }>({ isBranching: false, isEdit: false });

  // Refs to break circular dependency between startStream and handleCompletion
  const handleStreamCompletionRef = useRef<(tid: number) => void>(() => {});
  const startStreamRef = useRef<(content: string, files?: File[], branchParentId?: number | null) => void>(() => {});

  // --- Utility callbacks ---

  const showFeedback = useCallback((type: 'info' | 'error', text: string) => {
    setCommandFeedback({ type, text });
    clearTimeout(feedbackTimer.current);
    feedbackTimer.current = setTimeout(() => setCommandFeedback(null), 4000);
  }, []);

  const playCompletionSound = useCallback(() => {
    if (document.hidden) {
      const audio = new Audio('/sounds/work-work.ogg');
      audio.volume = 0.5;
      audio.play().catch(() => {});
    }
  }, []);

  useEffect(() => {
    return () => clearTimeout(feedbackTimer.current);
  }, []);

  // Global Shift+Tab handler for plan mode toggle
  useEffect(() => {
    if (!threadId) return;
    const handler = (e: KeyboardEvent) => {
      if (e.key === 'Tab' && e.shiftKey) {
        e.preventDefault();
        setPlanMode(p => !p);
      }
    };
    window.addEventListener('keydown', handler);
    return () => window.removeEventListener('keydown', handler);
  }, [threadId]);

  // --- Drag handlers ---

  const handleDragEnter = useCallback((e: DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    dragCounterRef.current++;
    if (e.dataTransfer?.types.includes('Files')) {
      setDragOver(true);
    }
  }, []);

  const handleDragOver = useCallback((e: DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
  }, []);

  const handleDragLeave = useCallback((e: DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    dragCounterRef.current--;
    if (dragCounterRef.current === 0) {
      setDragOver(false);
    }
  }, []);

  const handleDrop = useCallback((e: DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    dragCounterRef.current = 0;
    setDragOver(false);

    const droppedFiles = e.dataTransfer?.files;
    if (!droppedFiles || droppedFiles.length === 0) return;

    const valid: File[] = [];
    const errors: string[] = [];

    for (const file of Array.from(droppedFiles)) {
      if (file.size > MAX_FILE_SIZE) {
        errors.push(`${file.name}: file too large (max 10 MB)`);
      } else if (!isAllowedFile(file)) {
        errors.push(`Unsupported file type: ${getFileExtension(file)}`);
      } else {
        valid.push(file);
      }
    }

    if (errors.length > 0) {
      showFeedback('error', errors.join(', '));
    }
    if (valid.length > 0) {
      chatInputRef.current?.addFiles(valid);
    }
  }, [showFeedback]);

  // --- Thread data ---

  const reloadThread = useCallback(async () => {
    if (!threadId) return;
    try {
      const data: ThreadDetail = await api.getThread(threadId);
      setMessages(data.messages);
      setForkPoints(data.fork_points || {});
    } catch {
      // ignore
    }
  }, [threadId]);

  // --- Sync effects from SSE session ---

  // Sync title updates from SSE session to parent
  useEffect(() => {
    if (!sseSession?.titleUpdate) return;
    onTitleUpdateRef.current?.(sseSession.titleUpdate.threadId, sseSession.titleUpdate.title);
  }, [sseSession?.titleUpdate]);

  // Sync usageInfo from SSE session to local state (persists after stream ends)
  useEffect(() => {
    if (sseSession?.usageInfo) setUsageInfo(sseSession.usageInfo);
  }, [sseSession?.usageInfo]);

  // Fetch session health after each message completes (when usageInfo updates)
  useEffect(() => {
    if (!threadId || !usageInfo) return;
    let cancelled = false;
    fetchSessionHealth(threadId).then(health => {
      if (!cancelled) setSessionHealth(health);
    }).catch(() => { /* ignore */ });
    return () => { cancelled = true; };
  }, [threadId, usageInfo]);

  // Clear session health when thread changes
  useEffect(() => {
    setSessionHealth(null);
  }, [threadId]);

  // Sync memorySuggestions from SSE session to local state (persists after stream ends)
  useEffect(() => {
    if (sseSession && sseSession.memorySuggestions.length > 0) {
      setMemorySuggestions(sseSession.memorySuggestions);
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [sseSession?.memorySuggestions.length]);

  // --- Thread load effect ---

  useEffect(() => {
    messageQueueRef.current = [];
    setQueuedIds(new Set());
    setMemorySuggestions([]);
    setBranchFromId(null);
    setForkPoints({});
    if (!threadId) {
      setMessages([]);
      return;
    }
    setLoading(true);
    api.getThread(threadId)
      .then((data: ThreadDetail) => {
        setMessages(data.messages);
        setForkPoints(data.fork_points || {});
      })
      .catch(() => setMessages([]))
      .finally(() => setLoading(false));
  }, [threadId]);

  // --- Reconnect to active backend stream on mount ---
  // Handles browser refresh and returning to a thread with an active backend process.
  // If the SSE manager already has an active session, skip (it's already consuming the stream).
  useEffect(() => {
    if (!threadId) return;
    if (sseManager.hasActiveSession(threadId)) return;

    const subController = new AbortController();
    let active = true;

    (async () => {
      try {
        const stream = streamSubscribe(threadId, subController.signal);
        const first = await stream.next();
        if (first.done || !active) return;

        // Active stream found — register with the SSE manager
        const managerSignal = sseManager.startSession(threadId);
        onStreamingChangeRef.current?.(threadId);

        // Link manager abort to the subscribe controller so aborting the
        // manager session also tears down the underlying fetch
        managerSignal.addEventListener('abort', () => subController.abort(), { once: true });

        async function* wrappedStream(): AsyncGenerator<StreamChunk> {
          yield first.value!;
          yield* stream;
        }

        completionContextRef.current = { isBranching: false, isEdit: false };

        sseManager.runStream(threadId, () => wrappedStream());
      } catch (err) {
        if (err instanceof Error && err.name === 'AbortError') return;
        // On error, refetch messages in case the response completed
        if (active && currentThreadIdRef.current === threadId) {
          api.getThread(threadId).then((data: ThreadDetail) => {
            if (currentThreadIdRef.current === threadId) {
              setMessages(data.messages);
              setForkPoints(data.fork_points || {});
            }
          }).catch(() => {});
        }
      }
    })();

    return () => {
      active = false;
      // Only abort the subscribe controller if the manager isn't handling this thread
      if (!sseManager.hasActiveSession(threadId)) {
        subController.abort();
      }
    };
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [threadId]);

  // --- Pending starter message ---
  useEffect(() => {
    if (pendingStarterMessage && threadId && !sseManager.hasActiveSession(threadId) && !loading) {
      onStarterMessageConsumed?.();
      handleSend(pendingStarterMessage);
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [pendingStarterMessage, threadId, loading]);

  // --- Stick-to-bottom scroll tracking ---
  useEffect(() => {
    const container = scrollContainerRef.current;
    if (!container) return;

    const checkAtBottom = () => {
      const distance = container.scrollHeight - container.scrollTop - container.clientHeight;
      isAtBottomRef.current = distance < 50;
    };

    container.addEventListener('scroll', checkAtBottom, { passive: true });
    window.addEventListener('resize', checkAtBottom);

    return () => {
      container.removeEventListener('scroll', checkAtBottom);
      window.removeEventListener('resize', checkAtBottom);
    };
  }, []);

  // --- Auto-scroll ---
  const prevThreadIdRef = useRef<number | null>(null);
  const needsScrollAfterLoadRef = useRef(false);
  useEffect(() => {
    const container = scrollContainerRef.current;
    if (!container) return;
    const isThreadSwitch = prevThreadIdRef.current !== threadId;
    if (isThreadSwitch) {
      prevThreadIdRef.current = threadId ?? null;
      needsScrollAfterLoadRef.current = true;
      isAtBottomRef.current = true;
      requestAnimationFrame(() => {
        bottomRef.current?.scrollIntoView();
      });
      return;
    }
    if (needsScrollAfterLoadRef.current) {
      needsScrollAfterLoadRef.current = false;
      isAtBottomRef.current = true;
      requestAnimationFrame(() => {
        bottomRef.current?.scrollIntoView();
      });
      return;
    }
    if (userSentRef.current) {
      userSentRef.current = false;
      isAtBottomRef.current = true;
      bottomRef.current?.scrollIntoView();
      return;
    }
    if (isAtBottomRef.current) {
      bottomRef.current?.scrollIntoView({ behavior: 'smooth' });
    }
  }, [messages, streamingContent, streamingThinking, threadId]);

  // --- Stream completion handler ---

  const handleStreamCompletion = useCallback((tid: number) => {
    const session = sseManager.getSessionState(tid);
    if (!session || !session.isComplete) return;

    // Update UI state if still viewing this thread
    if (currentThreadIdRef.current === tid) {
      if (session.completedMessage) {
        setMessages(prev => [...prev, session.completedMessage!]);
        notifyResponse(session.completedMessage.content);
      }

      if (session.streamError === 'Server unavailable') {
        setStreamError('Server unavailable');
        startHealthPolling();
      } else if (session.streamError) {
        setStreamError(session.streamError);
        setStreamErrorRaw(session.streamErrorRaw ?? null);
      }
    }

    if (session.gotResponse) playCompletionSound();

    const ctx = completionContextRef.current;
    if (ctx.isBranching && currentThreadIdRef.current === tid) {
      setBranchFromId(null);
      reloadThread();
    }
    if (ctx.isEdit && currentThreadIdRef.current === tid) {
      reloadThread();
    }

    // Process message queue
    if (currentThreadIdRef.current === tid && messageQueueRef.current.length > 0) {
      const next = messageQueueRef.current.shift()!;
      setQueuedIds(new Set(messageQueueRef.current.map(q => q.id)));
      startStreamRef.current(next.content, next.files);
    } else {
      onStreamingChangeRef.current?.(null);
    }

    sseManager.clearSession(tid);
  }, [sseManager, notifyResponse, playCompletionSound, startHealthPolling, reloadThread]);

  handleStreamCompletionRef.current = handleStreamCompletion;

  // --- Reactive stream completion ---
  // When the SSE session becomes complete, run the completion handler and
  // reload messages from DB. This handles the case where the user navigated
  // away and back — the old .then() callbacks have stale closures from the
  // unmounted component, but this effect always runs in the current instance.
  useEffect(() => {
    if (!threadId || !sseSession?.isComplete) return;
    handleStreamCompletionRef.current(threadId);
    // Reload to replace temporary message with DB-persisted version
    api.getThread(threadId).then((data: ThreadDetail) => {
      if (currentThreadIdRef.current === threadId) {
        setMessages(data.messages);
        setForkPoints(data.fork_points || {});
        broadcastThreadUpdated(threadId);
      }
    }).catch(() => {});
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [threadId, sseSession?.isComplete]);

  // --- Start a stream via the SSE manager ---

  const startStreamInManager = useCallback((content: string, files?: File[], branchParentId?: number | null) => {
    if (!threadId) return;

    const isBranching = branchParentId != null;
    completionContextRef.current = { isBranching, isEdit: false };

    setStreamError(null);
    stopHealthPolling();
    onStreamingChange?.(threadId);

    sseManager.startSession(threadId);

    sseManager.runStream(
      threadId,
      (signal) => isBranching
        ? streamBranch(threadId, branchParentId, content, signal)
        : streamChat(threadId, content, signal, files, planMode),
      { retryStreamFn: (signal) => streamRegenerate(threadId, signal) },
    );
  }, [threadId, planMode, sseManager, stopHealthPolling, onStreamingChange]);

  startStreamRef.current = startStreamInManager;

  // --- Wrapper for backward compat ---
  const sendToBackend = useCallback((content: string, files?: File[], branchParentId?: number | null) => {
    startStreamInManager(content, files, branchParentId);
  }, [startStreamInManager]);

  // --- Send message ---

  const handleSend = useCallback((content: string, files?: File[]) => {
    if (!threadId) return;
    userSentRef.current = true;

    if (branchFromId != null) {
      const branchIdx = messages.findIndex((m) => m.id === branchFromId);
      if (branchIdx !== -1) {
        const truncated = messages.slice(0, branchIdx + 1);
        const userMsg: Message = {
          id: Date.now(),
          thread_id: threadId,
          role: 'user',
          content: content || '(attached files)',
          created_at: new Date().toISOString(),
        };
        setMessages([...truncated, userMsg]);
      }
      sendToBackend(content, undefined, branchFromId);
      return;
    }

    const userMsg: Message = {
      id: Date.now(),
      thread_id: threadId,
      role: 'user',
      content: content || '(attached files)',
      created_at: new Date().toISOString(),
    };
    setMessages((prev) => [...prev, userMsg]);
    broadcastNewMessage(threadId, userMsg);

    if (sseManager.hasActiveSession(threadId)) {
      const queued = { id: userMsg.id, content, files };
      messageQueueRef.current = [...messageQueueRef.current, queued];
      setQueuedIds(new Set(messageQueueRef.current.map((q) => q.id)));
      return;
    }

    sendToBackend(content, files);
  }, [threadId, sendToBackend, branchFromId, messages, sseManager, broadcastNewMessage]);

  // --- Stop (interrupt) ---

  const handleStop = useCallback(() => {
    if (!threadId) return;
    interruptThread(threadId).catch(() => {
      // Ignore errors — response may have already finished
    });
  }, [threadId]);

  // --- Regenerate ---

  const handleRegenerate = useCallback(async () => {
    if (!threadId || sseManager.hasActiveSession(threadId)) return;

    setMessages((prev) => {
      const last = prev[prev.length - 1];
      if (last?.role === 'assistant') return prev.slice(0, -1);
      return prev;
    });

    completionContextRef.current = { isBranching: false, isEdit: false };
    setStreamError(null);
    onStreamingChange?.(threadId);

    sseManager.startSession(threadId);

    sseManager.runStream(
      threadId,
      (signal) => streamRegenerate(threadId, signal),
    );
  }, [threadId, sseManager, onStreamingChange]);

  // --- Edit message ---

  const handleEdit = useCallback(async (messageId: number, content: string) => {
    if (!threadId || sseManager.hasActiveSession(threadId)) return;

    setMessages((prev) => {
      const idx = prev.findIndex((m) => m.id === messageId);
      if (idx === -1) return prev;
      return [
        ...prev.slice(0, idx),
        { ...prev[idx]!, content, id: Date.now() },
      ];
    });

    completionContextRef.current = { isBranching: false, isEdit: true };
    setStreamError(null);
    onStreamingChange?.(threadId);

    sseManager.startSession(threadId);

    sseManager.runStream(
      threadId,
      (signal) => streamEdit(threadId, messageId, content, signal),
    );
  }, [threadId, sseManager, onStreamingChange]);

  // --- Queue ---

  const removeQueuedMessage = useCallback((msgId: number) => {
    messageQueueRef.current = messageQueueRef.current.filter((q) => q.id !== msgId);
    setQueuedIds(new Set(messageQueueRef.current.map((q) => q.id)));
    setMessages((prev) => prev.filter((m) => m.id !== msgId));
  }, []);

  // --- Branching ---

  const handleBranch = useCallback((messageId: number) => {
    if (threadId && sseManager.hasActiveSession(threadId)) return;
    setBranchFromId(messageId);
    chatInputRef.current?.focus();
  }, [sseManager, threadId]);

  const handleSwitchBranch = useCallback(async (forkMessageId: number, childId: number) => {
    if (!threadId || sseManager.hasActiveSession(threadId)) return;
    try {
      await api.switchBranch(threadId, forkMessageId, childId);
      await reloadThread();
    } catch {
      // ignore
    }
  }, [threadId, sseManager, reloadThread]);

  // --- Hide/Unhide ---

  const handleHide = useCallback(async (messageId: number) => {
    try {
      const updated = await toggleMessageHidden(messageId);
      setMessages((prev) => prev.map((m) => m.id === messageId ? { ...m, hidden: updated.hidden } : m));
    } catch {
      // ignore
    }
  }, []);

  // --- Slash commands ---

  const handleSlashCommand = useCallback(async (command: string, args: string) => {
    const known = COMMANDS.some((c) => c.name === command);
    if (!known) {
      showFeedback('error', `Unknown command: ${command}`);
      return;
    }

    switch (command) {
      case '/new':
        onNewThread?.();
        break;

      case '/search':
        onOpenSearch?.();
        break;

      case '/status': {
        try {
          const status = await api.getStatus();
          const model = thread?.model || status.default_model || 'Default';
          const msgCount = messages.length;
          showFeedback('info', `Model: ${model} \u00B7 Messages: ${msgCount}`);
        } catch {
          showFeedback('error', 'Failed to fetch status');
        }
        break;
      }

      case '/model': {
        if (!threadId) {
          showFeedback('error', 'No active thread');
          return;
        }
        if (!args) {
          const currentModel = thread?.model || 'Default';
          showFeedback('info', `Current model: ${currentModel}. Usage: /model <model-name>`);
          return;
        }
        try {
          await api.updateModel(threadId, args);
          showFeedback('info', `Model switched to: ${args}`);
        } catch {
          showFeedback('error', 'Failed to update model');
        }
        break;
      }

      case '/export': {
        if (!threadId || messages.length === 0) {
          showFeedback('error', 'No messages to export');
          return;
        }
        const format: ExportFormat = args.trim().toLowerCase() === 'json' ? 'json' : 'md';
        downloadExport(messages, format, thread);
        showFeedback('info', `Thread exported as ${format === 'json' ? 'JSON' : 'markdown'}`);
        break;
      }

      case '/clear': {
        if (!threadId) {
          showFeedback('error', 'No active thread');
          return;
        }
        if (!clearConfirm) {
          setClearConfirm(true);
          showFeedback('info', 'Type /clear again to confirm clearing all messages');
          setTimeout(() => setClearConfirm(false), 5000);
          return;
        }
        try {
          sseManager.abortSession(threadId);
          await api.clearMessages(threadId);
          await api.clearSession(threadId);
          setMessages([]);
          setUsageInfo(null);
          setClearConfirm(false);
          showFeedback('info', 'Messages and session cleared');
        } catch {
          showFeedback('error', 'Failed to clear');
        }
        break;
      }

      case '/compact': {
        if (!threadId) {
          showFeedback('error', 'No active thread');
          return;
        }
        handleSend('/compact');
        showFeedback('info', 'Compacting context...');
        break;
      }

      case '/reset': {
        if (!threadId) {
          showFeedback('error', 'No active thread');
          return;
        }
        try {
          await api.clearSession(threadId);
          showFeedback('info', 'Claude session reset \u2014 next message starts fresh');
        } catch {
          showFeedback('error', 'Failed to reset session');
        }
        break;
      }
    }
  }, [threadId, thread, messages, onNewThread, onOpenSearch, showFeedback, clearConfirm, handleSend, sseManager]);

  // --- Derived render values ---

  const lastAssistantId = (() => {
    for (let i = messages.length - 1; i >= 0; i--) {
      if (messages[i]!.role === 'assistant') return messages[i]!.id;
    }
    return null;
  })();

  if (!threadId) {
    return (
      <div className="flex-1 flex items-center justify-center">
        <div className="text-center">
          <div className="w-12 h-12 mx-auto mb-4 rounded-2xl bg-zinc-100 flex items-center justify-center">
            <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="currentColor" className="w-6 h-6 text-emerald-500/60">
              <path d="M15.98 1.804a1 1 0 00-1.96 0l-.24 1.192a1 1 0 01-.784.784l-1.192.238a1 1 0 000 1.962l1.192.238a1 1 0 01.784.785l.24 1.192a1 1 0 001.96 0l.24-1.192a1 1 0 01.784-.785l1.192-.238a1 1 0 000-1.962l-1.192-.238a1 1 0 01-.784-.784l-.24-1.192zM6.735 5.803a1 1 0 00-1.47 0l-.863.864a1 1 0 01-.785.284l-1.22-.068a1 1 0 00-1.04 1.04l.069 1.22a1 1 0 01-.284.784l-.864.864a1 1 0 000 1.47l.864.863a1 1 0 01.284.785l-.069 1.22a1 1 0 001.04 1.04l1.22-.07a1 1 0 01.785.285l.863.864a1 1 0 001.47 0l.864-.864a1 1 0 01.785-.284l1.22.069a1 1 0 001.04-1.04l-.07-1.22a1 1 0 01.285-.785l.864-.863a1 1 0 000-1.47l-.864-.864a1 1 0 01-.284-.785l.069-1.22a1 1 0 00-1.04-1.04l-1.22.07a1 1 0 01-.785-.285l-.864-.864zM7 10a3 3 0 116 0 3 3 0 01-6 0z" />
            </svg>
          </div>
          <p className="text-xl font-light text-zinc-700 mb-1">Botka Chat</p>
          <p className="text-sm text-zinc-400">Start a new conversation</p>
        </div>
      </div>
    );
  }

  return (
    <div
      className="flex-1 flex flex-col min-h-0 relative"
      onDragEnter={handleDragEnter}
      onDragOver={handleDragOver}
      onDragLeave={handleDragLeave}
      onDrop={handleDrop}
    >
      <div ref={scrollContainerRef} className="flex-1 overflow-y-auto overscroll-contain px-4 py-6 transition-colors duration-200" style={{ backgroundColor: getThreadBackground(thread?.color, resolvedTheme) }}>
        <div className="max-w-3xl mx-auto">
          {loading && (
            <div className="flex justify-center py-12">
              <div className="loading-dots flex gap-1.5">
                <span />
                <span />
                <span />
              </div>
            </div>
          )}
          {messages.map((msg, idx) => {
            const fp = forkPoints[String(msg.id)] || (idx === 0 ? forkPoints["0"] : undefined);
            const forkId = forkPoints[String(msg.id)] ? msg.id : 0;
            return (
              <MessageBubble
                key={msg.id}
                message={msg}
                isLastAssistant={msg.id === lastAssistantId}
                isPending={queuedIds.has(msg.id)}
                forkPoint={fp}
                onEdit={msg.role === 'user' && !isStreamingThisThread ? handleEdit : undefined}
                onRegenerate={msg.id === lastAssistantId && !isStreamingThisThread ? handleRegenerate : undefined}
                onBranch={msg.role === 'assistant' && !isStreamingThisThread ? () => handleBranch(msg.id) : undefined}
                onHide={!isStreamingThisThread ? () => handleHide(msg.id) : undefined}
                onSwitchBranch={fp ? (childId: number) => handleSwitchBranch(forkId, childId) : undefined}
                onImageClick={(att, allImages) => setLightbox({ attachment: att, allImages })}
                onRemoveQueued={queuedIds.has(msg.id) ? () => removeQueuedMessage(msg.id) : undefined}
                onOptionSelect={msg.id === lastAssistantId && !isStreamingThisThread ? (text) => handleSend(text) : undefined}
              />
            );
          })}
          {isStreamingThisThread && (streamingContent || streamingThinking || activeToolCalls.length > 0) && (
            <>
              {activeToolCalls.length > 0 && (
                <div className="mb-2 max-w-3xl">
                  {activeToolCalls.map((tc) =>
                    tc.name === 'AskUserQuestion' && threadId ? (
                      <AskUserPanel
                        key={tc.id}
                        toolCall={tc}
                        threadId={threadId}
                      />
                    ) : (
                      <ToolCallPanel
                        key={tc.id}
                        name={tc.name}
                        input={tc.input}
                        result={tc.result}
                        isError={tc.isError}
                        isStreaming={tc.result == null}
                      />
                    ),
                  )}
                </div>
              )}
              {(streamingContent || streamingThinking) && (
                <MessageBubble
                  message={{
                    id: -1,
                    thread_id: threadId,
                    role: 'assistant',
                    content: streamingContent,
                    thinking: streamingThinking || undefined,
                    thinking_duration_ms: thinkingDurationMs ?? undefined,
                    created_at: new Date().toISOString(),
                  }}
                  isStreaming
                />
              )}
            </>
          )}
          {isStreamingThisThread && !streamingContent && !streamingThinking && (
            <div className="flex items-start gap-3 mb-6 animate-message-in">
              <div className="w-7 h-7 rounded-full bg-zinc-200 flex items-center justify-center flex-shrink-0 mt-1 hidden md:flex">
                <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="currentColor" className="w-3.5 h-3.5 text-zinc-500">
                  <path d="M15.98 1.804a1 1 0 00-1.96 0l-.24 1.192a1 1 0 01-.784.784l-1.192.238a1 1 0 000 1.962l1.192.238a1 1 0 01.784.785l.24 1.192a1 1 0 001.96 0l.24-1.192a1 1 0 01.784-.785l1.192-.238a1 1 0 000-1.962l-1.192-.238a1 1 0 01-.784-.784l-.24-1.192zM6.735 5.803a1 1 0 00-1.47 0l-.863.864a1 1 0 01-.785.284l-1.22-.068a1 1 0 00-1.04 1.04l.069 1.22a1 1 0 01-.284.784l-.864.864a1 1 0 000 1.47l.864.863a1 1 0 01.284.785l-.069 1.22a1 1 0 001.04 1.04l1.22-.07a1 1 0 01.785.285l.863.864a1 1 0 001.47 0l.864-.864a1 1 0 01.785-.284l1.22.069a1 1 0 001.04-1.04l-.07-1.22a1 1 0 01.285-.785l.864-.863a1 1 0 000-1.47l-.864-.864a1 1 0 01-.284-.785l.069-1.22a1 1 0 00-1.04-1.04l-1.22.07a1 1 0 01-.785-.285l-.864-.864zM7 10a3 3 0 116 0 3 3 0 01-6 0z" />
                </svg>
              </div>
              {reconnecting ? (
                <div className="flex items-center gap-2 pt-2.5">
                  <svg className="w-4 h-4 text-amber-500 animate-spin" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                    <path d="M4 12a8 8 0 018-8v4l4-4-4-4v4a10 10 0 100 20 10 10 0 006.32-2.26" strokeLinecap="round" />
                  </svg>
                  <span className="text-sm text-amber-600">
                    Connection lost, reconnecting ({reconnecting.attempt}/{reconnecting.maxAttempts})...
                  </span>
                </div>
              ) : retryInfo ? (
                <div className="flex items-center gap-2 pt-2.5">
                  <svg className="w-4 h-4 text-amber-500 animate-spin" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                    <path d="M4 12a8 8 0 018-8v4l4-4-4-4v4a10 10 0 100 20 10 10 0 006.32-2.26" strokeLinecap="round" />
                  </svg>
                  <span className="text-sm text-amber-600">
                    Retrying ({retryInfo.attempt}/{retryInfo.max_attempts})...
                  </span>
                </div>
              ) : (
                <div className="loading-dots flex gap-1.5 pt-3">
                  <span />
                  <span />
                  <span />
                </div>
              )}
            </div>
          )}
          {memorySuggestions.length > 0 && !isStreamingThisThread && (
            <div className="space-y-2 mb-4">
              {memorySuggestions.map((suggestion, idx) => (
                <div key={idx} className="flex items-start gap-3 px-3 py-2.5 rounded-xl bg-amber-50 border border-amber-200 animate-message-in">
                  <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="currentColor" className="w-4 h-4 text-amber-500 flex-shrink-0 mt-0.5">
                    <path d="M10 1a.75.75 0 01.75.75v1.5a.75.75 0 01-1.5 0v-1.5A.75.75 0 0110 1zM5.05 3.05a.75.75 0 011.06 0l1.062 1.06A.75.75 0 116.11 5.173L5.05 4.11a.75.75 0 010-1.06zm9.9 0a.75.75 0 010 1.06l-1.06 1.062a.75.75 0 01-1.062-1.061l1.061-1.06a.75.75 0 011.06 0zM10 7a3 3 0 100 6 3 3 0 000-6zm-6.25 3a.75.75 0 01-.75.75H1.5a.75.75 0 010-1.5H3a.75.75 0 01.75.75zm14.5 0a.75.75 0 01-.75.75h-1.5a.75.75 0 010-1.5H17a.75.75 0 01.75.75zm-12.14 4.89a.75.75 0 010 1.06l-1.06 1.06a.75.75 0 11-1.06-1.06l1.06-1.06a.75.75 0 011.06 0zm8.28 0a.75.75 0 011.06 0l1.06 1.06a.75.75 0 01-1.06 1.06l-1.06-1.06a.75.75 0 010-1.06zM10 15a.75.75 0 01.75.75v1.5a.75.75 0 01-1.5 0v-1.5A.75.75 0 0110 15z" />
                  </svg>
                  <span className="text-sm text-amber-800 flex-1">{suggestion}</span>
                  <div className="flex gap-1.5 flex-shrink-0">
                    <button
                      onClick={async () => {
                        try {
                          await api.createMemory(suggestion);
                          setMemorySuggestions((prev) => prev.filter((_, i) => i !== idx));
                        } catch (err) {
                          showFeedback('error', err instanceof Error ? err.message : 'Failed to save memory');
                        }
                      }}
                      className="px-2.5 py-1 text-xs font-medium rounded-lg bg-amber-200/60 hover:bg-amber-200 text-amber-800 transition-colors cursor-pointer"
                    >
                      Save
                    </button>
                    <button
                      onClick={() => setMemorySuggestions((prev) => prev.filter((_, i) => i !== idx))}
                      className="px-2.5 py-1 text-xs font-medium rounded-lg bg-zinc-100 hover:bg-zinc-200 text-zinc-500 transition-colors cursor-pointer"
                    >
                      Dismiss
                    </button>
                  </div>
                </div>
              ))}
            </div>
          )}
          {streamError && streamError !== 'Server unavailable' && !isStreamingThisThread && (
            <div className="max-w-3xl animate-message-in">
              <StreamErrorBlock
                message={streamError}
                raw={streamErrorRaw}
                onRetry={() => { setStreamError(null); setStreamErrorRaw(null); handleRegenerate(); }}
              />
            </div>
          )}
          <div ref={bottomRef} />
        </div>
      </div>
      {connectionStatus === 'offline' && (
        <div className="max-w-3xl mx-auto w-full px-4">
          <div className="flex items-center gap-3 text-sm px-4 py-2.5 rounded-xl mb-1 bg-zinc-100 text-zinc-600 border border-zinc-200">
            <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="currentColor" className="w-4 h-4 flex-shrink-0 text-zinc-400">
              <path fillRule="evenodd" d="M2.628 1.601C5.028 1.206 7.49 1 10 1s4.973.206 7.372.601a.75.75 0 01-.249 1.479A49.71 49.71 0 0010 2.5a49.71 49.71 0 00-7.123.58.75.75 0 11-.249-1.479zM.712 8.157A.75.75 0 011.269 7.2c2.56-.74 5.6-1.2 8.731-1.2 3.13 0 6.17.46 8.73 1.2a.75.75 0 11-.436 1.432A39.524 39.524 0 0010 7.5a39.524 39.524 0 00-8.294 1.332.75.75 0 01-.994-.675zm2.539 3.18a.75.75 0 01.984-.58c1.823.517 3.903.843 5.765.843s3.942-.326 5.765-.843a.75.75 0 01.984.58.748.748 0 01-.58.984c-1.99.564-4.15.904-6.169.904s-4.18-.34-6.17-.904a.75.75 0 01-.579-.984zM10 15a1 1 0 100 2 1 1 0 000-2z" clipRule="evenodd" />
            </svg>
            <span className="flex-1">No internet connection</span>
          </div>
        </div>
      )}
      {connectionStatus === 'unavailable' && (
        <div className="max-w-3xl mx-auto w-full px-4">
          <div className="flex items-center gap-3 text-sm px-4 py-2.5 rounded-xl mb-1 bg-amber-50 text-amber-700 border border-amber-200">
            <svg className="w-4 h-4 flex-shrink-0 text-amber-500 animate-pulse" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="currentColor">
              <path fillRule="evenodd" d="M8.485 2.495c.673-1.167 2.357-1.167 3.03 0l6.28 10.875c.673 1.167-.17 2.625-1.516 2.625H3.72c-1.347 0-2.189-1.458-1.515-2.625L8.485 2.495zM10 5a.75.75 0 01.75.75v3.5a.75.75 0 01-1.5 0v-3.5A.75.75 0 0110 5zm0 9a1 1 0 100-2 1 1 0 000 2z" clipRule="evenodd" />
            </svg>
            <span className="flex-1">Server unavailable — reconnecting in the background...</span>
          </div>
        </div>
      )}
      {streamError === 'Server unavailable' && !isStreamingThisThread && (
        <div className="max-w-3xl mx-auto w-full px-4">
          <div className="flex items-center gap-3 text-sm px-4 py-2.5 rounded-xl mb-1 bg-red-50 text-red-700 border border-red-200">
            <span className="flex-1 truncate">{streamError}</span>
            <button
              onClick={() => { setStreamError(null); stopHealthPolling(); handleRegenerate(); }}
              className="flex-shrink-0 px-3 py-1 rounded-lg bg-red-100 hover:bg-red-200 text-red-700 text-xs font-medium transition-colors"
            >
              Try again
            </button>
            <button
              onClick={() => { setStreamError(null); stopHealthPolling(); }}
              className="flex-shrink-0 text-red-400 hover:text-red-600 transition-colors"
            >
              <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="currentColor" className="w-4 h-4">
                <path d="M6.28 5.22a.75.75 0 00-1.06 1.06L8.94 10l-3.72 3.72a.75.75 0 101.06 1.06L10 11.06l3.72 3.72a.75.75 0 101.06-1.06L11.06 10l3.72-3.72a.75.75 0 00-1.06-1.06L10 8.94 6.28 5.22z" />
              </svg>
            </button>
          </div>
        </div>
      )}
      {commandFeedback && (
        <div className="max-w-3xl mx-auto w-full px-4">
          <div className={`text-sm px-4 py-2.5 rounded-xl mb-1 ${
            commandFeedback.type === 'error'
              ? 'bg-red-50 text-red-700 border border-red-200'
              : 'bg-zinc-100 text-zinc-600 border border-zinc-200'
          }`}>
            {commandFeedback.text}
          </div>
        </div>
      )}
      {branchFromId != null && (
        <div className="max-w-3xl mx-auto w-full px-4">
          <div className="flex items-center gap-3 text-sm px-4 py-2 rounded-xl mb-1 bg-indigo-50 text-indigo-700 border border-indigo-200">
            <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16" fill="currentColor" className="w-4 h-4 flex-shrink-0 text-indigo-500">
              <path fillRule="evenodd" d="M4.75 2a.75.75 0 0 1 .75.75v3.5c0 .414.336.75.75.75h2.032l-.721-.72a.75.75 0 0 1 1.06-1.061l2 2a.75.75 0 0 1 0 1.06l-2 2a.75.75 0 1 1-1.06-1.06l.72-.72H6.25A2.25 2.25 0 0 1 4 6.25V2.75A.75.75 0 0 1 4.75 2ZM4 9.25a.75.75 0 0 1 .75-.75h6.5a.75.75 0 0 1 0 1.5h-6.5a.75.75 0 0 1-.75-.75ZM4.75 12a.75.75 0 0 0 0 1.5h6.5a.75.75 0 0 0 0-1.5h-6.5Z" clipRule="evenodd" />
            </svg>
            <span className="flex-1">Branching — type a new question to explore an alternative path</span>
            <button
              onClick={() => setBranchFromId(null)}
              className="flex-shrink-0 text-indigo-400 hover:text-indigo-600 transition-colors cursor-pointer"
              title="Cancel branching"
            >
              <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="currentColor" className="w-4 h-4">
                <path d="M6.28 5.22a.75.75 0 00-1.06 1.06L8.94 10l-3.72 3.72a.75.75 0 101.06 1.06L10 11.06l3.72 3.72a.75.75 0 101.06-1.06L11.06 10l3.72-3.72a.75.75 0 00-1.06-1.06L10 8.94 6.28 5.22z" />
              </svg>
            </button>
          </div>
        </div>
      )}
      <div className="max-w-3xl mx-auto w-full">
        {usageInfo && (usageInfo.input_tokens || usageInfo.cost_usd) && (
          <div className="flex items-center gap-3 px-4 pb-1 text-[10px] text-zinc-400">
            {usageInfo.input_tokens != null && usageInfo.input_tokens > 0 && (() => {
              const contextWindow = thread?.model === 'opus' ? 1_000_000 : 200_000;
              const used = usageInfo.input_tokens!;
              const pct = Math.min((used / contextWindow) * 100, 100);
              const formatK = (n: number) => n >= 1000 ? `${(n / 1000).toFixed(1)}k` : String(n);
              const barColor = pct > 80 ? 'bg-red-500' : pct > 50 ? 'bg-amber-500' : 'bg-emerald-500';
              const pctColor = pct > 80 ? 'text-red-400' : pct > 50 ? 'text-amber-400' : 'text-zinc-400';
              const tooltipLines = [
                `Context: ${formatK(used)} / ${formatK(contextWindow)} (${pct.toFixed(1)}%)`,
                `Input tokens: ${usageInfo.input_tokens?.toLocaleString()}`,
                `Output tokens: ${usageInfo.output_tokens?.toLocaleString() || '\u2014'}`,
              ];
              if (sessionHealth?.active) {
                tooltipLines.push(`Total input: ${sessionHealth.total_input_tokens?.toLocaleString()}`);
                tooltipLines.push(`Total output: ${sessionHealth.total_output_tokens?.toLocaleString()}`);
                tooltipLines.push(`Messages: ${sessionHealth.message_count}`);
                if (sessionHealth.started_at) {
                  const elapsed = Math.floor((Date.now() - new Date(sessionHealth.started_at).getTime()) / 1000);
                  const mins = Math.floor(elapsed / 60);
                  const secs = elapsed % 60;
                  tooltipLines.push(`Session uptime: ${mins}m ${secs}s`);
                }
              }
              return (
                <div className="flex items-center gap-1.5 flex-1 min-w-0" title={tooltipLines.join('\n')}>
                  <div className="flex-1 h-1 bg-zinc-200 rounded-full overflow-hidden min-w-[60px]">
                    <div
                      className={`h-full rounded-full transition-all ${barColor}`}
                      style={{ width: `${pct}%` }}
                    />
                  </div>
                  <span className={pctColor}>{Math.round(pct)}%</span>
                  <span>{formatK(used)} / {formatK(contextWindow)}</span>
                  {sessionHealth?.active && sessionHealth.message_count != null && (
                    <span className="text-zinc-300">{sessionHealth.message_count} msg</span>
                  )}
                </div>
              );
            })()}
            {(() => {
              const historyCost = messages.reduce((sum, m) => sum + (m.cost_usd || 0), 0);
              // Only add current stream cost when actively streaming (message not yet persisted to DB)
              const currentCost = isStreamingThisThread ? (usageInfo.cost_usd || 0) : 0;
              const totalCost = historyCost + currentCost;
              return totalCost > 0 ? <span>${totalCost.toFixed(4)}</span> : null;
            })()}
          </div>
        )}
        {!usageInfo && (() => {
          const totalCost = messages.reduce((sum, m) => sum + (m.cost_usd || 0), 0);
          return totalCost > 0 ? (
            <div className="flex items-center justify-end px-4 pb-1 text-[10px] text-zinc-400">
              <span>${totalCost.toFixed(4)}</span>
            </div>
          ) : null;
        })()}
        <ChatInput key={threadId} ref={chatInputRef} threadId={threadId} onSend={handleSend} onSlashCommand={handleSlashCommand} queuedCount={queuedIds.size} planMode={planMode} onTogglePlanMode={() => setPlanMode((p) => !p)} isStreaming={isStreamingThisThread} onStop={handleStop} />
      </div>
      {lightbox && (
        <Lightbox
          attachment={lightbox.attachment}
          allImages={lightbox.allImages}
          onClose={() => setLightbox(null)}
        />
      )}
      {dragOver && (
        <div className="absolute inset-0 bg-zinc-50/80 backdrop-blur-sm border-2 border-dashed border-emerald-400/40 rounded-xl flex flex-col items-center justify-center pointer-events-none z-50 transition-all">
          <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="currentColor" className="w-10 h-10 text-emerald-500/60 mb-3">
            <path fillRule="evenodd" d="M11.47 2.47a.75.75 0 011.06 0l4.5 4.5a.75.75 0 01-1.06 1.06l-3.22-3.22V16.5a.75.75 0 01-1.5 0V4.81L8.03 8.03a.75.75 0 01-1.06-1.06l4.5-4.5zM3 15.75a.75.75 0 01.75.75v2.25a1.5 1.5 0 001.5 1.5h13.5a1.5 1.5 0 001.5-1.5V16.5a.75.75 0 011.5 0v2.25a3 3 0 01-3 3H5.25a3 3 0 01-3-3V16.5a.75.75 0 01.75-.75z" clipRule="evenodd" />
          </svg>
          <span className="text-emerald-600 text-base font-medium">Drop files here</span>
        </div>
      )}
    </div>
  );
}
