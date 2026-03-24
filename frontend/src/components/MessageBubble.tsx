import { useState, useRef, useEffect } from 'react';
import type { Message, Attachment, ForkPoint } from '../types';
import MarkdownContent from './MarkdownContent';
import ThinkingSection from './ThinkingSection';
import MessageActions from './MessageActions';
import BranchIndicator from './BranchIndicator';

interface Props {
  message: Message;
  isStreaming?: boolean;
  isLastAssistant?: boolean;
  isPending?: boolean;
  forkPoint?: ForkPoint;
  onEdit?: (messageId: number, content: string) => void;
  onRegenerate?: () => void;
  onBranch?: () => void;
  onSwitchBranch?: (childId: number) => void;
  onImageClick?: (attachment: Attachment, allImages: Attachment[]) => void;
  onRemoveQueued?: () => void;
  onOptionSelect?: (text: string) => void;
}

function formatFileSize(bytes: number) {
  if (bytes < 1024) return bytes + ' B';
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB';
  return (bytes / (1024 * 1024)).toFixed(1) + ' MB';
}

function formatTokens(n: number): string {
  return n.toLocaleString();
}

function TokenBadge({ promptTokens, completionTokens }: { promptTokens: number; completionTokens: number }) {
  const total = promptTokens + completionTokens;
  return (
    <span
      className="text-[11px] text-zinc-400 md:opacity-0 md:group-hover:opacity-100 transition-opacity duration-150"
      title={`Prompt: ${formatTokens(promptTokens)} | Completion: ${formatTokens(completionTokens)} | Total: ${formatTokens(total)}`}
    >
      {formatTokens(promptTokens)} → {formatTokens(completionTokens)}
    </span>
  );
}

function UserAvatar() {
  return (
    <div className="w-7 h-7 rounded-full bg-emerald-100 flex items-center justify-center flex-shrink-0">
      <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="currentColor" className="w-3.5 h-3.5 text-emerald-600">
        <path d="M10 8a3 3 0 100-6 3 3 0 000 6zM3.465 14.493a1.23 1.23 0 00.41 1.412A9.957 9.957 0 0010 18c2.31 0 4.438-.784 6.131-2.1.43-.333.604-.903.408-1.41a7.002 7.002 0 00-13.074.003z" />
      </svg>
    </div>
  );
}

function AssistantAvatar() {
  return (
    <img
      src="/avatar-assistant.jpg"
      alt="Pan Botka"
      className="w-7 h-7 rounded-full flex-shrink-0"
    />
  );
}

