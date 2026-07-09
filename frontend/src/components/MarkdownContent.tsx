import { lazy, memo, Suspense } from 'react';
import Markdown from 'react-markdown';
import type { Components } from 'react-markdown';
import remarkGfm from 'remark-gfm';
import remarkMath from 'remark-math';
import rehypeKatex from 'rehype-katex';
import 'katex/dist/katex.min.css';
import { linkifyTasksMarkdown } from '../utils/linkifyTasks';

const CodeBlock = lazy(() => import('./CodeBlock'));

// Hoisted to module scope: react-markdown treats new plugin arrays and a new
// `components` object as changed props and rebuilds its processor, so inline
// literals would defeat the memo() below on every parent render.
const REMARK_PLUGINS = [remarkGfm, remarkMath];
const REHYPE_PLUGINS = [rehypeKatex];

const COMPONENTS: Components = {
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

interface Props {
  content: string;
}

function MarkdownContent({ content }: Props) {
  return (
    <div className="markdown-content">
      <Markdown
        remarkPlugins={REMARK_PLUGINS}
        rehypePlugins={REHYPE_PLUGINS}
        components={COMPONENTS}
      >
        {linkifyTasksMarkdown(content)}
      </Markdown>
    </div>
  );
}

// Parsing markdown (remark + rehype + KaTeX) is the single most expensive thing
// the chat view does. Memoizing on `content` keeps a streamed token from
// re-parsing every other message in the thread.
export default memo(MarkdownContent);
