// The deep-import module declarations live in the package root typings,
// which are only loaded when something references them.
/// <reference types="react-syntax-highlighter" />
import PrismLight from 'react-syntax-highlighter/dist/esm/prism-light';
import bash from 'react-syntax-highlighter/dist/esm/languages/prism/bash';
import css from 'react-syntax-highlighter/dist/esm/languages/prism/css';
import diff from 'react-syntax-highlighter/dist/esm/languages/prism/diff';
import docker from 'react-syntax-highlighter/dist/esm/languages/prism/docker';
import go from 'react-syntax-highlighter/dist/esm/languages/prism/go';
import ini from 'react-syntax-highlighter/dist/esm/languages/prism/ini';
import javascript from 'react-syntax-highlighter/dist/esm/languages/prism/javascript';
import json from 'react-syntax-highlighter/dist/esm/languages/prism/json';
import jsx from 'react-syntax-highlighter/dist/esm/languages/prism/jsx';
import makefile from 'react-syntax-highlighter/dist/esm/languages/prism/makefile';
import markdown from 'react-syntax-highlighter/dist/esm/languages/prism/markdown';
import markup from 'react-syntax-highlighter/dist/esm/languages/prism/markup';
import nginx from 'react-syntax-highlighter/dist/esm/languages/prism/nginx';
import python from 'react-syntax-highlighter/dist/esm/languages/prism/python';
import rust from 'react-syntax-highlighter/dist/esm/languages/prism/rust';
import sql from 'react-syntax-highlighter/dist/esm/languages/prism/sql';
import toml from 'react-syntax-highlighter/dist/esm/languages/prism/toml';
import tsx from 'react-syntax-highlighter/dist/esm/languages/prism/tsx';
import typescript from 'react-syntax-highlighter/dist/esm/languages/prism/typescript';
import yaml from 'react-syntax-highlighter/dist/esm/languages/prism/yaml';

// The full Prism build ships ~300 grammars (~630 kB). Chat and task output
// only ever show a handful, so register a curated set on the light build and
// let everything else fall back to plain preformatted text.
const LANGUAGES: Record<string, unknown> = {
  bash,
  css,
  diff,
  docker,
  go,
  ini,
  javascript,
  json,
  jsx,
  makefile,
  markdown,
  markup,
  nginx,
  python,
  rust,
  sql,
  toml,
  tsx,
  typescript,
  yaml,
};

for (const [name, grammar] of Object.entries(LANGUAGES)) {
  PrismLight.registerLanguage(name, grammar);
}

// Fence tags as models actually write them, mapped onto a registered grammar.
const ALIASES: Record<string, string> = {
  sh: 'bash',
  zsh: 'bash',
  shell: 'bash',
  console: 'bash',
  ts: 'typescript',
  js: 'javascript',
  mjs: 'javascript',
  cjs: 'javascript',
  yml: 'yaml',
  golang: 'go',
  py: 'python',
  html: 'markup',
  xml: 'markup',
  svg: 'markup',
  md: 'markdown',
  dockerfile: 'docker',
  make: 'makefile',
  rs: 'rust',
  patch: 'diff',
  jsonc: 'json',
};

/**
 * Maps a code-fence language tag onto a registered Prism grammar, or `text`
 * when it is unknown so the block renders unhighlighted instead of throwing.
 */
export function resolvePrismLanguage(language: string): string {
  const key = language.toLowerCase();
  if (key in LANGUAGES) return key;
  return ALIASES[key] ?? 'text';
}

export { PrismLight };
