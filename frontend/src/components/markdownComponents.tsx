import { lazy, Suspense } from 'react';
import type { Components } from 'react-markdown';

const CodeBlock = lazy(() => import('./CodeBlock'));

// Shared by MarkdownContent and the lazily loaded MathMarkdown so both render
// links, tables and code fences identically. Module-scoped because
// react-markdown treats a new `components` object as a changed prop.
export const MARKDOWN_COMPONENTS: Components = {
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
};
