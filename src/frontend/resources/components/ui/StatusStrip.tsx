// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { Box, Flex } from '@resources/components/ui/Layout';
import { cn } from '@resources/utils/cn';

/**
 * How one block reads:
 *
 *   pass  a full-height block   — the probe passed / the container was up
 *   fail  a short centred bar   — the probe failed / the container was down
 *   warn  a half-height block   — degraded but not down (starting, unhealthy
 *                                 while still running)
 *   idle  a faint full block    — no observation for this slot yet
 *
 * Failures are drawn as a squat bar rather than a red block on purpose: the
 * silhouette alone marks them out, so the strip stays readable for anyone who
 * cannot separate the red from the green.
 */
export type StripTone = 'pass' | 'fail' | 'warn' | 'idle';

export type StripBlock = {
  key: string;
  tone: StripTone;
  /** Translated hover text for this single block. */
  label: string;
};

type StatusStripProps = {
  blocks: StripBlock[];
  /**
   * Slots to fill. Shorter histories are padded on the left with idle blocks
   * so the strip keeps its width and new observations arrive from the right.
   */
  slots?: number;
  /** Translated description of the whole strip, for screen readers. */
  ariaLabel: string;
  className?: string;
};

const TONE_CLASSES: Record<StripTone, string> = {
  pass: 'bg-success',
  fail: 'bg-destructive',
  warn: 'bg-warning',
  idle: 'bg-muted',
};

export function StatusStrip({ blocks, slots = 30, ariaLabel, className }: StatusStripProps) {
  const visible = blocks.slice(-slots);
  const padding = Math.max(0, slots - visible.length);

  return (
    <>
      {ariaLabel && <span className="sr-only">{ariaLabel}</span>}
      <Flex align="center" gap={1} className={cn('w-full', className)} aria-hidden={ariaLabel ? true : undefined}>
        {Array.from({ length: padding }, (_, index) => (
          <Box key={`pad-${index}`} className="flex h-7 flex-1 basis-0 items-center justify-center">
            <Box className="h-full w-full bg-muted/40" />
          </Box>
        ))}

        {visible.map((block) => (
          <Box key={block.key} title={block.label} className="flex h-7 flex-1 basis-0 items-center justify-center">
            {block.tone === 'fail' ? (
              <Box className="h-1.5 w-full rounded-full bg-destructive" />
            ) : (
              <Box className={cn('w-full', block.tone === 'warn' ? 'h-4' : 'h-full', TONE_CLASSES[block.tone])} />
            )}
          </Box>
        ))}
      </Flex>
    </>
  );
}
