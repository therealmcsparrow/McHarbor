// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useParams } from 'react-router';
import { useTranslation } from 'react-i18next';
import { IconRefresh, IconArrowsUpDown } from '@tabler/icons-react';
import { PageHeader } from '@resources/layout/PageHeader';
import { Card } from '@resources/components/ui/Card';
import { Button } from '@resources/components/ui/Button';
import { Badge } from '@resources/components/ui/Badge';
import { Spinner } from '@resources/components/ui/Spinner';
import { useDeployment, useRestartDeployment } from '../hooks/useDeployments';

export default function DeploymentDetailPage() {
  const { t } = useTranslation('kubernetes');
  const { t: tc } = useTranslation('common');
  const { namespace = '', name = '' } = useParams<{ namespace: string; name: string }>();
  const { data: deployment, isLoading } = useDeployment(namespace, name);
  const restartMutation = useRestartDeployment();

  if (isLoading) {
    return (
      <div className="flex h-64 items-center justify-center">
        <Spinner size="lg" />
      </div>
    );
  }

  if (!deployment) {
    return <div className="text-muted-foreground">{t('deployments.detail.notFound')}</div>;
  }

  return (
    <div className="space-y-6">
      <PageHeader
        title={deployment.name}
        description={`${deployment.namespace} / ${deployment.strategy}`}
        actions={
          <Button
            variant="outline"
            onClick={() => restartMutation.mutate({ namespace, name })}
            disabled={restartMutation.isPending}
          >
            <IconRefresh className="h-4 w-4" /> {tc('actions.restart')}
          </Button>
        }
      />

      <div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-4">
        <Card className="p-4">
          <div className="text-xs text-muted-foreground">{t('deployments.detail.ready')}</div>
          <div className="mt-1 text-lg font-semibold">{deployment.ready}</div>
        </Card>
        <Card className="p-4">
          <div className="text-xs text-muted-foreground">{t('deployments.detail.upToDate')}</div>
          <div className="mt-1 text-lg font-semibold">{deployment.upToDate}</div>
        </Card>
        <Card className="p-4">
          <div className="text-xs text-muted-foreground">{t('deployments.detail.available')}</div>
          <div className="mt-1 text-lg font-semibold">{deployment.available}</div>
        </Card>
        <Card className="p-4">
          <div className="text-xs text-muted-foreground">{t('deployments.detail.strategy')}</div>
          <div className="mt-1 text-lg font-semibold">{deployment.strategy}</div>
        </Card>
      </div>

      <Card className="p-4">
        <h3 className="mb-3 text-sm font-semibold flex items-center gap-2">
          <IconArrowsUpDown className="h-4 w-4" /> {t('deployments.detail.conditions')}
        </h3>
        <div className="space-y-2">
          {deployment.conditions?.map((c) => (
            <div key={c.type} className="flex items-center justify-between rounded-md border border-border p-3">
              <span className="font-medium">{c.type}</span>
              <div className="flex items-center gap-2">
                <Badge variant={c.status === 'True' ? 'success' : 'secondary'}>{c.status}</Badge>
                {c.message && (
                  <span className="text-xs text-muted-foreground">{c.message}</span>
                )}
              </div>
            </div>
          ))}
        </div>
      </Card>
    </div>
  );
}

