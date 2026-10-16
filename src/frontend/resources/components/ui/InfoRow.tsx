// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ReactNode } from 'react';
import { cn } from '@resources/utils/cn';

/**
 * How the row divides its width.
 *
 * `fixed` — a 144px label column, value left-aligned and wrapping. Right for
 * rows whose label is a translated field name of predictable length and whose
 * value may legitimately need several lines: a container's `Cmd`, an image's
 * `Digests`, a volume's `Mountpoint`. Truncating those would hide data.
 *
 * `spread` — the label takes its natural width on the left and is never cut;
 * the value is pushed to the right edge, right-aligned, and ellipsised. Right
 * for rows whose *key is data* rather than a caption — driver options, labels,
 * environment variables — where keys vary wildly in length, a fixed column
 * either wastes space or overflows, and the value is a short scalar.
 */
export type InfoRowVariant = 'fixed' | 'spread';

type InfoRowProps = {
  label: string;
  children: ReactNode;
  variant?: InfoRowVariant;
  /**
   * Full value text, surfaced on hover. Worth passing whenever `spread` might
   * truncate — the ellipsis is otherwise a dead end.
   */
  title?: string;
  className?: string;
};

export function InfoRow({ label, children, variant = 'fixed', title, className }: InfoRowProps) {
  const spread = variant === 'spread';

  return (
    <div
      className={cn(
        'flex min-w-0 gap-4 border-b border-border py-2 last:border-0',
        spread && 'items-baseline',
        className,
      )}
    >
      {/* `w-36 shrink-0` fixes the label column at 144px, but a fixed width is
          only half a contract — the text still has to be told what to do when
          it does not fit, and a key like `com.docker.network.bridge.enable_icc`
          has no whitespace for the default `overflow-wrap: normal` to break at.
          `break-words` breaks the token only when it would otherwise overflow.
          In `spread` there is no fixed column to overflow: the label simply
          takes what it needs. */}
      <span className={cn('shrink-0 text-sm font-medium break-words text-muted-foreground', !spread && 'w-36')}>
        {label}
      </span>
      {/* `min-w-0` is what makes `truncate` work at all: without it a flex item
          keeps `min-width: auto` and refuses to shrink below its own content,
          so the value pushes the row wider instead of ellipsising. */}
      <span
        title={title}
        className={cn('min-w-0 text-sm text-foreground', spread ? 'ml-auto truncate text-right' : 'break-words')}
      >
        {children}
      </span>
    </div>
  );
}
