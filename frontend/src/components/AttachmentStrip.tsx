import { useState } from 'react';
import type { Attachment } from '../types';
import {
  ChevronDown,
  Download,
  File,
  FileCode,
  FileImage,
  FileJson,
  FileSpreadsheet,
  FileText,
  Paperclip,
  type LucideIcon,
} from 'lucide-react';

/** More attachments than this collapse into a single "N souborů" row. */
export const COLLAPSE_THRESHOLD = 4;

/** How many tiny image previews the collapsed row shows. */
const COLLAPSED_PREVIEWS = 3;

export type AttachmentKind = 'image' | 'pdf' | 'html' | 'csv' | 'json' | 'code' | 'text' | 'other';

const CODE_EXTENSIONS = new Set([
  'js', 'jsx', 'ts', 'tsx', 'go', 'py', 'rb', 'rs', 'java', 'c', 'h', 'cpp', 'sh', 'sql',
  'css', 'scss', 'xml', 'yaml', 'yml', 'toml',
]);
const SPREADSHEET_EXTENSIONS = new Set(['csv', 'tsv', 'xls', 'xlsx', 'ods']);

function extensionOf(name: string): string {
  const dot = name.lastIndexOf('.');
  return dot > 0 ? name.slice(dot + 1).toLowerCase() : '';
}

/** Classifies an attachment by MIME type, falling back to the file extension. */
export function attachmentKind(att: Attachment): AttachmentKind {
  const mime = att.mime_type.toLowerCase();
  const ext = extensionOf(att.original_name);
  if (mime.startsWith('image/')) return 'image';
  if (mime === 'application/pdf' || ext === 'pdf') return 'pdf';
  if (mime === 'text/html' || mime === 'application/xhtml+xml' || ext === 'html' || ext === 'htm') return 'html';
  if (mime === 'text/csv' || mime === 'text/tab-separated-values' || mime.includes('spreadsheet') || mime.includes('excel') || SPREADSHEET_EXTENSIONS.has(ext)) return 'csv';
  if (mime === 'application/json' || mime.endsWith('+json') || ext === 'json') return 'json';
  if (CODE_EXTENSIONS.has(ext)) return 'code';
  if (mime.startsWith('text/')) return 'text';
  return 'other';
}

const KIND_ICONS: Record<AttachmentKind, LucideIcon> = {
  image: FileImage,
  pdf: FileText,
  html: FileCode,
  csv: FileSpreadsheet,
  json: FileJson,
  code: FileCode,
  text: FileText,
  other: File,
};

/** PDF, HTML and text open in a new tab; everything else downloads. */
function opensInBrowser(att: Attachment): boolean {
  const kind = attachmentKind(att);
  return kind === 'pdf' || kind === 'html' || att.mime_type.startsWith('text/');
}

export function formatFileSize(bytes: number) {
  if (bytes < 1024) return bytes + ' B';
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB';
  return (bytes / (1024 * 1024)).toFixed(1) + ' MB';
}

/**
 * Splits a filename so the stem can truncate while the extension stays
 * visible ("very-long-rep….pdf" rather than "very-long-report-…").
 */
function splitName(name: string): [string, string] {
  const dot = name.lastIndexOf('.');
  if (dot <= 0 || dot === name.length - 1) return [name, ''];
  return [name.slice(0, dot), name.slice(dot)];
}

type Variant = 'user' | 'assistant';

interface Palette {
  chip: string;
  icon: string;
  name: string;
  muted: string;
  hover: string;
  thumbBorder: string;
}

const PALETTES: Record<Variant, Palette> = {
  user: {
    chip: 'bg-white/10',
    icon: 'text-white/70',
    name: 'text-white',
    muted: 'text-white/50',
    hover: 'hover:text-white',
    thumbBorder: 'border-white/20',
  },
  assistant: {
    chip: 'bg-zinc-100 border border-zinc-200 dark:bg-zinc-200',
    icon: 'text-zinc-500',
    name: 'text-zinc-700',
    muted: 'text-zinc-400',
    hover: 'hover:text-zinc-700',
    thumbBorder: 'border-zinc-200',
  },
};

