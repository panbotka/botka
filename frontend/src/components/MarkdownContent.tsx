import { lazy, memo, Suspense } from 'react';
import Markdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { linkifyTasksMarkdown } from '../utils/linkifyTasks';
import { hasMath } from '../utils/hasMath';
import { MARKDOWN_COMPONENTS } from './markdownComponents';

// KaTeX + remark-math + rehype-katex live in their own chunk, fetched the
// first time a message actually contains math.
const MathMarkdown = lazy(() => import('./MathMarkdown'));

// Hoisted to module scope: react-markdown treats new plugin arrays and a new
// `components` object as changed props and rebuilds its processor, so inline
// literals would defeat the memo() below on every parent render.
const REMARK_PLUGINS = [remarkGfm];

interface Props {
  content: string;
}

function MarkdownContent({ content }: Props) {
  const source = linkifyTasksMarkdown(content);
  const plain = (
    <Markdown remarkPlugins={REMARK_PLUGINS} components={MARKDOWN_COMPONENTS}>
      {source}
    </Markdown>
  );
  return (
    <div className="markdown-content">
      {hasMath(source) ? (
        // Until the math chunk arrives, show the message without math
        // rendering rather than a blank bubble.
        <Suspense fallback={plain}>
          <MathMarkdown content={source} />
        </Suspense>
      ) : (
        plain
      )}
    </div>
  );
}

// Parsing markdown (remark + rehype, plus KaTeX for math) is the single most
// expensive thing the chat view does. Memoizing on `content` keeps a streamed
// token from re-parsing every other message in the thread.
export default memo(MarkdownContent);
