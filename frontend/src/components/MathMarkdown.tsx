import Markdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import remarkMath from 'remark-math';
import rehypeKatex from 'rehype-katex';
import 'katex/dist/katex.min.css';
import { MARKDOWN_COMPONENTS } from './markdownComponents';

// Loaded lazily by MarkdownContent only for content that contains math, so
// KaTeX (JS, CSS and fonts) never ships with messages that don't need it.
// Plugin arrays are hoisted so react-markdown doesn't rebuild its processor.
const REMARK_PLUGINS = [remarkGfm, remarkMath];
const REHYPE_PLUGINS = [rehypeKatex];

interface Props {
  content: string;
}

export default function MathMarkdown({ content }: Props) {
  return (
    <Markdown
      remarkPlugins={REMARK_PLUGINS}
      rehypePlugins={REHYPE_PLUGINS}
      components={MARKDOWN_COMPONENTS}
    >
      {content}
    </Markdown>
  );
}
