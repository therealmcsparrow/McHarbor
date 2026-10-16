// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useParams } from 'react-router';
import { useTranslation } from 'react-i18next';
import { IconRefresh } from '@tabler/icons-react';
import { Alert } from '@resources/components/ui/Alert';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import { Spinner } from '@resources/components/ui/Spinner';
import { PageBody } from '@resources/layout/PageBody';
import { PageHeader } from '@resources/layout/PageHeader';
import { PERM } from '@core/auth/permissionKeys';
import { formatDate } from '@resources/utils/format';
import { useIncident, useRefreshEvidence, useResolveIncident } from '@resources/hooks/useIncidents';
import { useIncidentLabels } from '../hooks/useIncidentLabels';
import { NarrativePanel } from '../components/NarrativePanel';
import { EvidenceView } from '../components/EvidenceView';
import { TimelinePanel } from '../components/TimelinePanel';

export default function IncidentDetailPage() {
  const { t } = useTranslation('incidents');
  const labels = useIncidentLabels();
  const { id } = useParams();
  const incident = useIncident(id);
  const refresh = useRefreshEvidence();
  const resolve = useResolveIncident();
  const [confirming, setConfirming] = useState(false);
  const data = incident.data;

  if (incident.isLoading) return <Spinner />;
  if (!data) return <Alert variant="destructive">{t('notFound')}</Alert>;

  return (
    <div className="flex h-full min-h-0 flex-col">
      <PageHeader
        title={data.subjectName}
        breadcrumbs={[{ label: t('title'), to: '/incidents' }, { label: data.subjectName }]}
        description={t('detail.description', {
          signal: labels.signal(data),
          count: data.signalCount,
          first: formatDate(data.firstSignalAt),
          last: formatDate(data.lastSignalAt),
        })}
        actions={
          <div className="flex items-center gap-2">
            <Badge variant={data.status === 'open' ? 'warning' : 'success'}>{t(`status.${data.status}`)}</Badge>
            <Button
              variant="outline"
              permission={PERM.INCIDENTS_MANAGE}
              disabled={refresh.isPending}
              onClick={() => !refresh.isPending && refresh.mutate(data.id)}
            >
              <IconRefresh className="size-4" /> {t('detail.refresh')}
            </Button>
            {data.status === 'open' && (
              <Button permission={PERM.INCIDENTS_MANAGE} onClick={() => setConfirming(true)}>
                {t('detail.resolve')}
              </Button>
            )}
          </div>
        }
      />
      <PageBody className="gap-4">
        <NarrativePanel incident={data} />
        {data.status === 'resolved' && <TimelinePanel incident={data} />}
        {data.evidence ? (
          <EvidenceView evidence={data.evidence} />
        ) : (
          <Alert variant="info">{t('evidence.pending')}</Alert>
        )}
      </PageBody>
      <ConfirmDialog
        open={confirming}
        onOpenChange={(isOpen) => {
          if (isOpen) return;
          setConfirming(false);
          resolve.reset();
        }}
        title={t('detail.resolveTitle')}
        description={t('detail.resolveDescription', { name: data.subjectName })}
        confirmLabel={t('detail.resolveConfirm')}
        variant="default"
        loading={resolve.isPending}
        permission={PERM.INCIDENTS_MANAGE}
        onConfirm={() => !resolve.isPending && resolve.mutate(data.id, { onSuccess: () => setConfirming(false) })}
      >
        {resolve.error && <Alert variant="destructive">{resolve.error.message}</Alert>}
      </ConfirmDialog>
    </div>
  );
}
