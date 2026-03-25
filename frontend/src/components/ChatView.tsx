import { useState, useEffect, useRef, useCallback, type DragEvent } from 'react';
import type { Message, Thread, ThreadDetail, Attachment, ForkPoint } from '../types';
import { api, streamChat, streamRegenerate, streamEdit, streamBranch } from '../api/client';
import type { StreamChunk } from '../api/client';
import MessageBubble from './MessageBubble';
import ChatInput, { isAllowedFile, getFileExtension, MAX_FILE_SIZE } from './ChatInput';
import type { ChatInputHandle } from './ChatInput';
import ToolCallPanel from './ToolCallPanel';
import Lightbox from './Lightbox';
import { COMMANDS } from './SlashCommandMenu';
import { downloadExport } from '../utils/exportThread';
import type { ExportFormat } from '../utils/exportThread';
import { useNotifications } from '../hooks/useNotifications';
import { useConnectionStatus } from '../hooks/useConnectionStatus';

interface ActiveToolCall {
  id: string;
  name: string;
  input: Record<string, unknown>;
  result?: string;
  isError?: boolean;
}

class StreamError extends Error {
  connectionLost: boolean;
  constructor(message: string, connectionLost: boolean) {
    super(message);
    this.connectionLost = connectionLost;
  }
}

