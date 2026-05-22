// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useRef } from 'react';

type LogViewerProps = {
  lines: string[];
  emptyMessage?: string;
  autoScroll?: boolean;
  className?: string;
};

export function LogViewer({
  lines,
  emptyMessage,
  autoScroll = true,
  className,
}: LogViewerProps) {
  const ref = useRef<HTMLPreElement>(null);

  useEffect(() => {
    if (autoScroll && ref.current) {
      ref.current.scrollTop = ref.current.scrollHeight;
    }
  }, [lines, autoScroll]);

  return (
    <div className={`min-h-[300px] overflow-hidden rounded-lg border border-border bg-[#0a0a0a] ${className ?? ''}`}>
      <pre
        ref={ref}
        className="h-full overflow-auto p-4 font-mono text-xs leading-5 text-zinc-300"
      >
        {lines.map((line, i) => (
          <div key={`log-line-${i + 1}-${line}`} className="hover:bg-white/5">
            {line}
          </div>
        ))}
        {lines.length === 0 && emptyMessage && (
          <span className="text-muted-foreground">{emptyMessage}</span>
        )}
      </pre>
    </div>
  );
}
