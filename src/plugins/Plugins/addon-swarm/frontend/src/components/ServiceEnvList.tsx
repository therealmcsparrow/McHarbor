// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconEye, IconEyeOff } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { isRedactedValue } from '@resources/utils/env-redaction';
import { StringList } from './DetailTabs';
import { useSwarmServiceEnv } from '../hooks/useSwarmServices';
import { PERM } from '../permissions';

function envValue(entry: string): string {
  const idx = entry.indexOf('=');
  return idx > -1 ? entry.slice(idx + 1) : '';
}

export function ServiceEnvList({ serviceId, env }: { serviceId: string; env: string[] | null | undefined }) {
  const { t } = useTranslation('swarm');
  const [revealed, setRevealed] = useState(false);
  const revealQuery = useSwarmServiceEnv(serviceId, revealed);
  const hasMasked = (env ?? []).some((entry) => isRedactedValue(envValue(entry)));
  const shown = revealed && revealQuery.data ? revealQuery.data : env;

  return (
    <div className="space-y-2">
      {(hasMasked || revealed) && (
        <div className="flex items-center justify-between gap-3">
          <span className="text-xs text-muted-foreground">{t('detail.env.withheld')}</span>
          <Button
            variant="ghost"
            size="sm"
            permission={PERM.SWARM_SERVICES_MANAGE}
            disabled={revealed && revealQuery.isFetching}
            onClick={() => setRevealed((value) => !value)}
          >
            {revealed ? <IconEyeOff className="h-3.5 w-3.5" /> : <IconEye className="h-3.5 w-3.5" />}
            {revealed ? t('detail.env.hide') : t('detail.env.reveal')}
          </Button>
        </div>
      )}
      <StringList items={shown} />
    </div>
  );
}
