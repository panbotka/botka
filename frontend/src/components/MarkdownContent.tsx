import { lazy, Suspense } from 'react';
import Markdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import remarkMath from 'remark-math';
import rehypeKatex from 'rehype-katex';
import 'katex/dist/katex.min.css';
import SelectableOptions from './SelectableOptions';
import { linkifyTasksMarkdown } from '../utils/linkifyTasks';

const CodeBlock = lazy(() => import('./CodeBlock'));

interface Props {
  content: string;
  onOptionSelect?: (text: string) => void;
}

export default function MarkdownContent({ content, onOptionSelect }: Props) {
  return (
    <div className="markdown-content">
    <Markdown
      remarkPlugins={[remarkGfm, remarkMath]}
      rehypePlugins={[rehypeKatex]}
      components={{
        ol({ children }) {
          if (onOptionSelect) {
            return <SelectableOptions onSelect={onOptionSelect}>{children}</SelectableOptions>;
          }
          return <ol>{children}</ol>;
        },
        a({ children, ...props }) {
          return (
            <a target="_blank" rel="noopener noreferrer" {...props}>
              {children}
            </a>
          );
        },
        table({ children, ...props }) {
          return (
            <div className="table-wrapper">
              <table {...props}>{children}</table>
            </div>
          );
        },
        code({ className, children, ...props }) {
          const match = /language-(\w+)/.exec(className || '');
          const code = String(children).replace(/\n$/, '');
          if (match) {
            return (
              <Suspense
                fallback={
                  <pre className="bg-zinc-100 rounded-lg overflow-hidden my-3 p-4 text-sm text-zinc-700">
                    <code>{code}</code>
                  </pre>
                }
              >
                <CodeBlock language={match[1] || ''} code={code} />
              </Suspense>
            );
          }
          return (
            <code className="bg-zinc-200 px-1.5 py-0.5 rounded text-sm" {...props}>
              {children}
            </code>
          );
        },
      }}
    >
      {linkifyTasksMarkdown(content)}
    </Markdown>
    </div>
  );
}
