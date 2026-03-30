import { useState } from 'react';

interface CodeBlockProps {
  code: string;
  language?: string;
  maxHeight?: string;
}

export function CodeBlock({ code, language, maxHeight = '400px' }: CodeBlockProps) {
  const [copied, setCopied] = useState(false);

  const handleCopy = async () => {
    await navigator.clipboard.writeText(code);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <div className="relative group">
      <button
        onClick={handleCopy}
        className="absolute top-2 right-2 px-2 py-1 text-xs rounded bg-zinc-700 text-zinc-300 hover:bg-zinc-600 opacity-0 group-hover:opacity-100 transition-opacity z-10"
      >
        {copied ? 'Copied!' : 'Copy'}
      </button>
      <pre
        className="bg-zinc-950 border border-zinc-800 rounded-lg p-4 overflow-auto text-sm font-mono text-zinc-300"
        style={{ maxHeight }}
      >
        <code className={language ? `language-${language}` : ''}>{code}</code>
      </pre>
    </div>
  );
}
