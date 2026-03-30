import { createContext, useContext, useEffect, useRef, useReducer, type ReactNode } from 'react';
import type { Attachment, Message } from '../types';
import type { StreamChunk } from '../api/client';

// ============= Types =============

export interface ActiveToolCall {
  id: string;
  name: string;
  input: Record<string, unknown>;
  result?: string;
  isError?: boolean;
}

export interface SSESessionState {
  content: string;
  thinking: string;
  thinkingDurationMs: number | null;
  toolCalls: ActiveToolCall[];
  usageInfo: { cost_usd?: number; input_tokens?: number; output_tokens?: number } | null;
  memorySuggestions: string[];
  retryInfo: { attempt: number; max_attempts: number } | null;
  reconnecting: { attempt: number; maxAttempts: number } | null;
  streamError: string | null;
  streamErrorRaw: string | null;
  isStreaming: boolean;
  isComplete: boolean;
  completedMessage: Message | null;
  attachments: Attachment[];
  titleUpdate: { threadId: number; title: string } | null;
  gotResponse: boolean;
}

// ============= StreamError =============

class StreamError extends Error {
  connectionLost: boolean;
  raw: string | undefined;
  constructor(message: string, connectionLost: boolean, raw?: string) {
    super(message);
    this.connectionLost = connectionLost;
    this.raw = raw;
  }
}

// ============= Constants =============

const RETRY_DELAYS = [1000, 2000, 4000];
const CLEANUP_DELAY_MS = 5 * 60 * 1000; // 5 minutes

// ============= Session (internal) =============

interface Session {
  threadId: number;
  controller: AbortController;
  state: SSESessionState;
  subscribers: Set<() => void>;
  cleanupTimer: ReturnType<typeof setTimeout> | null;
}

function createSessionState(): SSESessionState {
  return {
    content: '',
    thinking: '',
    thinkingDurationMs: null,
    toolCalls: [],
    usageInfo: null,
    memorySuggestions: [],
    retryInfo: null,
    reconnecting: null,
    streamError: null,
    streamErrorRaw: null,
    isStreaming: true,
    isComplete: false,
    completedMessage: null,
    attachments: [],
    titleUpdate: null,
    gotResponse: false,
  };
}

// ============= SSESessionManager =============

export class SSESessionManager {
  private sessions = new Map<number, Session>();
  private globalSubscribers = new Set<() => void>();

  /** Get the current session state for a thread, or null if no session exists. */
  getSessionState(threadId: number): SSESessionState | null {
    return this.sessions.get(threadId)?.state ?? null;
  }

  /** Check if a thread has an active (still streaming) session. */
  hasActiveSession(threadId: number): boolean {
    const session = this.sessions.get(threadId);
    return !!session && session.state.isStreaming;
  }

  /** Get all thread IDs that currently have an active streaming session. */
  getStreamingThreadIds(): Set<number> {
    const ids = new Set<number>();
    for (const [threadId, session] of this.sessions) {
      if (session.state.isStreaming) ids.add(threadId);
    }
    return ids;
  }

  /** Subscribe to global session lifecycle changes (start/stop/abort). */
  onSessionChange(callback: () => void): () => void {
    this.globalSubscribers.add(callback);
    return () => this.globalSubscribers.delete(callback);
  }

  private notifyGlobal(): void {
    for (const cb of this.globalSubscribers) cb();
  }

  /**
   * Start a new session for a thread. Aborts any existing session.
   * Returns the AbortSignal for the new session.
   */
  startSession(threadId: number): AbortSignal {
    this.abortSession(threadId);

    const session: Session = {
      threadId,
      controller: new AbortController(),
      state: createSessionState(),
      subscribers: new Set(),
      cleanupTimer: null,
    };

    this.sessions.set(threadId, session);
    this.notifyGlobal();
    return session.controller.signal;
  }

  /**
   * Subscribe to session state changes for a thread.
   * Cancels cleanup timer when subscribing; starts timer on last unsubscribe.
   * Returns an unsubscribe function.
   */
  subscribe(threadId: number, callback: () => void): () => void {
    const session = this.sessions.get(threadId);
    if (!session) return () => {};

    session.subscribers.add(callback);

    // A subscriber is present — cancel any pending cleanup
    if (session.cleanupTimer) {
      clearTimeout(session.cleanupTimer);
      session.cleanupTimer = null;
    }

    return () => {
      session.subscribers.delete(callback);

      // Only manage cleanup if this session is still registered
      if (this.sessions.get(threadId) !== session) return;

      if (session.subscribers.size === 0) {
        if (session.state.isStreaming) {
          // Keep alive for 5 minutes then abort
          session.cleanupTimer = setTimeout(() => {
            this.abortSession(threadId);
          }, CLEANUP_DELAY_MS);
        } else {
          // Completed sessions: clean up after a short delay
          session.cleanupTimer = setTimeout(() => {
            if (this.sessions.get(threadId) === session) {
              this.sessions.delete(threadId);
            }
          }, 30_000);
        }
      }
    };
  }

