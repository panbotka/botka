import { useState, useRef, useEffect } from 'react';
import type { Message, Attachment, ForkPoint } from '../types';
import { formatTime, formatDateTime } from '../utils/dateFormat';
import MarkdownContent from './MarkdownContent';
import ThinkingSection from './ThinkingSection';
import MessageActions from './MessageActions';
import BranchIndicator from './BranchIndicator';
import ToolCallPanel from './ToolCallPanel';
import { Wrench, ChevronDown, EyeOff } from 'lucide-react';

interface Props {
  message: Message;
  isStreaming?: boolean;
  isLastAssistant?: boolean;
  isPending?: boolean;
  forkPoint?: ForkPoint;
  onEdit?: (messageId: number, content: string) => void;
  onRegenerate?: () => void;
  onBranch?: () => void;
  onHide?: () => void;
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

function formatTimestamp(dateStr: string): string {
  const date = new Date(dateStr);
  const now = new Date();

  const isToday = date.getFullYear() === now.getFullYear()
    && date.getMonth() === now.getMonth()
    && date.getDate() === now.getDate();

  if (isToday) return formatTime(date);
  return formatDateTime(date);
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

function PdfIcon({ className }: { className?: string }) {
  return (
    <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.5} className={className}>
      <path strokeLinecap="round" strokeLinejoin="round" d="M19.5 14.25v-2.625a3.375 3.375 0 0 0-3.375-3.375h-1.5A1.125 1.125 0 0 1 13.5 7.125v-1.5a3.375 3.375 0 0 0-3.375-3.375H8.25m2.25 0H5.625c-.621 0-1.125.504-1.125 1.125v17.25c0 .621.504 1.125 1.125 1.125h12.75c.621 0 1.125-.504 1.125-1.125V11.25a9 9 0 0 0-9-9Z" />
    </svg>
  );
}

function TextIcon({ className }: { className?: string }) {
  return (
    <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={1.5} className={className}>
      <path strokeLinecap="round" strokeLinejoin="round" d="M19.5 14.25v-2.625a3.375 3.375 0 0 0-3.375-3.375h-1.5A1.125 1.125 0 0 1 13.5 7.125v-1.5a3.375 3.375 0 0 0-3.375-3.375H8.25m0 12.75h7.5m-7.5 3H12M10.5 2.25H5.625c-.621 0-1.125.504-1.125 1.125v17.25c0 .621.504 1.125 1.125 1.125h12.75c.621 0 1.125-.504 1.125-1.125V11.25a9 9 0 0 0-9-9Z" />
    </svg>
  );
}

function FileCard({ attachment, icon, action, variant }: { attachment: Attachment; icon: React.ReactNode; action?: React.ReactNode; variant: 'user' | 'assistant' }) {
  const isUser = variant === 'user';
  return (
    <div className={`flex items-center gap-2.5 rounded-lg px-3 py-2 text-sm ${isUser ? 'bg-white/10' : 'bg-zinc-100 border border-zinc-200'}`}>
      <div className={`flex-shrink-0 ${isUser ? 'text-white/70' : 'text-zinc-500'}`}>
        {icon}
      </div>
      <div className="min-w-0 flex-1">
        <div className={`truncate font-medium ${isUser ? 'text-white' : 'text-zinc-700'}`}>{attachment.original_name}</div>
        <div className={`text-xs ${isUser ? 'text-white/50' : 'text-zinc-400'}`}>{formatFileSize(attachment.size)}</div>
      </div>
      {action}
    </div>
  );
}

function AttachmentPreviews({ imageAttachments, pdfAttachments, textAttachments, otherAttachments, onImageClick, variant }: {
  imageAttachments: Attachment[];
  pdfAttachments: Attachment[];
  textAttachments: Attachment[];
  otherAttachments: Attachment[];
  onImageClick?: (attachment: Attachment, allImages: Attachment[]) => void;
  variant: 'user' | 'assistant';
}) {
  const hasAny = imageAttachments.length + pdfAttachments.length + textAttachments.length + otherAttachments.length > 0;
  if (!hasAny) return null;

  const isUser = variant === 'user';
  const linkClass = isUser ? 'text-white/70 hover:text-white' : 'text-emerald-600 hover:text-emerald-700';

  return (
    <div className="mt-2 flex flex-wrap gap-2">
      {imageAttachments.map((att) => (
        <button
          key={att.id}
          type="button"
          onClick={() => onImageClick?.(att, imageAttachments)}
          className="cursor-pointer rounded-lg overflow-hidden hover:opacity-80 transition-opacity border border-white/20"
        >
          <img
            src={att.url}
            alt={att.original_name}
            className="max-w-[300px] max-h-[200px] rounded-lg object-cover"
          />
        </button>
      ))}
      {pdfAttachments.map((att) => (
        <FileCard
          key={att.id}
          attachment={att}
          variant={variant}
          icon={<PdfIcon className="w-5 h-5" />}
          action={
            <div className="flex gap-2 flex-shrink-0">
              <a
                href={att.url}
                target="_blank"
                rel="noopener noreferrer"
                className={`text-xs font-medium transition-colors ${linkClass}`}
                onClick={(e) => e.stopPropagation()}
              >
                Open
              </a>
              <a
                href={att.url}
                download={att.original_name}
                className={`text-xs font-medium transition-colors ${linkClass}`}
                onClick={(e) => e.stopPropagation()}
              >
                Download
              </a>
            </div>
          }
        />
      ))}
      {textAttachments.map((att) => (
        <FileCard
          key={att.id}
          attachment={att}
          variant={variant}
          icon={<TextIcon className="w-5 h-5" />}
          action={
            <div className="flex gap-2 flex-shrink-0">
              <a
                href={att.url}
                target="_blank"
                rel="noopener noreferrer"
                className={`text-xs font-medium transition-colors ${linkClass}`}
                onClick={(e) => e.stopPropagation()}
              >
                Open
              </a>
              <a
                href={att.url}
                download={att.original_name}
                className={`text-xs font-medium transition-colors ${linkClass}`}
                onClick={(e) => e.stopPropagation()}
              >
                Download
              </a>
            </div>
          }
        />
      ))}
      {otherAttachments.map((att) => (
        <FileCard
          key={att.id}
          attachment={att}
          variant={variant}
          icon={<PdfIcon className="w-5 h-5" />}
          action={
            <a
              href={att.url}
              download={att.original_name}
              className={`text-xs font-medium transition-colors flex-shrink-0 ${linkClass}`}
              onClick={(e) => e.stopPropagation()}
            >
              Download
            </a>
          }
        />
      ))}
    </div>
  );
}

export default function MessageBubble({ message, isStreaming, isLastAssistant, isPending, forkPoint, onEdit, onRegenerate, onBranch, onHide, onSwitchBranch, onImageClick, onRemoveQueued, onOptionSelect }: Props) {
  const isUser = message.role === 'user';
  const [editing, setEditing] = useState(false);
  const [editContent, setEditContent] = useState(message.content);
  const [toolCallsExpanded, setToolCallsExpanded] = useState(false);
  const toolCalls = message.tool_calls;
  const textareaRef = useRef<HTMLTextAreaElement>(null);

  if (message.hidden) {
    return (
      <div className={`group flex ${isUser ? 'justify-end' : 'justify-start'} mb-3`}>
        <button
          type="button"
          onClick={onHide}
          className="flex items-center gap-1.5 px-3 py-1.5 rounded-full text-xs text-zinc-400 hover:text-zinc-600 bg-zinc-50 hover:bg-zinc-100 border border-zinc-200 transition-colors cursor-pointer"
          title="Click to unhide"
        >
          <EyeOff size={12} />
          <span>Message hidden</span>
        </button>
      </div>
    );
  }

  const attachments = message.attachments || [];
  const imageAttachments = attachments.filter((a) => a.mime_type.startsWith('image/'));
  const pdfAttachments = attachments.filter((a) => a.mime_type === 'application/pdf');
  const textAttachments = attachments.filter((a) => a.mime_type.startsWith('text/'));
  const otherAttachments = attachments.filter((a) => !a.mime_type.startsWith('image/') && a.mime_type !== 'application/pdf' && !a.mime_type.startsWith('text/'));

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
                className="w-full bg-zinc-50 text-zinc-900 text-sm rounded-lg px-3 py-2 resize-none outline-none focus:ring-1 focus:ring-emerald-500 border border-zinc-300 min-w-[200px]"
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
              <p className="whitespace-pre-wrap text-[15px] leading-relaxed">{message.content}</p>
              <AttachmentPreviews
                imageAttachments={imageAttachments}
                pdfAttachments={pdfAttachments}
                textAttachments={textAttachments}
                otherAttachments={otherAttachments}
                onImageClick={onImageClick}
                variant="user"
              />
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
              {attachments.length > 0 && (
                <AttachmentPreviews
                  imageAttachments={imageAttachments}
                  pdfAttachments={pdfAttachments}
                  textAttachments={textAttachments}
                  otherAttachments={otherAttachments}
                  onImageClick={onImageClick}
                  variant="assistant"
                />
              )}
              {!isStreaming && toolCalls && toolCalls.length > 0 && (
                <div className="mt-3">
                  <button
                    type="button"
                    onClick={() => setToolCallsExpanded(!toolCallsExpanded)}
                    className="flex items-center gap-1.5 text-xs text-zinc-400 hover:text-zinc-600 transition-colors cursor-pointer"
                  >
                    <Wrench size={12} />
                    <span>{toolCalls.length} tool {toolCalls.length === 1 ? 'call' : 'calls'}</span>
                    <ChevronDown
                      size={12}
                      className={`transition-transform duration-200 ${toolCallsExpanded ? 'rotate-180' : ''}`}
                    />
                  </button>
                  <div
                    className={`grid transition-[grid-template-rows] duration-200 ease-in-out ${
                      toolCallsExpanded ? 'grid-rows-[1fr]' : 'grid-rows-[0fr]'
                    }`}
                  >
                    <div className="overflow-hidden">
                      <div className="mt-2">
                        {toolCalls.map((tc, i) => (
                          <ToolCallPanel
                            key={i}
                            name={tc.name}
                            input={tc.input}
                          />
                        ))}
                      </div>
                    </div>
                  </div>
                </div>
              )}
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
                isHidden={message.hidden}
                onEdit={onEdit ? () => setEditing(true) : undefined}
                onRegenerate={onRegenerate}
                onBranch={onBranch}
                onHide={onHide}
              />
            </div>
            {forkPoint && onSwitchBranch && (
              <BranchIndicator forkPoint={forkPoint} onSwitch={onSwitchBranch} />
            )}
            {!isUser && message.prompt_tokens != null && message.completion_tokens != null && (
              <TokenBadge promptTokens={message.prompt_tokens} completionTokens={message.completion_tokens} />
            )}
            {message.created_at && (
              <span
                className="text-[11px] text-zinc-400 md:opacity-0 md:group-hover:opacity-100 transition-opacity duration-150"
                title={new Date(message.created_at).toLocaleString()}
              >
                {formatTimestamp(message.created_at)}
              </span>
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
