// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconEye, IconEyeOff } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { PERM } from '@core/auth/permissionKeys';
import { isRedactedValue } from '@resources/utils/env-redaction';
import { useImageEnvRevealed } from '@resources/hooks/useImages';
import { ImageKeyValueRow } from './ImageKeyValueRow';

type ImageEnvListProps = {
  imageId: string;
  env: string[] | null | undefined;
};

function splitEntry(entry: string): { key: string; value: string } {
  const separator = entry.indexOf('=');
  return separator > -1
    ? { key: entry.slice(0, separator), value: entry.slice(separator + 1) }
    : { key: entry, value: '' };
}

export function ImageEnvList({ imageId, env }: ImageEnvListProps) {
  const { t } = useTranslation('images');
  const [revealed, setRevealed] = useState(false);
  const revealQuery = useImageEnvRevealed(imageId, revealed);
  const entries = (env ?? []).map(splitEntry);
  const hasRedacted = entries.some((entry) => isRedactedValue(entry.value));
  // A value holding one of McHarbor's own keys stays the marker even when revealed.
  const shown = revealed && revealQuery.data ? revealQuery.data.map(splitEntry) : entries;

  if (entries.length === 0) {
    return <p className="text-sm text-muted-foreground">{t('detail.noEnvVars')}</p>;
  }

  return (
    <div>
      {(hasRedacted || revealed) && (
        <div className="mb-2 flex items-center justify-between gap-3">
          <p className="text-xs text-muted-foreground">{t('detail.envWithheld')}</p>
          <Button
            variant="ghost"
            size="sm"
            permission={PERM.IMAGES_MANAGE}
            disabled={revealed && revealQuery.isFetching}
            onClick={() => setRevealed((value) => !value)}
          >
            {revealed ? <IconEyeOff className="h-3.5 w-3.5" /> : <IconEye className="h-3.5 w-3.5" />}
            {revealed ? t('detail.hideValues') : t('detail.revealValues')}
          </Button>
        </div>
      )}
      {shown.map((entry) => (
        <ImageKeyValueRow key={`${entry.key}=${entry.value}`} itemKey={entry.key} value={entry.value} />
      ))}
    </div>
  );
}
