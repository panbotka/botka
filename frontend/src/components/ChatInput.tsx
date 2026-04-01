import { useState, useRef, useCallback, useEffect, useImperativeHandle, forwardRef, type FormEvent, type KeyboardEvent } from 'react';
import { useAutoResize } from '../hooks/useAutoResize';
import { useInputHistory } from '../hooks/useInputHistory';
import { useVoiceInput } from '../hooks/useVoiceInput';
import { useSettings } from '../context/SettingsContext';
import SlashCommandMenu, { getFilteredCommands } from './SlashCommandMenu';

// Module-level draft storage — survives component remounts but not page reloads
const draftsMap = new Map<number, string>();

export function clearDraft(threadId: number): void {
  draftsMap.delete(threadId);
}

export const MAX_FILE_SIZE = 10 * 1024 * 1024; // 10 MB
const ALLOWED_MIME_TYPES = [
  'image/png', 'image/jpeg', 'image/gif', 'image/webp',
  'application/pdf', 'text/plain', 'text/markdown', 'text/calendar',
  'application/zip', 'application/x-zip-compressed',
];
const ALLOWED_EXTENSIONS = ['.png', '.jpg', '.jpeg', '.gif', '.webp', '.pdf', '.txt', '.md', '.ics', '.zip'];

export function isAllowedFile(file: File): boolean {
  if (file.size > MAX_FILE_SIZE) return false;
  if (ALLOWED_MIME_TYPES.includes(file.type)) return true;
  const ext = '.' + file.name.split('.').pop()?.toLowerCase();
  return ALLOWED_EXTENSIONS.includes(ext);
}

export function getFileExtension(file: File): string {
  return '.' + (file.name.split('.').pop()?.toLowerCase() || '');
}

interface Props {
  threadId?: number | null;
  onSend: (content: string, files?: File[]) => void;
  onSlashCommand: (command: string, args: string) => void;
  queuedCount?: number;
  planMode?: boolean;
  onTogglePlanMode?: () => void;
  isStreaming?: boolean;
  onStop?: () => void;
}

export interface ChatInputHandle {
  addFiles: (files: FileList | File[]) => void;
  focus: () => void;
}