export default function MessageBubble({ message, isStreaming, isLastAssistant, isPending, forkPoint, onEdit, onRegenerate, onBranch, onSwitchBranch, onImageClick, onRemoveQueued, onOptionSelect }: Props) {
  const isUser = message.role === 'user';
  const [editing, setEditing] = useState(false);
  const [editContent, setEditContent] = useState(message.content);
  const textareaRef = useRef<HTMLTextAreaElement>(null);

  const attachments = message.attachments || [];
  const imageAttachments = attachments.filter((a) => a.mime_type.startsWith('image/'));
  const fileAttachments = attachments.filter((a) => !a.mime_type.startsWith('image/'));

  useEffect(() => {
    if (editing && textareaRef.current) {
      textareaRef.current.focus();
      textareaRef.current.style.height = 'auto';
      textareaRef.current.style.height = textareaRef.current.scrollHeight + 'px';
    }
  }, [editing]);

  const handleEditSubmit = () => {
    const trimmed = editContent.trim();
    if (trimmed && trimmed !== message.content && onEdit) {
      onEdit(message.id, trimmed);
    }
    setEditing(false);
  };

  const handleEditCancel = () => {
    setEditContent(message.content);
    setEditing(false);
  };

  const handleEditKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      handleEditSubmit();
    }
    if (e.key === 'Escape') {
      handleEditCancel();
    }
  };

  return (
    <div className={`group animate-message-in ${isUser ? 'flex justify-end' : 'flex justify-start'} mb-6`}>
      {!isUser && (
        <div className="mt-1 mr-3 hidden md:block">
          <AssistantAvatar />
        </div>
      )}

      <div className={`flex flex-col ${isUser ? 'items-end' : 'items-start'} max-w-[85%] md:max-w-[75%] min-w-0`}>
        <div
          className={`rounded-2xl px-4 py-3 break-words overflow-hidden ${
            isUser
              ? isPending ? 'bg-zinc-100 text-zinc-400' : 'bg-emerald-600 text-white'
              : 'text-zinc-900'
          }`}
        >
          {editing ? (
            <div className="flex flex-col gap-2">
              <textarea
                ref={textareaRef}
                value={editContent}
                onChange={(e) => {
                  setEditContent(e.target.value);
                  e.target.style.height = 'auto';
                  e.target.style.height = e.target.scrollHeight + 'px';
                }}
                onKeyDown={handleEditKeyDown}
                className="w-full bg-white text-zinc-900 text-sm rounded-lg px-3 py-2 resize-none outline-none focus:ring-1 focus:ring-emerald-500 border border-zinc-300 min-w-[200px]"
                rows={1}
              />
              <div className="flex justify-end gap-2">
                <button
                  onClick={handleEditCancel}
                  className="px-3 py-1 text-xs rounded-md bg-zinc-100 text-zinc-600 hover:bg-zinc-200 transition-colors"
                >
                  Cancel
                </button>
                <button
                  onClick={handleEditSubmit}
                  className="px-3 py-1 text-xs rounded-md bg-emerald-600 text-white hover:bg-emerald-700 transition-colors"
                >
                  Save & Submit
                </button>
              </div>
            </div>
          ) : isUser ? (
            <>
              {imageAttachments.length > 0 && (
                <div className="flex flex-wrap gap-2 mb-2">
                  {imageAttachments.map((att) => (
                    <button
                      key={att.id}
                      type="button"
                      onClick={() => onImageClick?.(att, imageAttachments)}
                      className="cursor-pointer rounded-lg overflow-hidden hover:opacity-80 transition-opacity"
                    >
                      <img
                        src={`/api/v1/files/${att.id}`}
                        alt={att.original_name}
                        className="max-w-[200px] max-h-[150px] rounded-lg object-cover"
                      />
                    </button>
                  ))}
                </div>
              )}
              {fileAttachments.length > 0 && (
                <div className="flex flex-col gap-1 mb-2">
                  {fileAttachments.map((att) => (
                    <a
                      key={att.id}
                      href={`/api/v1/files/${att.id}/download`}
                      className="flex items-center gap-2 text-xs text-white/70 hover:text-white transition-colors"
                      download
                    >
                      <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16" fill="currentColor" className="w-3.5 h-3.5 flex-shrink-0">
                        <path d="M3 3.5A1.5 1.5 0 014.5 2h6.879a1.5 1.5 0 011.06.44l.44.44v.001l.44.44A1.5 1.5 0 0113.5 4.378V12.5A1.5 1.5 0 0112 14H4.5A1.5 1.5 0 013 12.5v-9z" />
                      </svg>
                      <span className="truncate">{att.original_name}</span>
                      <span className="text-white/50 flex-shrink-0">({formatFileSize(att.size)})</span>
                    </a>
                  ))}
                </div>
              )}
              <p className="whitespace-pre-wrap text-[15px] leading-relaxed">{message.content}</p>
            </>
          ) : (
            <div className="text-[15px]" style={{ lineHeight: 1.7 }}>
              {message.thinking && (
                <ThinkingSection
                  content={message.thinking}
                  durationMs={message.thinking_duration_ms}
                  isStreaming={isStreaming && !message.content}
                />
              )}
              {message.content && <MarkdownContent content={message.content} onOptionSelect={onOptionSelect} />}
              {isStreaming && (
                <span className="inline-block w-1.5 h-4 bg-emerald-500/70 ml-0.5 rounded-sm" style={{ animation: 'pulse-glow 1s ease-in-out infinite' }} />
              )}
            </div>
          )}
        </div>
        {isPending && (
          <div className="mt-1.5 flex items-center gap-2 self-end">
            <span className="flex items-center gap-1 text-xs text-zinc-400">
              <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16" fill="currentColor" className="w-3 h-3">
                <path fillRule="evenodd" d="M1 8a7 7 0 1 1 14 0A7 7 0 0 1 1 8Zm7.75-4.25a.75.75 0 0 0-1.5 0V8c0 .414.336.75.75.75h3.25a.75.75 0 0 0 0-1.5h-2.5v-3.5Z" clipRule="evenodd" />
              </svg>
              Queued
            </span>
            {onRemoveQueued && (
              <button
                type="button"
                onClick={onRemoveQueued}
                className="text-zinc-400 hover:text-red-500 transition-colors cursor-pointer"
                title="Remove from queue"
              >
                <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16" fill="currentColor" className="w-3.5 h-3.5">
                  <path d="M5.28 4.22a.75.75 0 00-1.06 1.06L6.94 8l-2.72 2.72a.75.75 0 101.06 1.06L8 9.06l2.72 2.72a.75.75 0 101.06-1.06L9.06 8l2.72-2.72a.75.75 0 00-1.06-1.06L8 6.94 5.28 4.22z" />
                </svg>
              </button>
            )}
          </div>
        )}
        {!isStreaming && !editing && !isPending && (
          <div className={`mt-1.5 flex items-center gap-3 ${isUser ? 'self-end' : 'self-start md:ml-0'}`}>
            <div className="opacity-100 md:opacity-0 md:group-hover:opacity-100 focus-within:opacity-100 transition-opacity duration-150">
              <MessageActions
                role={message.role}
                content={message.content}
                isLastAssistant={isLastAssistant ?? false}
                onEdit={onEdit ? () => setEditing(true) : undefined}
                onRegenerate={onRegenerate}
                onBranch={onBranch}
              />
            </div>
            {forkPoint && onSwitchBranch && (
              <BranchIndicator forkPoint={forkPoint} onSwitch={onSwitchBranch} />
            )}
            {!isUser && message.prompt_tokens != null && message.completion_tokens != null && (
              <TokenBadge promptTokens={message.prompt_tokens} completionTokens={message.completion_tokens} />
            )}
          </div>
        )}
      </div>

      {isUser && (
        <div className="mt-1 ml-3 hidden md:block">
          <UserAvatar />
        </div>
      )}
    </div>
  );
}
