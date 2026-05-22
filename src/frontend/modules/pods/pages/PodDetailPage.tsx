// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useParams } from 'react-router';
import { useTranslation } from 'react-i18next';
import { IconBox } from '@tabler/icons-react';
import { PageHeader } from '@resources/layout/PageHeader';
import { Badge } from '@resources/components/ui/Badge';
import { Card } from '@resources/components/ui/Card';
import { Spinner } from '@resources/components/ui/Spinner';
import { usePod } from '../hooks/usePods';

export default function PodDetailPage() {
  const { t } = useTranslation('kubernetes');
  const { namespace = '', name = '' } = useParams<{ namespace: string; name: string }>();
  const { data: pod, isLoading } = usePod(namespace, name);

  if (isLoading) {
    return (
      <div className="flex h-64 items-center justify-center">
        <Spinner size="lg" />
      </div>
    );
  }

  if (!pod) {
    return <div className="text-muted-foreground">{t('pods.detail.notFound')}</div>;
  }

  return (
    <div className="space-y-6">
      <PageHeader title={pod.name} description={`${pod.namespace} / ${pod.status}`} />

      <div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-4">
        <Card className="p-4">
          <div className="text-xs text-muted-foreground">{t('pods.detail.status')}</div>
          <div className="mt-1 text-lg font-semibold">{pod.status}</div>
        </Card>
        <Card className="p-4">
          <div className="text-xs text-muted-foreground">{t('pods.detail.ready')}</div>
          <div className="mt-1 text-lg font-semibold">{pod.ready}</div>
        </Card>
        <Card className="p-4">
          <div className="text-xs text-muted-foreground">{t('pods.detail.node')}</div>
          <div className="mt-1 text-lg font-semibold">{pod.node || '-'}</div>
        </Card>
        <Card className="p-4">
          <div className="text-xs text-muted-foreground">{t('pods.detail.ip')}</div>
          <div className="mt-1 text-lg font-semibold font-mono">{pod.ip || '-'}</div>
        </Card>
      </div>

      <Card className="p-4">
        <h3 className="mb-3 text-sm font-semibold">{t('pods.detail.containers')}</h3>
        <div className="space-y-2">
          {pod.containers?.map((c) => (
            <div key={c.name} className="flex items-center justify-between rounded-md border border-border p-3">
              <div className="flex items-center gap-2">
                <IconBox className="h-4 w-4 text-muted-foreground" />
                <span className="font-medium">{c.name}</span>
                <span className="text-xs text-muted-foreground">{c.image}</span>
              </div>
              <div className="flex items-center gap-2">
                <Badge variant={c.ready ? 'success' : 'destructive'}>{c.state}</Badge>
                {c.restartCount > 0 && (
                  <span className="text-xs text-muted-foreground">
                    {t('pods.detail.restartCount', { count: c.restartCount })}
                  </span>
                )}
              </div>
            </div>
          ))}
        </div>
      </Card>
    </div>
  );
}
