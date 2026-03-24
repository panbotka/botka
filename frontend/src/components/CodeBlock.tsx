import { useState } from 'react';
import { Prism as SyntaxHighlighter } from 'react-syntax-highlighter';
import { oneDark } from 'react-syntax-highlighter/dist/esm/styles/prism';
import { oneLight } from 'react-syntax-highlighter/dist/esm/styles/prism';
import { useSettings } from '../context/SettingsContext';

interface Props {
  language: string;
  code: string;
}

export default function CodeBlock({ language, code }: Props) {
  const { resolvedTheme } = useSettings();
  const isLight = resolvedTheme === 'light';
  const [copied, setCopied] = useState(false);
  const [wordWrap, setWordWrap] = useState(false);

  const lineCount = code.split('\n').length;
  const showLineNumbers = lineCount > 10;

  const handleCopy = async () => {
    await navigator.clipboard.writeText(code);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <div className="relative group rounded-xl overflow-hidden my-3">
      <div className={`flex items-center justify-between px-4 py-2 text-xs ${
        isLight ? 'bg-zinc-100 text-zinc-500' : 'bg-zinc-800 text-zinc-400'
      }`}>
        <span>{language}</span>
        <div className="flex items-center gap-3">
          {lineCount > 5 && (
            <button
              onClick={() => setWordWrap(w => !w)}
              className="md:opacity-0 md:group-hover:opacity-100 transition-opacity duration-150
                         hover:text-zinc-700 cursor-pointer"
              title={wordWrap ? 'Disable word wrap' : 'Enable word wrap'}
            >
              {wordWrap ? 'No wrap' : 'Wrap'}
            </button>
          )}
          <button
            onClick={handleCopy}
            className="md:opacity-0 md:group-hover:opacity-100 transition-opacity duration-150
                       hover:text-zinc-700 cursor-pointer min-w-[52px] text-right"
          >
            {copied ? '✓ Copied' : 'Copy'}
          </button>
        </div>
      </div>
      <SyntaxHighlighter
        style={isLight ? oneLight : oneDark}
        language={language}
        PreTag="div"
        showLineNumbers={showLineNumbers}
        wrapLongLines={wordWrap}
        lineNumberStyle={{
          minWidth: '2.5em',
          paddingRight: '1em',
          color: isLight ? '#c0c0c0' : '#555',
          userSelect: 'none',
        }}
        customStyle={{
          margin: 0,
          borderRadius: 0,
          background: isLight ? 'rgb(250, 250, 252)' : 'rgb(30, 30, 35)',
          fontSize: '0.8rem',
        }}
      >
        {code}
      </SyntaxHighlighter>
    </div>
  );
}
