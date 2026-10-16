// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import ReactMarkdown, { type Components } from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { Box } from '@resources/components/ui/Layout';
import { Caption } from '@resources/components/ui/Typography';
import { cn } from '@resources/utils/cn';

interface MarkdownProps {
  source: string;
  className?: string;
}

const maxMarkdownLength = 100_000;

const markdownComponents: Components = {
  a: ({ node: _node, children, ...props }) => (
    <a
      {...props}
      className="rounded-sm text-primary underline decoration-primary/50 underline-offset-2 transition-colors hover:decoration-primary focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
    >
      {children}
    </a>
  ),
  code: ({ node: _node, className, ...props }) => (
    <code
      {...props}
      className={cn('rounded bg-muted px-1.5 py-0.5 font-mono text-[0.8125rem] text-foreground', className)}
    />
  ),
  pre: ({ node: _node, ...props }) => (
    <pre
      {...props}
      // oxlint-disable-next-line jsx-a11y/no-noninteractive-tabindex -- a horizontally scrollable region must be keyboard-focusable (WCAG 2.1.1)
      tabIndex={0}
      className="my-3 max-w-full overflow-x-auto rounded-lg border border-border bg-muted/60 p-3 font-mono text-[0.8125rem] leading-relaxed focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none [&>code]:bg-transparent [&>code]:p-0"
    />
  ),
  table: ({ node: _node, ...props }) => (
    <Box
      // oxlint-disable-next-line jsx-a11y/no-noninteractive-tabindex -- a horizontally scrollable region must be keyboard-focusable (WCAG 2.1.1)
      tabIndex={0}
      className="my-3 max-w-full overflow-x-auto rounded-lg border border-border focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
    >
      <table {...props} className="w-full border-collapse text-left text-xs" />
    </Box>
  ),
  th: ({ node: _node, ...props }) => (
    <th {...props} scope="col" className="border-b border-border bg-muted/60 px-3 py-2 font-semibold text-foreground" />
  ),
  td: ({ node: _node, ...props }) => (
    <td {...props} className="border-b border-border px-3 py-2 align-top [tr:last-child_&]:border-b-0" />
  ),
  h1: 'h3',
  h2: 'h3',
  h3: 'h3',
  h4: 'h4',
  h5: 'h4',
  h6: 'h4',
  img: ({ node: _node, alt }) =>
    alt ? (
      <Caption as="span" className="italic">
        {alt}
      </Caption>
    ) : null,
};

/**
 * Renders untrusted Markdown without accepting embedded HTML or remote images.
 */
export function Markdown({ source, className }: MarkdownProps) {
  const renderedSource = source.length > maxMarkdownLength ? `${source.slice(0, maxMarkdownLength)}\n\n...` : source;

  return (
    <Box
      className={cn(
        'max-w-full text-sm leading-relaxed break-words text-foreground',
        '[&>*:first-child]:mt-0 [&>*:last-child]:mb-0',
        '[&_p]:my-2',
        '[&_h3]:mt-4 [&_h3]:mb-2 [&_h3]:text-base [&_h3]:font-semibold',
        '[&_h4]:mt-3 [&_h4]:mb-1 [&_h4]:font-semibold',
        '[&_ul]:my-2 [&_ul]:list-disc [&_ul]:space-y-1 [&_ul]:pl-5',
        '[&_ol]:my-2 [&_ol]:list-decimal [&_ol]:space-y-1 [&_ol]:pl-5',
        '[&_ul.contains-task-list]:list-none [&_ul.contains-task-list]:pl-0',
        '[&_.task-list-item]:list-none',
        '[&_li>p]:my-0',
        '[&_blockquote]:my-3 [&_blockquote]:border-l-2 [&_blockquote]:border-primary/50 [&_blockquote]:pl-3 [&_blockquote]:text-muted-foreground [&_blockquote]:italic',
        '[&_hr]:my-4 [&_hr]:border-border',
        '[&_del]:text-muted-foreground',
        '[&_input[type=checkbox]]:mr-2 [&_input[type=checkbox]]:accent-primary',
        className,
      )}
    >
      <ReactMarkdown remarkPlugins={[remarkGfm]} components={markdownComponents} skipHtml>
        {renderedSource}
      </ReactMarkdown>
    </Box>
  );
}
