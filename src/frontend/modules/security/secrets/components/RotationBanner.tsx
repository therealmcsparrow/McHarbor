// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconX } from '@tabler/icons-react';
import { Alert } from '@resources/components/ui/Alert';
import { Button } from '@resources/components/ui/Button';
import { useSecretImpact } from '@resources/hooks/useSecrets';
import type { Secret } from '../types';

type RotationBannerProps = {
  secret: Secret;
  onDismiss: () => void;
  onReview: () => void;
};

export function RotationBanner({ secret, onDismiss, onReview }: RotationBannerProps) {
  const { t } = useTranslation('secrets');
  const { data: impact } = useSecretImpact(secret.id);

  const staleCount = impact?.staleCount ?? 0;
  if (staleCount === 0) return null;

  return (
    <Alert variant="warning" title={t('rotation.title', { name: secret.name })}>
      <div className="space-y-2">
        <p>
          {impact?.autoApply
            ? t('rotation.autoBody', { count: staleCount })
            : t('rotation.body', { count: staleCount })}
        </p>
        <div className="flex flex-wrap items-center gap-2">
          <Button size="sm" variant="outline" onClick={onReview}>
            {t('rotation.review')}
          </Button>
          <Button variant="ghost" size="icon-sm" aria-label={t('actions.close', { ns: 'common' })} onClick={onDismiss}>
            <IconX className="size-4" />
          </Button>
        </div>
      </div>
    </Alert>
  );
}
