import React from "react";

const URL_REGEX = /https?:\/\/[^\s<>]+|www\.[^\s<>]+/g;
const TRAILING_PUNCT = /[.,;:!?)}\]]+$/;

export function linkifyText(text: string): React.ReactNode {
  const matches = [...text.matchAll(URL_REGEX)];
  if (matches.length === 0) return text;

  const parts: React.ReactNode[] = [];
  let lastIndex = 0;

  for (const match of matches) {
    const matchStart = match.index!;
    let url = match[0];

    // Strip trailing punctuation that's likely not part of the URL
    const trailingMatch = url.match(TRAILING_PUNCT);
    const stripped = trailingMatch ? trailingMatch[0] : "";
    url = url.slice(0, url.length - stripped.length);

    // Add text before this URL
    if (matchStart > lastIndex) {
      parts.push(text.slice(lastIndex, matchStart));
    }

    const href = url.startsWith("www.") ? `https://${url}` : url;
    parts.push(
      <a
        key={matchStart}
        href={href}
        target="_blank"
        rel="noopener noreferrer"
        className="underline decoration-current/40 underline-offset-2 hover:decoration-current/70 cursor-pointer break-all"
      >
        {url}
      </a>
    );

    lastIndex = matchStart + url.length;
  }

  // Add remaining text
  if (lastIndex < text.length) {
    parts.push(text.slice(lastIndex));
  }

  return parts;
}
