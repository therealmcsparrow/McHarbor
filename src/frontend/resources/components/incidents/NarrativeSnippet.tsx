// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { Link } from 'react-router';
import { useTranslation } from 'react-i18next';
import { IconSparkles } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { PERM } from '@core/auth/permissionKeys';
import { usePermission } from '@core/auth/usePermission';
import { useIncidentForAlert } from '@resources/hooks/useIncidents';

export function NarrativeSnippet({ alertEventId }: { alertEventId: string }) {
  const { t } = useTranslation('incidents');
  const allowed = usePermission(PERM.INCIDENTS_VIEW);
  const incident = useIncidentForAlert(alertEventId, allowed);
  const data = incident.data;
  if (!allowed || !data) return null;
  const n = data.narrative;

  return (
    <div className="mt-2 rounded-md border border-border bg-muted/40 p-2 text-sm">
      <div className="flex items-center gap-2">
        <IconSparkles className="size-4 text-primary" aria-hidden="true" />
        <span className="font-medium">{t('snippet.title')}</span>
        {n && <Badge variant="secondary">{t(`confidence.${n.confidence}`)}</Badge>}
      </div>
      <p className="mt-1">{n ? n.summary : t('snippet.pending')}</p>
      <Link className="text-xs text-primary underline" to={`/incidents/${encodeURIComponent(data.id)}`}>
        {t('snippet.open')}
      </Link>
    </div>
  );
}
