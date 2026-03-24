import { useState, useRef, useEffect, type ReactNode, Children, isValidElement } from 'react';

interface Props {
  children: ReactNode;
  onSelect: (text: string) => void;
}

function extractText(node: ReactNode): string {
  if (typeof node === 'string') return node;
  if (typeof node === 'number') return String(node);
  if (!node) return '';
  if (Array.isArray(node)) return node.map(extractText).join('');
  if (isValidElement(node)) return extractText((node.props as any).children);
  return '';
}

export default function SelectableOptions({ children, onSelect }: Props) {
  const [activeIndex, setActiveIndex] = useState(-1);
  const containerRef = useRef<HTMLOListElement>(null);

  const items = Children.toArray(children).filter(isValidElement);

  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;

    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'ArrowDown') {
        e.preventDefault();
        setActiveIndex((prev) => (prev + 1) % items.length);
      } else if (e.key === 'ArrowUp') {
        e.preventDefault();
        setActiveIndex((prev) => (prev <= 0 ? items.length - 1 : prev - 1));
      } else if (e.key === 'Enter' && activeIndex >= 0) {
        e.preventDefault();
        const text = extractText(items[activeIndex]);
        if (text.trim()) onSelect(text.trim());
      }
    };

    container.addEventListener('keydown', handleKeyDown);
    return () => container.removeEventListener('keydown', handleKeyDown);
  }, [activeIndex, items, onSelect]);

  const handleItemClick = (index: number) => {
    const text = extractText(items[index]);
    if (text.trim()) onSelect(text.trim());
  };

  return (
    <ol
      ref={containerRef}
      tabIndex={0}
      className="my-2 space-y-1 outline-none list-none pl-0 cursor-pointer"
    >
      {items.map((child, i) => (
        <li
          key={i}
          onClick={() => handleItemClick(i)}
          onMouseEnter={() => setActiveIndex(i)}
          className={`flex items-center gap-2 px-3 py-2 rounded-lg transition-colors duration-100 ${
            activeIndex === i
              ? 'bg-emerald-50 text-emerald-700'
              : 'hover:bg-zinc-100 text-zinc-700'
          }`}
        >
          <span className={`flex-shrink-0 w-6 h-6 rounded-md flex items-center justify-center text-xs font-medium ${
            activeIndex === i
              ? 'bg-emerald-100 text-emerald-600'
              : 'bg-zinc-200 text-zinc-500'
          }`}>
            {i + 1}
          </span>
          <span className="flex-1">{((child as React.ReactElement).props as any).children}</span>
          {activeIndex === i && (
            <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16" fill="currentColor" className="w-4 h-4 text-emerald-500/60 flex-shrink-0">
              <path fillRule="evenodd" d="M2 8a.75.75 0 0 1 .75-.75h8.69L8.22 4.03a.75.75 0 0 1 1.06-1.06l4.5 4.5a.75.75 0 0 1 0 1.06l-4.5 4.5a.75.75 0 0 1-1.06-1.06l3.22-3.22H2.75A.75.75 0 0 1 2 8Z" clipRule="evenodd" />
            </svg>
          )}
        </li>
      ))}
      {activeIndex >= 0 && (
        <li className="text-[11px] text-zinc-400 pl-3 pt-1">
          Press Enter to select
        </li>
      )}
    </ol>
  );
}
