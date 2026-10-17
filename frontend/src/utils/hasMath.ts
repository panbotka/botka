// Cheap gate in front of the KaTeX pipeline: only content that may contain
// math pays for downloading remark-math, rehype-katex and the KaTeX CSS.
// It must be a superset of what remark-math (default settings, so single-
// dollar inline math is on) would parse — a false positive just loads KaTeX
// for nothing, a false negative would show raw `$…$` source.
//   - `$$`         display math
//   - `\(` / `\[`  LaTeX-style delimiters
//   - `$…$`        inline math; may span lines like a code span
const MATH_RE = /\$\$|\\\(|\\\[|\$[^$]+\$/;

/** Reports whether markdown content contains any math delimiters. */
export function hasMath(content: string): boolean {
  return MATH_RE.test(content);
}
