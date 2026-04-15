import { useState, useRef, useEffect } from 'react';
import { MessageCircleQuestion, Send, Check } from 'lucide-react';
import { submitToolResult } from '../api/client';
import type { ActiveToolCall } from '../context/SSEContext';

interface AskUserQuestion {
  question: string;
  header?: string;
  multiSelect?: boolean;
  options?: Array<{ label: string; description?: string }>;
}

interface Props {
  toolCall: ActiveToolCall;
  threadId: number;
}

function parseQuestions(input: Record<string, unknown>): AskUserQuestion[] {
  // Handle both { questions: [...] } and { question: "..." } formats
  if (Array.isArray(input.questions)) {
    return input.questions as AskUserQuestion[];
  }
  if (typeof input.question === 'string') {
    return [{ question: input.question }];
  }
  return [];
}

export default function AskUserPanel({ toolCall, threadId }: Props) {
  const questions = parseQuestions(toolCall.input);
  const [answers, setAnswers] = useState<Map<number, string | string[]>>(new Map());
  const [freeText, setFreeText] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [submitted, setSubmitted] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const inputRef = useRef<HTMLTextAreaElement>(null);

  const isAnswered = !!toolCall.result;

  useEffect(() => {
    if (!isAnswered && !submitted && inputRef.current) {
      inputRef.current.focus();
    }
  }, [isAnswered, submitted]);

  const toggleOption = (qIndex: number, label: string, multiSelect: boolean) => {
    setAnswers(prev => {
      const next = new Map(prev);
      if (multiSelect) {
        const current = (next.get(qIndex) as string[]) || [];
        if (current.includes(label)) {
          next.set(qIndex, current.filter(l => l !== label));
        } else {
          next.set(qIndex, [...current, label]);
        }
      } else {
        next.set(qIndex, label);
      }
      return next;
    });
  };

  const buildResultContent = (): string => {
    if (questions.length === 0) return freeText;

    const parts: string[] = [];
    for (let i = 0; i < questions.length; i++) {
      const q = questions[i]!;
      if (q.options && q.options.length > 0) {
        const answer = answers.get(i);
        if (Array.isArray(answer)) {
          parts.push(answer.join(', '));
        } else if (typeof answer === 'string') {
          parts.push(answer);
        }
      } else {
        parts.push(freeText);
      }
    }
    return parts.join('\n');
  };

  const canSubmit = (): boolean => {
    if (questions.length === 0) return freeText.trim().length > 0;

    for (let i = 0; i < questions.length; i++) {
      const q = questions[i]!;
      if (q.options && q.options.length > 0) {
        const answer = answers.get(i);
        if (!answer || (Array.isArray(answer) && answer.length === 0)) return false;
      } else {
        if (!freeText.trim()) return false;
      }
    }
    return true;
  };

  const handleSubmit = async () => {
    if (!canSubmit() || submitting) return;
    setSubmitting(true);
    setError(null);
    try {
      await submitToolResult(threadId, toolCall.id, buildResultContent());
      setSubmitted(true);
    } catch (err) {
      const msg = err instanceof Error ? err.message : 'Failed to submit answer';
      setError(msg.includes('session') || msg.includes('Session')
        ? 'Session expired. Please resend your message.'
        : `Failed to submit: ${msg}`);
    } finally {
      setSubmitting(false);
    }
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      handleSubmit();
    }
  };

  // Already answered (via SSE tool_result or local submit)
  if (isAnswered || submitted) {
    const displayAnswer = toolCall.result || buildResultContent();
    return (
      <div className="my-3 rounded-xl border border-emerald-200 bg-emerald-50/50 p-4 max-w-3xl">
        <div className="flex items-center gap-2 mb-2">
          <Check size={18} className="text-emerald-600" />
          <span className="font-medium text-emerald-700 text-sm">Answered</span>
        </div>
        {questions.map((q, i) => (
          <div key={i} className="mb-1">
            <div className="text-sm text-zinc-600">{q.question}</div>
          </div>
        ))}
        <div className="mt-2 text-sm text-zinc-700 bg-white/60 rounded-lg px-3 py-2">
          {displayAnswer}
        </div>
      </div>
    );
  }

  return (
    <div className="my-3 rounded-xl border-2 border-blue-300 bg-gradient-to-b from-blue-50 to-white p-4 max-w-3xl shadow-sm">
      <div className="flex items-center gap-2 mb-3">
        <MessageCircleQuestion size={20} className="text-blue-600" />
        <span className="font-semibold text-blue-700 text-sm">Claude is asking a question</span>
      </div>

      {questions.map((q, qIndex) => (
        <div key={qIndex} className={qIndex > 0 ? 'mt-4 pt-4 border-t border-blue-100' : ''}>
          {q.header && (
            <div className="text-xs font-semibold text-blue-500 uppercase tracking-wide mb-1">
              {q.header}
            </div>
          )}
          <div className="text-sm text-zinc-800 font-medium mb-3">{q.question}</div>

          {q.options && q.options.length > 0 ? (
            <div className="space-y-2">
              {q.options.map((opt) => {
                const currentAnswer = answers.get(qIndex);
                const isSelected = q.multiSelect
                  ? Array.isArray(currentAnswer) && currentAnswer.includes(opt.label)
                  : currentAnswer === opt.label;

                return (
                  <button
                    key={opt.label}
                    onClick={() => toggleOption(qIndex, opt.label, !!q.multiSelect)}
                    disabled={submitting}
                    className={`w-full text-left px-3 py-2.5 rounded-lg border-2 transition-all text-sm cursor-pointer ${
                      isSelected
                        ? 'border-blue-500 bg-blue-50 text-blue-800'
                        : 'border-zinc-200 bg-white text-zinc-700 hover:border-blue-300 hover:bg-blue-50/50'
                    } ${submitting ? 'opacity-50 cursor-not-allowed' : ''}`}
                  >
                    <div className="font-medium">{opt.label}</div>
                    {opt.description && (
                      <div className="text-xs text-zinc-500 mt-0.5">{opt.description}</div>
                    )}
                  </button>
                );
              })}
            </div>
          ) : (
            <textarea
              ref={inputRef}
              value={freeText}
              onChange={(e) => setFreeText(e.target.value)}
              onKeyDown={handleKeyDown}
              disabled={submitting}
              placeholder="Type your answer..."
              rows={2}
              className="w-full px-3 py-2 rounded-lg border-2 border-zinc-200 bg-white text-sm text-zinc-800 placeholder-zinc-400 focus:border-blue-400 focus:outline-none resize-y disabled:opacity-50"
            />
          )}
        </div>
      ))}

      {error && (
        <div className="mt-2 px-3 py-2 rounded-lg bg-red-50 border border-red-200 text-sm text-red-700">
          {error}
        </div>
      )}

      <div className="flex justify-end mt-3">
        <button
          onClick={handleSubmit}
          disabled={!canSubmit() || submitting}
          className={`flex items-center gap-1.5 px-4 py-2 rounded-lg text-sm font-medium transition-colors cursor-pointer ${
            canSubmit() && !submitting
              ? 'bg-blue-600 text-white hover:bg-blue-700'
              : 'bg-zinc-200 text-zinc-400 cursor-not-allowed'
          }`}
        >
          <Send size={14} />
          {submitting ? 'Sending...' : error ? 'Retry' : 'Submit'}
        </button>
      </div>
    </div>
  );
}