  /** Abort a session immediately and remove it. */
  abortSession(threadId: number): void {
    const session = this.sessions.get(threadId);
    if (!session) return;

    session.controller.abort();
    if (session.cleanupTimer) clearTimeout(session.cleanupTimer);
    this.sessions.delete(threadId);
    this.notifyGlobal();
  }

  /** Remove a completed session from the manager. */
  clearSession(threadId: number): void {
    const session = this.sessions.get(threadId);
    if (!session) return;
    if (session.cleanupTimer) clearTimeout(session.cleanupTimer);
    this.sessions.delete(threadId);
    this.notifyGlobal();
  }

  /**
   * Run a stream to completion, consuming chunks and updating session state.
   * Handles retry logic for connection loss using retryStreamFn.
   * Resolves when the stream is done (success, error, or abort).
   */
  async runStream(
    threadId: number,
    createStream: (signal: AbortSignal) => AsyncGenerator<StreamChunk>,
    options?: {
      retryStreamFn?: (signal: AbortSignal) => AsyncGenerator<StreamChunk>;
    },
  ): Promise<void> {
    const session = this.sessions.get(threadId);
    if (!session) return;

    try {
      await this.consumeStream(session, createStream(session.controller.signal));
    } catch (err) {
      if (err instanceof Error && err.name === 'AbortError') return;

      if (err instanceof StreamError && err.connectionLost && options?.retryStreamFn) {
        let succeeded = false;

        for (let i = 0; i < RETRY_DELAYS.length; i++) {
          session.state.content = '';
          session.state.thinking = '';
          session.state.thinkingDurationMs = null;
          session.state.reconnecting = { attempt: i + 1, maxAttempts: RETRY_DELAYS.length };
          this.notify(session);

          await new Promise(r => setTimeout(r, RETRY_DELAYS[i]!));
          if (session.controller.signal.aborted) return;

          try {
            await this.consumeStream(session, options.retryStreamFn(session.controller.signal));
            succeeded = true;
            break;
          } catch (retryErr) {
            if (retryErr instanceof Error && retryErr.name === 'AbortError') return;
            if (!(retryErr instanceof StreamError && retryErr.connectionLost)) {
              session.state.streamError = retryErr instanceof Error ? retryErr.message : 'Unknown error';
              session.state.streamErrorRaw = retryErr instanceof StreamError ? retryErr.raw ?? null : null;
              this.notify(session);
              succeeded = true;
              break;
            }
          }
        }

        if (!succeeded) {
          session.state.streamError = 'Server unavailable';
          session.state.streamErrorRaw = null;
          session.state.reconnecting = null;
          this.notify(session);
        }
      } else {
        session.state.streamError = err instanceof Error ? err.message : 'Unknown error';
        session.state.streamErrorRaw = err instanceof StreamError ? err.raw ?? null : null;
        this.notify(session);
      }
    } finally {
      // Only update if this session hasn't been replaced
      if (this.sessions.get(threadId) === session) {
        session.state.isStreaming = false;
        session.state.isComplete = true;
        session.state.reconnecting = null;
        session.state.retryInfo = null;
        this.notify(session);
        this.notifyGlobal();

        // Start cleanup timer if no subscribers
        if (session.subscribers.size === 0) {
          session.cleanupTimer = setTimeout(() => {
            if (this.sessions.get(threadId) === session) {
              this.sessions.delete(threadId);
            }
          }, CLEANUP_DELAY_MS);
        }
      }
    }
  }

