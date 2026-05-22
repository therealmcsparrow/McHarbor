import { Suspense, lazy } from 'react';

type CronSchedulePreviewProps = {
  expression: string;
  timezone?: string | null;
  className?: string;
};

const CronSchedulePreviewImpl = lazy(() =>
  import('./CronSchedulePreviewImpl').then((module) => ({ default: module.CronSchedulePreviewImpl }))
);

export function CronSchedulePreview({ expression, timezone, className }: CronSchedulePreviewProps) {
  if (!expression.trim()) {
    return null;
  }

  return (
    <Suspense
      fallback={
        <div className={className ?? 'mt-2 rounded-md border border-border/60 bg-muted/30 p-2'}>
          <div className="space-y-1">
            <div className="h-3 w-24 animate-pulse rounded bg-muted/70" />
            <div className="h-3 w-full animate-pulse rounded bg-muted/60" />
            <div className="h-3 w-5/6 animate-pulse rounded bg-muted/60" />
            <div className="h-3 w-2/3 animate-pulse rounded bg-muted/60" />
          </div>
        </div>
      }
    >
      <CronSchedulePreviewImpl expression={expression} timezone={timezone} className={className} />
    </Suspense>
  );
}
