// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconUpload, IconRotate } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { cn } from '@resources/utils/cn';
import { fileToMarkPNG, MarkImageError, type MarkError } from './mark-image';

// One brand mark on the settings page: the wordmark that opens a report,
// or the small mark that repeats in the running header.
//
// The preview is the point of the control. A logo is judged by looking at
// it, and the operator should see what a cover is about to carry before a
// hundred-page PDF leaves the building.

type Props = {
  label: string;
  help: string;
  /** Base64 PNG with no `data:` prefix. Empty means the built-in mark. */
  value: string;
  /** Host asset URL for the built-in mark, shown while `value` is empty. */
  defaultSrc: string;
  /** Longest edge the rasteriser targets. Wordmarks want more than marks. */
  maxSide: number;
  wide?: boolean;
  onChange: (next: string) => void;
};

export function MarkField({ label, help, value, defaultSrc, maxSide, wide, onChange }: Props) {
  const { t } = useTranslation('reporting');
  const input = useRef<HTMLInputElement>(null);
  const [error, setError] = useState<MarkError | null>(null);
  const [busy, setBusy] = useState(false);

  const pick = async (file: File | undefined) => {
    if (!file) return;
    setError(null);
    setBusy(true);
    try {
      onChange(await fileToMarkPNG(file, maxSide));
    } catch (err) {
      setError(err instanceof MarkImageError ? err.reason : 'badImage');
    } finally {
      setBusy(false);
      // Clearing the input lets the same file be picked again after a fix.
      if (input.current) input.current.value = '';
    }
  };

  return (
    <div className="grid gap-2 sm:grid-cols-[minmax(0,1fr)_16rem] sm:items-start sm:gap-6">
      <div>
        <p className="text-sm font-medium text-foreground">{label}</p>
        <p className="mt-0.5 text-xs leading-relaxed text-muted-foreground">{help}</p>
        <p className="mt-1 text-xs text-muted-foreground">{t('settings.mark.hint')}</p>
      </div>

      <div className="space-y-2">
        <div
          className={cn(
            'flex items-center justify-center rounded-lg border border-border bg-muted p-3',
            wide ? 'h-20' : 'h-20 w-20',
          )}
        >
          <img
            src={value ? `data:image/png;base64,${value}` : defaultSrc}
            alt={label}
            className="max-h-full max-w-full object-contain"
          />
        </div>

        <div className="flex items-center gap-2">
          <Button variant="secondary" size="sm" disabled={busy} onClick={() => input.current?.click()}>
            <IconUpload className="size-4" />
            {t('settings.mark.upload')}
          </Button>
          <Button
            variant="ghost"
            size="sm"
            disabled={busy || value === ''}
            onClick={() => {
              setError(null);
              onChange('');
            }}
          >
            <IconRotate className="size-4" />
            {t('settings.mark.reset')}
          </Button>
        </div>

        <p className={cn('text-xs', error ? 'text-danger' : 'text-muted-foreground')}>
          {error ? t(`settings.mark.${error}`) : value ? '' : t('settings.mark.default')}
        </p>

        <input
          ref={input}
          type="file"
          accept="image/png,image/svg+xml"
          className="hidden"
          aria-label={label}
          onChange={(e) => void pick(e.target.files?.[0])}
        />
      </div>
    </div>
  );
}