const ChatInput = forwardRef<ChatInputHandle, Props>(function ChatInput({ threadId, onSend, onSlashCommand, queuedCount = 0, planMode, onTogglePlanMode, isStreaming, onStop }, ref) {
  const { settings } = useSettings();
  const [value, setValue] = useState(() => (threadId ? draftsMap.get(threadId) ?? '' : ''));
  const [selectedIndex, setSelectedIndex] = useState(0);
  const [files, setFiles] = useState<File[]>([]);

  // Save draft on unmount (threadId change causes remount via key=)
  const valueRef = useRef(value);
  valueRef.current = value;
  useEffect(() => {
    const tid = threadId;
    return () => {
      if (tid) {
        const draft = valueRef.current.trim();
        if (draft) {
          draftsMap.set(tid, valueRef.current);
        } else {
          draftsMap.delete(tid);
        }
      }
    };
  }, [threadId]);
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);
  useAutoResize(textareaRef);
  const history = useInputHistory();

  const showMenu = value.startsWith('/') && !value.includes('\n');
  const filtered = showMenu ? getFilteredCommands(value.split(' ')[0] || '/') : [];
  const menuVisible = showMenu && filtered.length > 0;

  const hasContent = value.trim().length > 0 || files.length > 0;

  const handleVoiceTranscript = useCallback((text: string) => {
    setValue((prev) => {
      const needsSpace = prev.length > 0 && !prev.endsWith(' ');
      return prev + (needsSpace ? ' ' : '') + text.trim();
    });
  }, []);
  const { state: voiceState, toggle: toggleVoice, cancel: cancelVoice, isSupported: voiceSupported, mode: voiceMode } = useVoiceInput(handleVoiceTranscript);

  const addFiles = useCallback((newFiles: FileList | File[]) => {
    const valid = Array.from(newFiles).filter(isAllowedFile);
    if (valid.length > 0) {
      setFiles((prev) => [...prev, ...valid]);
    }
  }, []);

  const focus = useCallback(() => {
    textareaRef.current?.focus();
  }, []);

  useImperativeHandle(ref, () => ({ addFiles, focus }), [addFiles, focus]);

  const removeFile = useCallback((index: number) => {
    setFiles((prev) => prev.filter((_, i) => i !== index));
  }, []);

  const handleSubmit = (e?: FormEvent) => {
    e?.preventDefault();
    const trimmed = value.trim();
    if (!trimmed && files.length === 0) return;

    if (trimmed.startsWith('/') && files.length === 0) {
      const spaceIdx = trimmed.indexOf(' ');
      const cmd = spaceIdx === -1 ? trimmed : trimmed.slice(0, spaceIdx);
      const args = spaceIdx === -1 ? '' : trimmed.slice(spaceIdx + 1).trim();
      onSlashCommand(cmd.toLowerCase(), args);
    } else {
      onSend(trimmed, files.length > 0 ? files : undefined);
    }

    history.push(trimmed);
    history.reset();
    setValue('');
    setFiles([]);
    if (threadId) clearDraft(threadId);
    setSelectedIndex(0);
    if (textareaRef.current) {
      textareaRef.current.style.height = 'auto';
    }
  };

  const handleCommandSelect = (command: string) => {
    if (command === '/model') {
      setValue(command + ' ');
      textareaRef.current?.focus();
      setSelectedIndex(0);
    } else {
      onSlashCommand(command, '');
      setValue('');
      setSelectedIndex(0);
    }
  };

  const handleKeyDown = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Tab' && e.shiftKey) {
      e.preventDefault();
      onTogglePlanMode?.();
      return;
    }
    if (menuVisible) {
      if (e.key === 'ArrowUp') {
        e.preventDefault();
        setSelectedIndex((prev) => (prev > 0 ? prev - 1 : filtered.length - 1));
        return;
      }
      if (e.key === 'ArrowDown') {
        e.preventDefault();
        setSelectedIndex((prev) => (prev < filtered.length - 1 ? prev + 1 : 0));
        return;
      }
      if (e.key === 'Tab') {
        e.preventDefault();
        const cmd = filtered[selectedIndex];
        if (cmd) handleCommandSelect(cmd.name);
        return;
      }
      if (e.key === 'Enter' && !e.shiftKey) {
        e.preventDefault();
        const cmd = filtered[selectedIndex];
        if (cmd) handleCommandSelect(cmd.name);
        return;
      }
      if (e.key === 'Escape') {
        e.preventDefault();
        setValue('');
        setSelectedIndex(0);
        return;
      }
    }
    // Input history navigation (bash-style)
    if (e.key === 'ArrowUp') {
      const ta = textareaRef.current;
      if (ta) {
        const isSingleLine = !value.includes('\n');
        const cursorAtStart = ta.selectionStart === 0;
        if (isSingleLine || cursorAtStart) {
          const result = history.navigateUp(value);
          if (result !== null) {
            e.preventDefault();
            setValue(result);
          }
          return;
        }
      }
    }
    if (e.key === 'ArrowDown') {
      const ta = textareaRef.current;
      if (ta) {
        const isSingleLine = !value.includes('\n');
        const cursorAtEnd = ta.selectionStart === value.length;
        if (isSingleLine || cursorAtEnd) {
          const result = history.navigateDown();
          if (result !== null) {
            e.preventDefault();
            setValue(result);
          }
          return;
        }
      }
    }
    if (e.key === 'Escape' && voiceState === 'recording') {
      e.preventDefault();
      cancelVoice();
      return;
    }
    if (settings.sendOnEnter) {
      if (e.key === 'Enter' && !e.shiftKey) {
        e.preventDefault();
        handleSubmit();
      }
    } else {
      if (e.key === 'Enter' && (e.ctrlKey || e.metaKey)) {
        e.preventDefault();
        handleSubmit();
      }
    }
  };

  const handleChange = (newValue: string) => {
    setValue(newValue);
    setSelectedIndex(0);
  };

  const handlePaste = (e: React.ClipboardEvent) => {
    const items = e.clipboardData?.items;
    if (!items) return;
    const pastedFiles: File[] = [];
    for (const item of items) {
      if (item.kind === 'file') {
        const file = item.getAsFile();
        if (file && isAllowedFile(file)) {
          pastedFiles.push(file);
        }
      }
    }
    if (pastedFiles.length > 0) {
      e.preventDefault();
      addFiles(pastedFiles);
    }
  };

  const formatFileSize = (bytes: number) => {
    if (bytes < 1024) return bytes + ' B';
    if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB';
    return (bytes / (1024 * 1024)).toFixed(1) + ' MB';
  };

  return (
    <form
      onSubmit={handleSubmit}
      className="relative px-3 pb-4 pt-2 md:px-4"
    >
      {menuVisible && (
        <SlashCommandMenu
          filter={value.split(' ')[0] || '/'}
          selectedIndex={selectedIndex}
          onSelect={handleCommandSelect}
        />
      )}
      <div className={`bg-zinc-50 border rounded-2xl shadow-sm focus-within:shadow-md transition-all duration-200 ${
        planMode
          ? 'border-blue-400/40 focus-within:border-blue-400/60'
          : 'border-zinc-300 focus-within:border-zinc-400'
      }`}>
        {/* Plan/Act mode toggle */}
        <div className="flex items-center px-4 pt-2">
          <button
            type="button"
            onClick={onTogglePlanMode}
            className={`flex items-center gap-1.5 px-2 py-0.5 rounded-md text-[11px] font-medium transition-colors cursor-pointer ${
              planMode
                ? 'bg-blue-50 text-blue-600 hover:bg-blue-100'
                : 'bg-amber-50 text-amber-600 hover:bg-amber-100'
            }`}
            title="Shift+Tab to toggle"
          >
            {planMode ? (
              <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16" fill="currentColor" className="w-3 h-3">
                <path d="M2 4a2 2 0 0 1 2-2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V4Zm4.78 1.97a.75.75 0 0 1 0 1.06L5.81 8l.97.97a.75.75 0 1 1-1.06 1.06l-1.5-1.5a.75.75 0 0 1 0-1.06l1.5-1.5a.75.75 0 0 1 1.06 0Zm2.44 1.06a.75.75 0 0 1 1.06-1.06l1.5 1.5a.75.75 0 0 1 0 1.06l-1.5 1.5a.75.75 0 1 1-1.06-1.06l.97-.97-.97-.97Z" />
              </svg>
            ) : (
              <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16" fill="currentColor" className="w-3 h-3">
                <path fillRule="evenodd" d="M11.013 2.513a1.75 1.75 0 0 1 2.475 2.474L6.226 12.25a2.751 2.751 0 0 1-.892.596l-2.047.848a.75.75 0 0 1-.98-.98l.848-2.047a2.75 2.75 0 0 1 .596-.892l7.262-7.261Z" clipRule="evenodd" />
              </svg>
            )}
            {planMode ? 'Plan' : 'Act'}
          </button>
          <span className="text-[10px] text-zinc-400 ml-2">Shift+Tab</span>
        </div>
        {/* File previews */}
        {files.length > 0 && (
          <div className="flex flex-wrap gap-2 px-4 pt-3">
            {files.map((file, i) => (
              <div key={i} className="relative group/file flex items-center gap-2 bg-zinc-100 dark:bg-zinc-200 rounded-lg px-3 py-1.5 text-xs text-zinc-600">
                {file.type.startsWith('image/') ? (
                  <img
                    src={URL.createObjectURL(file)}
                    alt={file.name}
                    className="w-8 h-8 rounded object-cover"
                    onLoad={(e) => URL.revokeObjectURL((e.target as HTMLImageElement).src)}
                  />
                ) : (
                  <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="currentColor" className="w-4 h-4 text-zinc-400">
                    <path d="M3 3.5A1.5 1.5 0 014.5 2h6.879a1.5 1.5 0 011.06.44l4.122 4.12A1.5 1.5 0 0117 7.622V16.5a1.5 1.5 0 01-1.5 1.5h-11A1.5 1.5 0 013 16.5v-13z" />
                  </svg>
                )}
                <span className="max-w-[120px] truncate">{file.name}</span>
                <span className="text-zinc-400">{formatFileSize(file.size)}</span>
                <button
                  type="button"
                  onClick={() => removeFile(i)}
                  className="ml-1 text-zinc-400 hover:text-red-500 transition-colors"
                >
                  <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16" fill="currentColor" className="w-3.5 h-3.5">
                    <path d="M5.28 4.22a.75.75 0 00-1.06 1.06L6.94 8l-2.72 2.72a.75.75 0 101.06 1.06L8 9.06l2.72 2.72a.75.75 0 101.06-1.06L9.06 8l2.72-2.72a.75.75 0 00-1.06-1.06L8 6.94 5.28 4.22z" />
                  </svg>
                </button>
              </div>
            ))}
          </div>
        )}
        <div className="flex items-end gap-1">
          {/* File upload button */}
          <button
            type="button"
            onClick={() => fileInputRef.current?.click()}
            className="text-zinc-400 hover:text-zinc-600 p-3 transition-colors flex-shrink-0 cursor-pointer"
            title="Attach files"
          >
            <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="currentColor" className="w-5 h-5">
              <path fillRule="evenodd" d="M15.621 4.379a3 3 0 00-4.242 0l-7 7a3 3 0 004.241 4.243h.001l.497-.5a.75.75 0 011.064 1.057l-.498.501a4.5 4.5 0 01-6.364-6.364l7-7a4.5 4.5 0 016.368 6.36l-3.455 3.553A2.625 2.625 0 119.52 9.52l3.45-3.451a.75.75 0 111.061 1.06l-3.45 3.451a1.125 1.125 0 001.587 1.595l3.454-3.553a3 3 0 000-4.242z" clipRule="evenodd" />
            </svg>
          </button>
          <input
            ref={fileInputRef}
            type="file"
            multiple
            accept={[...ALLOWED_MIME_TYPES, ...ALLOWED_EXTENSIONS].join(',')}
            className="hidden"
            onChange={(e) => {
              if (e.target.files) addFiles(e.target.files);
              e.target.value = '';
            }}
          />
          <textarea
            ref={textareaRef}
            value={value}
            onChange={(e) => handleChange(e.target.value)}
            onKeyDown={handleKeyDown}
            onPaste={handlePaste}
            placeholder="Message Pan Botka..."
            rows={1}
            className="flex-1 bg-transparent py-3 pr-1
                       text-zinc-900 placeholder-zinc-400 resize-none
                       focus:outline-none
                       text-[15px] leading-relaxed
                       max-h-[150px]"
          />
          {/* Mic button */}
          {voiceSupported && (
            <button
              type="button"
              onClick={toggleVoice}
              disabled={voiceState === 'transcribing'}
              className={`relative p-2.5 m-1.5 rounded-xl transition-all duration-200 flex-shrink-0 cursor-pointer
                ${voiceState === 'recording'
                  ? 'bg-red-600 text-white animate-pulse shadow-md'
                  : voiceState === 'transcribing'
                    ? 'text-amber-500 animate-pulse cursor-wait'
                    : voiceState === 'denied'
                      ? 'text-red-400 hover:text-red-500'
                      : 'text-zinc-400 hover:text-zinc-600'
                }`}
              title={
                voiceState === 'recording' ? 'Stop recording (Esc to cancel)'
                : voiceState === 'transcribing' ? 'Transcribing...'
                : voiceState === 'denied' ? 'Microphone access denied — click to retry'
                : voiceMode === 'whisper' ? 'Voice input (Whisper)' : 'Voice input' as string
              }
            >
              {voiceState === 'transcribing' ? (
                <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="currentColor" className="w-4.5 h-4.5 animate-spin">
                  <path fillRule="evenodd" d="M15.312 11.424a5.5 5.5 0 01-9.201 2.466l-.312-.311h2.451a.75.75 0 000-1.5H4.5a.75.75 0 00-.75.75v3.75a.75.75 0 001.5 0v-2.033a7 7 0 0011.712-3.138.75.75 0 00-1.449-.386zm1.938-5.174a.75.75 0 00-1.5 0v2.033a7 7 0 00-11.712 3.138.75.75 0 001.449.386 5.5 5.5 0 019.201-2.466l.312.311h-2.451a.75.75 0 000 1.5H16.5a.75.75 0 00.75-.75v-3.75a.75.75 0 00-.75-.75v.598z" clipRule="evenodd" />
                </svg>
              ) : (
                <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="currentColor" className="w-4.5 h-4.5">
                  <path d="M7 4a3 3 0 0 1 6 0v6a3 3 0 1 1-6 0V4Z" />
                  <path d="M5.5 9.643a.75.75 0 0 0-1.5 0V10c0 3.06 2.29 5.585 5.25 5.954V17.5h-1.5a.75.75 0 0 0 0 1.5h4.5a.75.75 0 0 0 0-1.5h-1.5v-1.546A6.001 6.001 0 0 0 16 10v-.357a.75.75 0 0 0-1.5 0V10a4.5 4.5 0 0 1-9 0v-.357Z" />
                </svg>
              )}
            </button>
          )}
          {/* Stop / Send button */}
          {isStreaming ? (
            <button
              type="button"
              onClick={onStop}
              className="p-2.5 m-1.5 rounded-xl transition-all duration-200 flex-shrink-0 cursor-pointer bg-zinc-700 hover:bg-zinc-800 text-white shadow-md"
              title="Stop response"
            >
              <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="currentColor" className="w-4.5 h-4.5">
                <rect x="5" y="5" width="10" height="10" rx="1.5" />
              </svg>
            </button>
          ) : (
            <button
              type="submit"
              disabled={!hasContent}
              className={`p-2.5 m-1.5 rounded-xl transition-all duration-200 flex-shrink-0 cursor-pointer
                ${hasContent
                  ? 'bg-emerald-600 hover:bg-emerald-700 text-white shadow-md'
                  : 'text-zinc-300 opacity-0 pointer-events-none'
                }`}
            >
              <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="currentColor" className="w-4.5 h-4.5">
                <path d="M3.105 2.289a.75.75 0 00-.826.95l1.414 4.925A1.5 1.5 0 005.135 9.25h6.115a.75.75 0 010 1.5H5.135a1.5 1.5 0 00-1.442 1.086l-1.414 4.926a.75.75 0 00.826.95 28.896 28.896 0 0015.293-7.154.75.75 0 000-1.115A28.897 28.897 0 003.105 2.289z" />
              </svg>
            </button>
          )}
        </div>
        {queuedCount > 0 && (
          <div className="flex justify-end px-3 pb-2">
            <span className="text-xs text-zinc-400 flex items-center gap-1">
              <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16" fill="currentColor" className="w-3 h-3">
                <path fillRule="evenodd" d="M1 8a7 7 0 1 1 14 0A7 7 0 0 1 1 8Zm7.75-4.25a.75.75 0 0 0-1.5 0V8c0 .414.336.75.75.75h3.25a.75.75 0 0 0 0-1.5h-2.5v-3.5Z" clipRule="evenodd" />
              </svg>
              {queuedCount} queued
            </span>
          </div>
        )}
      </div>
    </form>
  );
});

export default ChatInput;
