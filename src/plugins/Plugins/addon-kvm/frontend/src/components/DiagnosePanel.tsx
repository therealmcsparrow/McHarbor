// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconSparkles, IconX } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Spinner } from '@resources/components/ui/Spinner';

/**
 * The AI's reading of why a guest is misbehaving.
 *
 * Two things this panel is careful about, both of which matter more than the
 * answer itself:
 *
 *   - It says the answer is a guess. A model reading a guest definition can be
 *     confidently wrong, and an operator acting on it may destroy a disk.
 *   - It says exactly what was sent. "We asked an AI" is not consent; naming
 *     the hardware settings and the error — and naming what was NOT sent — is.
 */
export function DiagnosePanel({
  pending,
  result,
  onDismiss,
}: {
  pending: boolean;
  result?: { available: boolean; diagnosis?: string };
  onDismiss: () => void;
}) {
  const { t } = useTranslation('vm');

  return (
    <section className="rounded-lg border border-border bg-card p-4">
      <header className="flex items-start justify-between gap-2">
        <div className="flex items-center gap-2">
          <IconSparkles className="h-4 w-4 text-primary" aria-hidden="true" />
          <h3 className="font-medium text-foreground">{t('diagnose.title')}</h3>
        </div>
        <Button variant="ghost" size="icon" aria-label={t('actions.cancel')} onClick={onDismiss}>
          <IconX className="h-4 w-4" aria-hidden="true" />
        </Button>
      </header>

      {pending && (
        <div className="mt-3 flex items-center gap-2 text-sm text-muted-foreground">
          <Spinner className="h-4 w-4" />
          {t('diagnose.asking')}
        </div>
      )}

      {!pending && result && !result.available && (
        <p className="mt-3 text-sm text-muted-foreground">{t('diagnose.unavailable')}</p>
      )}

      {!pending && result?.diagnosis && (
        <>
          {/* whitespace-pre-line keeps the model's paragraph breaks without
              rendering its output as markup. */}
          <p className="mt-3 text-sm whitespace-pre-line text-foreground">{result.diagnosis}</p>
          <p className="mt-3 text-xs text-muted-foreground">{t('diagnose.disclaimer')}</p>
          <p className="mt-1 text-xs text-muted-foreground">{t('diagnose.sent')}</p>
        </>
      )}
    </section>
  );
}