const RETRY_DELAYS = [1000, 2000, 4000];

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
  const [messages, setMessages] = useState<Message[]>([]);
  const [streamingContent, setStreamingContent] = useState('');
  const [streamingThinking, setStreamingThinking] = useState('');
  const [thinkingDurationMs, setThinkingDurationMs] = useState<number | null>(null);
  const [isStreaming, setIsStreaming] = useState(false);
  const [loading, setLoading] = useState(false);
  const [commandFeedback, setCommandFeedback] = useState<{ type: 'info' | 'error'; text: string } | null>(null);
  const [clearConfirm, setClearConfirm] = useState(false);
  const [lightbox, setLightbox] = useState<{ attachment: Attachment; allImages: Attachment[] } | null>(null);
  const [dragOver, setDragOver] = useState(false);
  const [queuedIds, setQueuedIds] = useState<Set<number>>(new Set());
  const [retryInfo, setRetryInfo] = useState<{ attempt: number; max_attempts: number } | null>(null);
  const [streamError, setStreamError] = useState<string | null>(null);
  const [reconnecting, setReconnecting] = useState<{ attempt: number; maxAttempts: number } | null>(null);
  const [memorySuggestions, setMemorySuggestions] = useState<string[]>([]);
  const [forkPoints, setForkPoints] = useState<Record<string, ForkPoint>>({});
  const [activeToolCalls, setActiveToolCalls] = useState<ActiveToolCall[]>([]);
  const [usageInfo, setUsageInfo] = useState<{ cost_usd?: number; input_tokens?: number; output_tokens?: number } | null>(null);
  const [branchFromId, setBranchFromId] = useState<number | null>(null);
  const [planMode, setPlanMode] = useState(false);
  const dragCounterRef = useRef(0);
  const chatInputRef = useRef<ChatInputHandle>(null);
  const bottomRef = useRef<HTMLDivElement>(null);
  const scrollContainerRef = useRef<HTMLDivElement>(null);
  const userSentRef = useRef(false);
  const abortRef = useRef<AbortController | null>(null);
  const feedbackTimer = useRef<ReturnType<typeof setTimeout>>(undefined);
  const messageQueueRef = useRef<{ id: number; content: string; files?: File[] }[]>([]);
  const streamingForThreadRef = useRef<number | null>(null);
  const streamContentRef = useRef('');
  const streamThinkingRef = useRef('');
  const streamThinkingDurationRef = useRef<number | null>(null);
  const currentThreadIdRef = useRef(threadId);
  currentThreadIdRef.current = threadId;
  const { notifyResponse } = useNotifications();
  const { status: connectionStatus, startHealthPolling, stopHealthPolling } = useConnectionStatus();

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

  // Global Shift+Tab handler for plan mode toggle (works regardless of focus)
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

  useEffect(() => {
    messageQueueRef.current = [];
    setQueuedIds(new Set());
    setMemorySuggestions([]);
    setBranchFromId(null);
    setForkPoints({});
    if (streamingForThreadRef.current === threadId) {
      setStreamingContent(streamContentRef.current);
      setStreamingThinking(streamThinkingRef.current);
      setThinkingDurationMs(streamThinkingDurationRef.current);
    }
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

  useEffect(() => {
    if (pendingStarterMessage && threadId && !(streamingForThreadRef.current === threadId) && !loading) {
      onStarterMessageConsumed?.();
      handleSend(pendingStarterMessage);
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [pendingStarterMessage, threadId, loading]);

  const prevThreadIdRef = useRef<number | null>(null);
  const needsScrollAfterLoadRef = useRef(false);
  useEffect(() => {
    const container = scrollContainerRef.current;
    if (!container) return;
    const isThreadSwitch = prevThreadIdRef.current !== threadId;
    if (isThreadSwitch) {
      prevThreadIdRef.current = threadId ?? null;
      needsScrollAfterLoadRef.current = true;
      requestAnimationFrame(() => {
        bottomRef.current?.scrollIntoView();
      });
      return;
    }
    if (needsScrollAfterLoadRef.current) {
      needsScrollAfterLoadRef.current = false;
      requestAnimationFrame(() => {
        bottomRef.current?.scrollIntoView();
      });
      return;
    }
    if (userSentRef.current) {
      userSentRef.current = false;
      bottomRef.current?.scrollIntoView();
      return;
    }
    const distanceFromBottom = container.scrollHeight - container.scrollTop - container.clientHeight;
    if (distanceFromBottom < 150) {
      bottomRef.current?.scrollIntoView();
    }
  }, [messages, streamingContent, streamingThinking, threadId]);

  const consumeStream = useCallback(async (stream: AsyncGenerator<StreamChunk>): Promise<Message | null> => {
    let fullContent = '';
    let fullThinking = '';
    let durationMs: number | null = null;
    let promptTokens: number | undefined;
    let completionTokens: number | undefined;

    for await (const chunk of stream) {
      if (chunk.done) break;
      if (chunk.retry) {
        setRetryInfo({ attempt: chunk.retry.attempt, max_attempts: chunk.retry.max_attempts });
        continue;
      }
      if (chunk.error) {
        setRetryInfo(null);
        throw new StreamError(chunk.error, chunk.connectionLost ?? false);
      }
      if (chunk.title && onTitleUpdate && threadId) {
        onTitleUpdate(threadId, chunk.title);
        continue;
      }
      if (chunk.usage) {
        promptTokens = chunk.usage.prompt_tokens;
        completionTokens = chunk.usage.completion_tokens;
        setUsageInfo({
          cost_usd: chunk.usage.cost_usd,
          input_tokens: chunk.usage.input_tokens,
          output_tokens: chunk.usage.output_tokens,
        });
        continue;
      }
      if (chunk.memory_suggestion) {
        setMemorySuggestions((prev) => [...prev, chunk.memory_suggestion!]);
        continue;
      }
      if (chunk.tool_use) {
        const tc = chunk.tool_use;
        setActiveToolCalls((prev) => [...prev, { id: tc.id, name: tc.name, input: tc.input }]);
        continue;
      }
      if (chunk.tool_result) {
        const tr = chunk.tool_result;
        setActiveToolCalls((prev) =>
          prev.map((tc) =>
            tc.id === tr.tool_use_id ? { ...tc, result: tr.content, isError: tr.is_error } : tc
          )
        );
        continue;
      }
      if (chunk.content || chunk.thinking) {
        setRetryInfo(null);
      }
      if (chunk.thinking) {
        fullThinking += chunk.thinking;
        streamThinkingRef.current = fullThinking;
        if (currentThreadIdRef.current === threadId) {
          setStreamingThinking(fullThinking);
        }
      }
      if (chunk.thinking_done) {
        durationMs = chunk.thinking_done.duration_ms;
        streamThinkingDurationRef.current = durationMs;
        if (currentThreadIdRef.current === threadId) {
          setThinkingDurationMs(durationMs);
        }
      }
      if (chunk.content) {
        fullContent += chunk.content;
        const clean = fullContent.replace(/<memory>.*?<\/memory>/gs, '').trim();
        streamContentRef.current = clean;
        if (currentThreadIdRef.current === threadId) {
          setStreamingContent(clean);
        }
      }
    }

    setRetryInfo(null);
    const cleanContent = fullContent.replace(/<memory>.*?<\/memory>/gs, '').trim();
    if (cleanContent) {
      return {
        id: Date.now() + 1,
        thread_id: threadId!,
        role: 'assistant',
        content: cleanContent,
        thinking: fullThinking || undefined,
        thinking_duration_ms: durationMs ?? undefined,
        prompt_tokens: promptTokens,
        completion_tokens: completionTokens,
        created_at: new Date().toISOString(),
      };
    }
    return null;
  }, [threadId, onTitleUpdate]);

  const sendToBackend = useCallback(async (content: string, files?: File[], branchParentId?: number | null) => {
    if (!threadId) return;

    if (abortRef.current) abortRef.current.abort();

    streamingForThreadRef.current = threadId;
    streamContentRef.current = '';
    streamThinkingRef.current = '';
    streamThinkingDurationRef.current = null;
    onStreamingChange?.(threadId);

    setStreamingContent('');
    setStreamingThinking('');
    setThinkingDurationMs(null);
    setStreamError(null);
    setReconnecting(null);
    setActiveToolCalls([]);
    setIsStreaming(true);
    stopHealthPolling();

    const isBranching = branchParentId != null;

    const controller = new AbortController();
    abortRef.current = controller;

    let succeeded = false;
    let gotResponse = false;
    try {
      const stream = isBranching
        ? streamBranch(threadId, branchParentId, content, controller.signal)
        : streamChat(threadId, content, controller.signal, files, planMode);
      const assistantMsg = await consumeStream(stream);
      if (assistantMsg) {
        if (currentThreadIdRef.current === threadId) {
          setMessages((prev) => [...prev, assistantMsg]);
        }
        notifyResponse(assistantMsg.content);
        gotResponse = true;
      }
      succeeded = true;
    } catch (err: unknown) {
      if (err instanceof Error && err.name === 'AbortError') {
        succeeded = true;
      } else if (err instanceof StreamError && err.connectionLost) {
        for (let i = 0; i < RETRY_DELAYS.length; i++) {
          setStreamingContent('');
          setStreamingThinking('');
          setThinkingDurationMs(null);
          setReconnecting({ attempt: i + 1, maxAttempts: RETRY_DELAYS.length });

          await new Promise((r) => setTimeout(r, RETRY_DELAYS[i]));
          if (controller.signal.aborted) break;

          try {
            const retryMsg = await consumeStream(streamRegenerate(threadId, controller.signal));
            if (retryMsg) {
              if (currentThreadIdRef.current === threadId) {
                setMessages((prev) => [...prev, retryMsg]);
              }
              notifyResponse(retryMsg.content);
              gotResponse = true;
            }
            succeeded = true;
            break;
          } catch (retryErr: unknown) {
            if (retryErr instanceof Error && retryErr.name === 'AbortError') {
              succeeded = true;
              break;
            }
            if (!(retryErr instanceof StreamError && retryErr.connectionLost)) {
              setStreamError(retryErr instanceof Error ? retryErr.message : 'Unknown error');
              succeeded = true;
              break;
            }
          }
        }

        if (!succeeded) {
          setStreamError('Server unavailable');
          startHealthPolling();
        }
      } else {
        setStreamError(err instanceof Error ? err.message : 'Unknown error');
        succeeded = true;
      }
    } finally {
      const isActiveStream = streamingForThreadRef.current === threadId;

      if (isActiveStream) {
        setStreamingContent('');
        setStreamingThinking('');
        setThinkingDurationMs(null);
        streamContentRef.current = '';
        streamThinkingRef.current = '';
        streamThinkingDurationRef.current = null;
      }
      setRetryInfo(null);
      setReconnecting(null);
      if (abortRef.current === controller) {
        abortRef.current = null;
      }

      if (isBranching && isActiveStream) {
        setBranchFromId(null);
        await reloadThread();
      }

      if (isActiveStream && messageQueueRef.current.length > 0) {
        const next = messageQueueRef.current[0];
        messageQueueRef.current = messageQueueRef.current.slice(1);
        setQueuedIds(new Set(messageQueueRef.current.map((q) => q.id)));
        sendToBackend(next!.content, next!.files);
      } else if (isActiveStream) {
        setIsStreaming(false);
        streamingForThreadRef.current = null;
        onStreamingChange?.(null);
        if (gotResponse) playCompletionSound();
      }
    }
  }, [threadId, planMode, consumeStream, notifyResponse, startHealthPolling, stopHealthPolling, playCompletionSound, reloadThread, onStreamingChange]);

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

    if (isStreaming && streamingForThreadRef.current === threadId) {
      const queued = { id: userMsg.id, content, files };
      messageQueueRef.current = [...messageQueueRef.current, queued];
      setQueuedIds(new Set(messageQueueRef.current.map((q) => q.id)));
      return;
    }

    sendToBackend(content, files);
  }, [threadId, isStreaming, sendToBackend, branchFromId, messages]);

  const handleRegenerate = useCallback(async () => {
    if (!threadId || (isStreaming && streamingForThreadRef.current === threadId)) return;

    if (abortRef.current) abortRef.current.abort();

    setMessages((prev) => {
      const last = prev[prev.length - 1];
      if (last?.role === 'assistant') return prev.slice(0, -1);
      return prev;
    });

    streamingForThreadRef.current = threadId;
    streamContentRef.current = '';
    streamThinkingRef.current = '';
    streamThinkingDurationRef.current = null;
    onStreamingChange?.(threadId);

    setStreamingContent('');
    setStreamingThinking('');
    setThinkingDurationMs(null);
    setStreamError(null);
    setIsStreaming(true);

    const controller = new AbortController();
    abortRef.current = controller;

    let gotResponse = false;
    try {
      const assistantMsg = await consumeStream(streamRegenerate(threadId, controller.signal));
      if (assistantMsg) {
        if (currentThreadIdRef.current === threadId) {
          setMessages((prev) => [...prev, assistantMsg]);
        }
        notifyResponse(assistantMsg.content);
        gotResponse = true;
      }
    } catch (err: unknown) {
      if (err instanceof Error && err.name !== 'AbortError') {
        setStreamError(err.message || 'Unknown error');
      }
    } finally {
      const isActiveStream = streamingForThreadRef.current === threadId;
      if (isActiveStream) {
        setStreamingContent('');
        setStreamingThinking('');
        setThinkingDurationMs(null);
        streamContentRef.current = '';
        streamThinkingRef.current = '';
        streamThinkingDurationRef.current = null;
        setIsStreaming(false);
        streamingForThreadRef.current = null;
        onStreamingChange?.(null);
      }
      setRetryInfo(null);
      if (abortRef.current === controller) {
        abortRef.current = null;
      }
      if (gotResponse) playCompletionSound();
    }
  }, [threadId, isStreaming, consumeStream, notifyResponse, playCompletionSound, onStreamingChange]);

  const handleEdit = useCallback(async (messageId: number, content: string) => {
    if (!threadId || (isStreaming && streamingForThreadRef.current === threadId)) return;

    if (abortRef.current) abortRef.current.abort();

    setMessages((prev) => {
      const idx = prev.findIndex((m) => m.id === messageId);
      if (idx === -1) return prev;
      return [
        ...prev.slice(0, idx),
        { ...prev[idx]!, content, id: Date.now() },
      ];
    });

    streamingForThreadRef.current = threadId;
    streamContentRef.current = '';
    streamThinkingRef.current = '';
    streamThinkingDurationRef.current = null;
    onStreamingChange?.(threadId);

    setStreamingContent('');
    setStreamingThinking('');
    setThinkingDurationMs(null);
    setStreamError(null);
    setIsStreaming(true);

    const controller = new AbortController();
    abortRef.current = controller;

    let gotResponse = false;
    try {
      const assistantMsg = await consumeStream(streamEdit(threadId, messageId, content, controller.signal));
      if (assistantMsg) {
        if (currentThreadIdRef.current === threadId) {
          setMessages((prev) => [...prev, assistantMsg]);
        }
        notifyResponse(assistantMsg.content);
        gotResponse = true;
      }
    } catch (err: unknown) {
      if (err instanceof Error && err.name !== 'AbortError') {
        setStreamError(err.message || 'Unknown error');
      }
    } finally {
      const isActiveStream = streamingForThreadRef.current === threadId;
      if (isActiveStream) {
        setStreamingContent('');
        setStreamingThinking('');
        setThinkingDurationMs(null);
        streamContentRef.current = '';
        streamThinkingRef.current = '';
        streamThinkingDurationRef.current = null;
        setIsStreaming(false);
        streamingForThreadRef.current = null;
        onStreamingChange?.(null);
      }
      setRetryInfo(null);
      if (abortRef.current === controller) {
        abortRef.current = null;
      }
      if (gotResponse) playCompletionSound();
      if (isActiveStream) await reloadThread();
    }
  }, [threadId, isStreaming, consumeStream, notifyResponse, playCompletionSound, reloadThread, onStreamingChange]);

  const removeQueuedMessage = useCallback((msgId: number) => {
    messageQueueRef.current = messageQueueRef.current.filter((q) => q.id !== msgId);
    setQueuedIds(new Set(messageQueueRef.current.map((q) => q.id)));
    setMessages((prev) => prev.filter((m) => m.id !== msgId));
  }, []);

  const handleBranch = useCallback((messageId: number) => {
    if (isStreaming && streamingForThreadRef.current === threadId) return;
    setBranchFromId(messageId);
    chatInputRef.current?.focus();
  }, [isStreaming, threadId]);

  const handleSwitchBranch = useCallback(async (forkMessageId: number, childId: number) => {
    if (!threadId || (isStreaming && streamingForThreadRef.current === threadId)) return;
    try {
      await api.switchBranch(threadId, forkMessageId, childId);
      await reloadThread();
    } catch {
      // ignore
    }
  }, [threadId, isStreaming, reloadThread]);

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
          await api.clearMessages(threadId);
          await api.clearSession(threadId);
          setMessages([]);
          setActiveToolCalls([]);
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
  }, [threadId, thread, messages, onNewThread, onOpenSearch, showFeedback, clearConfirm, handleSend]);

  const lastAssistantId = (() => {
    for (let i = messages.length - 1; i >= 0; i--) {
      if (messages[i]!.role === 'assistant') return messages[i]!.id;
    }
    return null;
  })();

  const isStreamingThisThread = isStreaming && streamingForThreadRef.current === threadId;

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
      <div ref={scrollContainerRef} className="flex-1 overflow-y-auto px-4 py-6">
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
                  {activeToolCalls.map((tc) => (
                    <ToolCallPanel
                      key={tc.id}
                      name={tc.name}
                      input={tc.input}
                      result={tc.result}
                      isError={tc.isError}
                      isStreaming={tc.result == null}
                    />
                  ))}
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
      {streamError && !isStreamingThisThread && (
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
              const contextWindow = 200000;
              const used = usageInfo.input_tokens!;
              const pct = Math.min((used / contextWindow) * 100, 100);
              const formatK = (n: number) => n >= 1000 ? `${(n / 1000).toFixed(1)}k` : String(n);
              return (
                <div className="flex items-center gap-1.5 flex-1 min-w-0">
                  <div className="flex-1 h-1 bg-zinc-200 rounded-full overflow-hidden min-w-[60px]">
                    <div
                      className={`h-full rounded-full transition-all ${pct > 80 ? 'bg-red-500' : pct > 50 ? 'bg-amber-500' : 'bg-emerald-500'}`}
                      style={{ width: `${pct}%` }}
                    />
                  </div>
                  <span>{formatK(used)} / {formatK(contextWindow)}</span>
                </div>
              );
            })()}
            {usageInfo.cost_usd != null && (
              <span>${usageInfo.cost_usd.toFixed(4)}</span>
            )}
          </div>
        )}
        <ChatInput ref={chatInputRef} onSend={handleSend} onSlashCommand={handleSlashCommand} queuedCount={queuedIds.size} planMode={planMode} onTogglePlanMode={() => setPlanMode((p) => !p)} />
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
