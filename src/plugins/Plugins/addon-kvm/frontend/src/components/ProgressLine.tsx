// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ProgressBar } from '@resources/components/ui/ProgressBar';
import { Spinner } from '@resources/components/ui/Spinner';

/**
 * A row for work happening on the host.
 *
 * The host reports milestones — quiesce, pause, write the memory, freeze the
 * disks — not a byte count, so a bar fed straight from them sits at one number
 * for a minute and then jumps. That reads as hung, which is exactly what it was
 * reported as.
 *
 * Two things fix it without inventing progress that does not exist. The bar
 * creeps toward the milestone it was given rather than snapping to it, so it is
 * always moving while the guest is busy, and it never passes the milestone or
 * goes backwards. And the elapsed time is on the row, which is the honest
 * answer to "is this still alive" for a step that genuinely takes minutes.
 */
export function ProgressLine({ title, step, percent }: { title: string; step: string; percent: number }) {
  const { t } = useTranslation('vm');
  const shown = useCreepingPercent(percent);
  const elapsed = useElapsedSeconds();

  return (
    <div className="flex min-w-0 items-center gap-2 rounded-lg border border-dashed border-primary/60 bg-primary/5 px-3 py-1.5">
      <Spinner className="size-3.5 shrink-0" />
      <span className="truncate text-sm font-medium text-foreground">{title}</span>
      <span className="shrink-0 text-[11px] text-muted-foreground">{step}</span>
      <span className="ml-auto shrink-0 text-[11px] text-muted-foreground tabular-nums">
        {t('progress.elapsed', { seconds: elapsed })}
      </span>
      <ProgressBar size="sm" className="w-40 shrink-0" value={shown} />
      <span className="w-9 shrink-0 text-right text-[11px] text-muted-foreground tabular-nums">
        {Math.round(shown)}%
      </span>
    </div>
  );
}

/**
 * A percentage that walks toward its target instead of jumping to it, and stops
 * just short so the bar never claims a step is finished before the host says so.
 */
function useCreepingPercent(target: number) {
  const [shown, setShown] = useState(target);
  const targetRef = useRef(target);
  useEffect(() => {
    targetRef.current = target;
  }, [target]);

  useEffect(() => {
    const timer = window.setInterval(() => {
      setShown((current) => {
        const goal = targetRef.current;
        if (current >= goal) return Math.max(current, goal);
        // A third of the remaining gap each tick: fast where there is room,
        // barely moving as it approaches, which is what makes a long step look
        // like work rather than a stall.
        return current + Math.max((goal - current) / 3, 0.4);
      });
    }, 400);
    return () => window.clearInterval(timer);
  }, []);

  return Math.min(shown, 100);
}

/** Seconds since the row appeared. */
function useElapsedSeconds() {
  const startedAt = useRef<number | null>(null);
  const [seconds, setSeconds] = useState(0);

  useEffect(() => {
    startedAt.current = Date.now();
    const timer = window.setInterval(() => {
      setSeconds(Math.round((Date.now() - (startedAt.current ?? Date.now())) / 1000));
    }, 1000);
    return () => window.clearInterval(timer);
  }, []);

  return seconds;
}