function FileChip({ attachment, palette }: { attachment: Attachment; palette: Palette }) {
  const kind = attachmentKind(attachment);
  const Icon = KIND_ICONS[kind];
  const [stem, ext] = splitName(attachment.original_name);
  const inBrowser = opensInBrowser(attachment);

  // The chip is 32px tall visually; both links are 44px tall and overflow it
  // vertically (the chip list's row gap keeps neighbours' targets apart).
  return (
    <div
      data-testid="attachment-chip"
      data-kind={kind}
      className={`flex items-center h-8 max-w-full min-w-0 rounded-lg text-[13px] leading-none ${palette.chip}`}
    >
      <a
        href={attachment.url}
        {...(inBrowser
          ? { target: '_blank', rel: 'noopener noreferrer' }
          : { download: attachment.original_name })}
        title={attachment.original_name}
        className="flex items-center gap-1.5 h-11 min-w-0 pl-2 pr-1 no-underline"
        onClick={(e) => e.stopPropagation()}
      >
        <Icon size={15} className={`flex-shrink-0 ${palette.icon}`} aria-hidden="true" />
        <span className={`flex min-w-0 font-medium ${palette.name}`}>
          <span className="truncate">{stem}</span>
          <span className="flex-shrink-0">{ext}</span>
        </span>
        <span className={`flex-shrink-0 text-[11px] ${palette.muted}`}>{formatFileSize(attachment.size)}</span>
      </a>
      <a
        href={attachment.url}
        download={attachment.original_name}
        title="Stáhnout"
        aria-label={`Stáhnout ${attachment.original_name}`}
        className={`flex items-center justify-center h-11 w-9 flex-shrink-0 transition-colors ${palette.muted} ${palette.hover}`}
        onClick={(e) => e.stopPropagation()}
      >
        <Download size={14} aria-hidden="true" />
      </a>
    </div>
  );
}

function ExpandedStrip({ images, files, palette, onImageClick }: {
  images: Attachment[];
  files: Attachment[];
  palette: Palette;
  onImageClick?: (attachment: Attachment, allImages: Attachment[]) => void;
}) {
  return (
    <div className="flex flex-col gap-2 min-w-0 max-w-full">
      {images.length > 0 && (
        <div
          data-testid="attachment-thumbnails"
          className="flex gap-2 overflow-x-auto snap-x snap-mandatory scrollbar-none min-w-0 max-w-full"
        >
          {images.map((att) => (
            <button
              key={att.id}
              type="button"
              onClick={() => onImageClick?.(att, images)}
              title={att.original_name}
              className={`flex-shrink-0 snap-start w-[72px] h-[72px] rounded-lg overflow-hidden border cursor-pointer hover:opacity-80 transition-opacity ${palette.thumbBorder}`}
            >
              <img
                src={att.url}
                alt={att.original_name}
                loading="lazy"
                className="w-full h-full object-cover"
              />
            </button>
          ))}
        </div>
      )}
      {files.length > 0 && (
        <div className="flex flex-wrap gap-x-2 gap-y-3 py-1.5 min-w-0 max-w-full">
          {files.map((att) => (
            <FileChip key={att.id} attachment={att} palette={palette} />
          ))}
        </div>
      )}
    </div>
  );
}

/**
 * Compact attachment strip under a chat message: image thumbnails in one
 * sideways-scrolling row, then one-line file chips. More than
 * COLLAPSE_THRESHOLD attachments collapse into a single toggle row.
 */
export default function AttachmentStrip({ attachments, variant, onImageClick }: {
  attachments: Attachment[];
  variant: Variant;
  onImageClick?: (attachment: Attachment, allImages: Attachment[]) => void;
}) {
  const [expanded, setExpanded] = useState(false);
  if (attachments.length === 0) return null;

  const palette = PALETTES[variant];
  const images = attachments.filter((a) => attachmentKind(a) === 'image');
  const files = attachments.filter((a) => attachmentKind(a) !== 'image');
  const collapsible = attachments.length > COLLAPSE_THRESHOLD;

  const strip = (
    <ExpandedStrip images={images} files={files} palette={palette} onImageClick={onImageClick} />
  );

  if (!collapsible) {
    return <div className="mt-2 min-w-0 max-w-full">{strip}</div>;
  }

  return (
    <div className="mt-2 flex flex-col gap-1 min-w-0 max-w-full">
      <button
        type="button"
        data-testid="attachment-toggle"
        aria-expanded={expanded}
        onClick={() => setExpanded((v) => !v)}
        className={`self-start flex items-center gap-2 min-h-[44px] px-2 rounded-lg text-[13px] cursor-pointer ${palette.chip}`}
      >
        <Paperclip size={15} className={palette.icon} aria-hidden="true" />
        <span className={`font-medium ${palette.name}`}>{attachments.length} souborů</span>
        {!expanded && images.length > 0 && (
          <span className="flex gap-1">
            {images.slice(0, COLLAPSED_PREVIEWS).map((att) => (
              <img
                key={att.id}
                src={att.url}
                alt=""
                loading="lazy"
                data-testid="attachment-mini-preview"
                className="w-6 h-6 rounded object-cover"
              />
            ))}
          </span>
        )}
        <ChevronDown
          size={15}
          className={`${palette.muted} transition-transform ${expanded ? 'rotate-180' : ''}`}
          aria-hidden="true"
        />
      </button>
      {expanded && strip}
    </div>
  );
}