  /** Consume an SSE stream, accumulating state in the session. */
  private async consumeStream(session: Session, stream: AsyncGenerator<StreamChunk>): Promise<void> {
    let fullContent = '';
    let fullThinking = '';
    let promptTokens: number | undefined;
    let completionTokens: number | undefined;

    for await (const chunk of stream) {
      if (chunk.done) break;

      if (chunk.retry) {
        session.state.retryInfo = { attempt: chunk.retry.attempt, max_attempts: chunk.retry.max_attempts };
        this.notify(session);
        continue;
      }

      if (chunk.error) {
        session.state.retryInfo = null;
        this.notify(session);
        throw new StreamError(chunk.error, chunk.connectionLost ?? false, chunk.error_raw);
      }

      if (chunk.title) {
        session.state.titleUpdate = { threadId: session.threadId, title: chunk.title };
        this.notify(session);
        continue;
      }

      if (chunk.usage) {
        promptTokens = chunk.usage.prompt_tokens;
        completionTokens = chunk.usage.completion_tokens;
        session.state.usageInfo = {
          cost_usd: chunk.usage.cost_usd,
          input_tokens: chunk.usage.input_tokens,
          output_tokens: chunk.usage.output_tokens,
        };
        this.notify(session);
        continue;
      }

      if (chunk.memory_suggestion) {
        session.state.memorySuggestions = [...session.state.memorySuggestions, chunk.memory_suggestion];
        this.notify(session);
        continue;
      }

      if (chunk.tool_use) {
        session.state.toolCalls = [...session.state.toolCalls, {
          id: chunk.tool_use.id,
          name: chunk.tool_use.name,
          input: chunk.tool_use.input,
        }];
        this.notify(session);
        continue;
      }

      if (chunk.tool_result) {
        const tr = chunk.tool_result;
        session.state.toolCalls = session.state.toolCalls.map(tc =>
          tc.id === tr.tool_use_id ? { ...tc, result: tr.content, isError: tr.is_error } : tc,
        );
        this.notify(session);
        continue;
      }

      if (chunk.attachments) {
        session.state.attachments = [...session.state.attachments, ...chunk.attachments];
        this.notify(session);
        continue;
      }

      if (chunk.content || chunk.thinking) {
        session.state.retryInfo = null;
      }

      let needsNotify = false;

      if (chunk.thinking) {
        fullThinking += chunk.thinking;
        session.state.thinking = fullThinking;
        needsNotify = true;
      }

      if (chunk.thinking_done) {
        session.state.thinkingDurationMs = chunk.thinking_done.duration_ms;
        needsNotify = true;
      }

      if (chunk.content) {
        fullContent += chunk.content;
        session.state.content = fullContent.replace(/<memory>.*?<\/memory>/gs, '').trim();
        needsNotify = true;
      }

      if (needsNotify) this.notify(session);
    }

    // Build the completed message
    session.state.retryInfo = null;
    const cleanContent = fullContent.replace(/<memory>.*?<\/memory>/gs, '').trim();
    if (cleanContent) {
      const persistedToolCalls = session.state.toolCalls.length > 0
        ? session.state.toolCalls.map(tc => ({ name: tc.name, input: tc.input }))
        : undefined;
      session.state.completedMessage = {
        id: Date.now() + 1,
        thread_id: session.threadId,
        role: 'assistant',
        content: cleanContent,
        thinking: fullThinking || undefined,
        thinking_duration_ms: session.state.thinkingDurationMs ?? undefined,
        prompt_tokens: promptTokens,
        completion_tokens: completionTokens,
        tool_calls: persistedToolCalls,
        attachments: session.state.attachments.length > 0 ? session.state.attachments : undefined,
        created_at: new Date().toISOString(),
      };
      session.state.gotResponse = true;
    }
  }

  private notify(session: Session): void {
    for (const cb of session.subscribers) {
      cb();
    }
  }
}

// ============= React Context =============

const SSEContext = createContext<SSESessionManager | null>(null);

export function SSEProvider({ children }: { children: ReactNode }) {
  const managerRef = useRef(new SSESessionManager());

  return (
    <SSEContext.Provider value={managerRef.current}>
      {children}
    </SSEContext.Provider>
  );
}

export function useSSEManager(): SSESessionManager {
  const manager = useContext(SSEContext);
  if (!manager) throw new Error('useSSEManager must be used within SSEProvider');
  return manager;
}

/**
 * Subscribe to an SSE session for a thread.
 * Returns the current session state, or null if no active/completed session exists.
 * Re-renders the component on every state change (each streamed chunk).
 */
export function useSSESession(threadId: number | null): SSESessionState | null {
  const manager = useSSEManager();
  const [, forceUpdate] = useReducer((x: number) => x + 1, 0);

  useEffect(() => {
    if (!threadId) return;
    return manager.subscribe(threadId, forceUpdate);
  }, [threadId, manager]);

  if (!threadId) return null;
  return manager.getSessionState(threadId);
}

/**
 * Subscribe to global session lifecycle changes (start/stop/abort).
 * Returns the set of thread IDs that currently have active streaming sessions.
 * Only re-renders on session start/stop/abort — not on every streamed chunk.
 */
export function useStreamingThreadIds(): Set<number> {
  const manager = useSSEManager();
  const [, forceUpdate] = useReducer((x: number) => x + 1, 0);

  useEffect(() => {
    return manager.onSessionChange(forceUpdate);
  }, [manager]);

  return manager.getStreamingThreadIds();
}
