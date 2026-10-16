// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconTrash } from '@tabler/icons-react';
import { PERM } from '@core/auth/permissionKeys';
import { Alert } from '@resources/components/ui/Alert';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { NumberInput } from '@resources/components/ui/NumberInput';
import { Select } from '@resources/components/ui/Select';
import { useLocaleFormat } from '@resources/hooks/useLocaleFormat';
import { formatDate } from '@resources/utils/format';
import { toNumberOrNull, toNumberText } from '@resources/utils/numberText';
import type { EnvironmentAccessState } from '../hooks/useEnvironmentAccess';

type EnvironmentAccessHostCardProps = {
  state: EnvironmentAccessState;
};

export function EnvironmentAccessHostCard({ state }: EnvironmentAccessHostCardProps) {
  const { t } = useTranslation('environments');
  const formatPrefs = useLocaleFormat();
  const { settings, forget, fields } = state;

  return (
    <div className="mc-glass rounded-lg p-6">
      <div className="mb-4 flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <h3 className="text-sm font-medium text-foreground">{t('detail.access.title')}</h3>
          <p className="mt-0.5 text-xs text-muted-foreground">{t('detail.access.description')}</p>
        </div>
        {settings?.configured ? (
          <Button
            size="sm"
            variant="outline"
            permission={PERM.ENVIRONMENTS_MANAGE}
            disabled={forget.isPending}
            onClick={() => forget.mutate()}
          >
            <IconTrash className="mr-1.5 h-4 w-4" aria-hidden="true" />
            {t('detail.access.forget')}
          </Button>
        ) : null}
      </div>

      {settings?.configured ? (
        <div className="mb-4 flex flex-wrap items-center gap-2">
          <Badge variant="success">{t('detail.access.configured')}</Badge>
          {settings.secretName ? <Badge variant="outline">{settings.secretName}</Badge> : null}
          {settings.lastAt ? (
            <span className="text-xs text-muted-foreground">
              {t('detail.access.lastDeployed', {
                when: formatDate(settings.lastAt, formatPrefs),
              })}
            </span>
          ) : null}
        </div>
      ) : (
        <Alert variant="info" className="mb-4" description={t('detail.access.notConfigured')} />
      )}

      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
        <div>
          <Label className="mb-2">{t('detail.access.host')}</Label>
          <Input value={fields.host} onChange={(e) => fields.setHost(e.target.value)} />
        </div>
        <div>
          <Label className="mb-2">{t('detail.access.port')}</Label>
          <NumberInput
            nullable
            aria-label={t('detail.access.port')}
            value={toNumberOrNull(fields.port)}
            onChange={(next) => fields.setPort(toNumberText(next))}
          />
        </div>
        <div>
          <Label className="mb-2">{t('detail.access.user')}</Label>
          <Input value={fields.user} onChange={(e) => fields.setUser(e.target.value)} />
        </div>
        <div>
          <Label className="mb-2">{t('detail.access.method')}</Label>
          <Select
            value={fields.method}
            onChange={fields.setMethod}
            ariaLabel={t('detail.access.method')}
            options={[
              { value: 'binary', label: t('detail.access.methodBinary') },
              { value: 'docker', label: t('detail.access.methodDocker') },
            ]}
          />
        </div>
        <div className="sm:col-span-2">
          <Label className="mb-2">{t('detail.access.fingerprint')}</Label>
          <Input
            value={fields.fingerprint}
            onChange={(e) => fields.setFingerprint(e.target.value)}
            className="font-mono text-xs"
          />
          <p className="mt-1 text-xs text-muted-foreground">{t('detail.access.fingerprintHelp')}</p>
        </div>
      </div>
    </div>
  );
}
