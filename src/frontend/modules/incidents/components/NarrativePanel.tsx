// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconSparkles } from '@tabler/icons-react';
import { Alert } from '@resources/components/ui/Alert';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { Card } from '@resources/components/ui/Card';
import { Disclosure } from '@resources/components/ui/Disclosure';
import { StatusError } from '@resources/components/ui/StatusError';
import { PERM } from '@core/auth/permissionKeys';
import { formatDate } from '@resources/utils/format';
import { type Confidence, type Incident, useNarrate } from '@resources/hooks/useIncidents';
import { RefusalAlert } from './RefusalAlert';

const CONFIDENCE_VARIANT: Record<Confidence, 'secondary' | 'warning' | 'success'> = {
  low: 'secondary',
  medium: 'warning',
  high: 'success',
};

export function NarrativePanel({ incident }: { incident: Incident }) {
  const { t } = useTranslation('incidents');
  const narrate = useNarrate();
  const n = incident.narrative;
  const meta = incident.narrativeMeta;
  const pending = narrate.isPending || incident.narrativeStatus === 'pending';

  return (
    <Card className="space-y-4 p-5">
      <div className="flex flex-wrap items-center gap-3">
        <h2 className="text-base font-semibold text-foreground">{t('narrative.title')}</h2>
        {n && <Badge variant={CONFIDENCE_VARIANT[n.confidence]}>{t(`confidence.${n.confidence}`)}</Badge>}
        <Button
          className="ml-auto"
          permission={PERM.INCIDENTS_MANAGE}
          disabled={pending}
          onClick={() => !pending && narrate.mutate(incident.id)}
        >
          <IconSparkles className="size-4" />
          {pending ? t('narrative.working') : n ? t('narrative.again') : t('narrative.explain')}
        </Button>
      </div>
      {narrate.error && <RefusalAlert error={narrate.error} envId={incident.envId} />}
      {incident.narrativeStatus === 'failed' && !narrate.error && (
        <p className="flex items-center gap-2 text-sm text-muted-foreground">
          {t('narrative.lastFailed')}
          <StatusError error={incident.narrativeError} title={t('narrative.failedTitle')} />
        </p>
      )}
      {!n && !pending && incident.narrativeStatus !== 'failed' && (
        <p className="text-sm text-muted-foreground">{t('narrative.none')}</p>
      )}
      {n && (
        <div className="space-y-3">
          {n.undetermined && <Alert variant="warning">{t('narrative.undetermined')}</Alert>}
          <p className="text-sm text-foreground">{n.summary}</p>
          {n.changes.length > 0 && (
            <div>
              <h3 className="text-xs font-semibold text-muted-foreground uppercase">{t('narrative.changes')}</h3>
              <ul className="mt-1 list-disc pl-5 text-sm">
                {n.changes.map((c) => (
                  <li key={c}>{c}</li>
                ))}
              </ul>
            </div>
          )}
          {n.causes.length > 0 && (
            <div>
              <h3 className="text-xs font-semibold text-muted-foreground uppercase">{t('narrative.causes')}</h3>
              <ol className="mt-1 space-y-2">
                {n.causes.map((c) => (
                  <li key={c.title} className="rounded-md border border-border p-3">
                    <div className="flex items-center gap-2">
                      <span className="font-medium">{c.title}</span>
                      <Badge variant={CONFIDENCE_VARIANT[c.confidence]}>{t(`confidence.${c.confidence}`)}</Badge>
                    </div>
                    <ul className="mt-1 list-disc pl-5 font-mono text-xs break-all text-muted-foreground">
                      {c.evidence.map((e) => (
                        <li key={e}>{e}</li>
                      ))}
                    </ul>
                  </li>
                ))}
              </ol>
            </div>
          )}
          {meta && (
            <p className="text-xs text-muted-foreground">
              {t(meta.proactive ? 'narrative.attributionProactive' : 'narrative.attribution', {
                model: meta.model,
                connection: meta.connection,
                provider: meta.provider,
                who: meta.requestedBy,
                time: formatDate(meta.at),
              })}
            </p>
          )}
          {incident.narrativePrompt && (
            <Disclosure
              title={t('narrative.shown')}
              description={t('narrative.shownHint', { hash: meta?.promptSha256.slice(0, 12) ?? '' })}
            >
              <pre className="max-h-96 overflow-auto rounded-md bg-muted p-3 font-mono text-xs break-all whitespace-pre-wrap">
                {incident.narrativePrompt}
              </pre>
            </Disclosure>
          )}
        </div>
      )}
    </Card>
  );
}
