// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { Card } from '@resources/components/ui/Card';
import { PERM } from '@core/auth/permissionKeys';
import { formatDate } from '@resources/utils/format';
import { type Incident, useSummarise } from '@resources/hooks/useIncidents';
import { RefusalAlert } from './RefusalAlert';

const KIND_VARIANT: Record<string, 'destructive' | 'secondary' | 'info' | 'warning' | 'success' | 'default'> = {
  symptom: 'destructive',
  event: 'secondary',
  action: 'info',
  alert: 'warning',
  change: 'default',
  resolved: 'success',
};

export function TimelinePanel({ incident }: { incident: Incident }) {
  const { t } = useTranslation('incidents');
  const summarise = useSummarise();
  const items = incident.timeline ?? [];

  return (
    <Card className="space-y-4 p-5">
      <div className="flex items-center gap-3">
        <h2 className="text-base font-semibold text-foreground">{t('timeline.title')}</h2>
        <Button
          className="ml-auto"
          variant="outline"
          permission={PERM.INCIDENTS_MANAGE}
          disabled={summarise.isPending}
          onClick={() => !summarise.isPending && summarise.mutate(incident.id)}
        >
          {summarise.isPending ? t('narrative.working') : t('timeline.summarise')}
        </Button>
      </div>
      {summarise.error && <RefusalAlert error={summarise.error} envId={incident.envId} />}
      {incident.summary && <p className="text-sm whitespace-pre-wrap text-foreground">{incident.summary}</p>}
      <ol className="space-y-1.5">
        {items.map((it) => (
          <li key={`${it.at}-${it.kind}-${it.detail}`} className="flex flex-wrap items-center gap-2 text-sm">
            <span className="w-40 shrink-0 text-xs text-muted-foreground">{formatDate(it.at)}</span>
            <Badge variant={KIND_VARIANT[it.kind] ?? 'secondary'}>{t(`timeline.kind.${it.kind}`)}</Badge>
            {it.who && <span className="font-medium">{it.who}</span>}
            <span className="break-all">{it.kind === 'symptom' ? t(`signal.${it.detail}`) : it.detail}</span>
          </li>
        ))}
      </ol>
    </Card>
  );
}
