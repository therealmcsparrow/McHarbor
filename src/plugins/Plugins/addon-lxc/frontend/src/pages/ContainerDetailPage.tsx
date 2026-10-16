// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconBox } from '@tabler/icons-react';
import { PageHeadActions, PageHeadIdentity } from '@resources/layout/PageHeadIdentity';
import { Badge } from '@resources/components/ui/Badge';
import { ErrorState } from '@resources/components/ui/ErrorState';
import { Skeleton } from '@resources/components/ui/Skeleton';
import { Card } from '@resources/components/ui/Card';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@resources/components/ui/Tabs';
import { Tooltip, TooltipContent, TooltipTrigger } from '@resources/components/ui/Tooltip';
import { useContainer, useDiagnose, useLifecycle } from '../hooks/useContainers';
import { IsolationBadge, StateBadge } from '../components/StateBadge';
import { ContainerConsole } from '../components/ContainerConsole';
import { ResourcesPanel } from '../components/ResourcesPanel';
import { MountsPanel } from '../components/MountsPanel';
import { ProcessesPanel } from '../components/ProcessesPanel';
import { SnapshotsPanel } from '../components/SnapshotsPanel';
import { BackupsPanel } from '../components/BackupsPanel';
import { CloneContainerDialog } from '../components/CloneContainerDialog';
import { DeleteContainerDialog } from '../components/DeleteContainerDialog';
import { DiagnosePanel } from '../components/DiagnosePanel';
import { ContainerHeaderActions } from '../components/ContainerHeaderActions';
import { ContainerOverviewTab } from '../components/ContainerOverviewTab';
import { isLive } from '../types';

/** One container, with its console, resources, mounts, processes and snapshots. */
export function ContainerDetailPage({ id, onBack }: { id: string; onBack: () => void }) {
  const { t } = useTranslation('ct');
  const { data: container, isLoading, isError, refetch } = useContainer(id);
  const lifecycle = useLifecycle();
  const diagnose = useDiagnose();
  const [deleting, setDeleting] = useState(false);
  const [cloning, setCloning] = useState(false);

  if (isLoading) return <Skeleton className="h-96 w-full" />;
  if (isError || !container) return <ErrorState onRetry={() => void refetch()} />;

  const live = isLive(container);
  const disabled = lifecycle.isPending || !container.hostAvailable;

  return (
    <div className="space-y-4">
      <PageHeadIdentity>
        <span
          aria-hidden="true"
          className="grid size-11 shrink-0 place-items-center rounded-xl bg-primary/12 ring-1 ring-primary/25 ring-inset"
        >
          <IconBox className="size-6 text-primary" />
        </span>
        <div className="min-w-0">
          <h1 className="truncate font-display text-2xl leading-tight font-semibold tracking-tight text-foreground">
            {container.name}
          </h1>
          <div className="mt-1 flex flex-wrap items-center gap-1.5">
            <StateBadge container={container} />
            <IsolationBadge container={container} />
            {container.isTemplate && <Badge variant="secondary">{t('nav.templates')}</Badge>}
            {container.nesting && (
              <Tooltip>
                <TooltipTrigger asChild>
                  <Badge variant="warning">{t('isolation.nesting')}</Badge>
                </TooltipTrigger>
                <TooltipContent>{t('isolation.nestingHint')}</TooltipContent>
              </Tooltip>
            )}
            {container.description && (
              <span className="truncate text-xs text-muted-foreground">{container.description}</span>
            )}
          </div>
        </div>
      </PageHeadIdentity>

      <PageHeadActions>
        <ContainerHeaderActions
          container={container}
          live={live}
          disabled={disabled}
          diagnose={diagnose}
          lifecycle={lifecycle}
          onBack={onBack}
          onClone={() => setCloning(true)}
          onDelete={() => setDeleting(true)}
        />
      </PageHeadActions>

      {!container.hostAvailable && (
        <p className="rounded-md bg-warning/10 p-3 text-sm text-foreground">{t('state.unknownHint')}</p>
      )}

      {(diagnose.isPending || diagnose.data) && (
        <DiagnosePanel pending={diagnose.isPending} result={diagnose.data} onDismiss={() => diagnose.reset()} />
      )}

      <Tabs defaultValue="overview">
        <Card className="flex-none overflow-visible px-5 py-3">
          <TabsList>
            <TabsTrigger value="overview">{t('detail.tabs.overview')}</TabsTrigger>
            <TabsTrigger value="console">{t('detail.tabs.console')}</TabsTrigger>
            <TabsTrigger value="resources">{t('detail.tabs.resources')}</TabsTrigger>
            <TabsTrigger value="mounts">{t('detail.tabs.mounts')}</TabsTrigger>
            <TabsTrigger value="processes">{t('detail.tabs.processes')}</TabsTrigger>
            <TabsTrigger value="snapshots">{t('detail.tabs.snapshots')}</TabsTrigger>
            <TabsTrigger value="backups">{t('detail.tabs.backups')}</TabsTrigger>
          </TabsList>
        </Card>

        <TabsContent value="overview">
          <ContainerOverviewTab container={container} />
        </TabsContent>

        <TabsContent value="console">
          <ContainerConsole container={container} />
        </TabsContent>

        <TabsContent value="resources">
          <ResourcesPanel container={container} />
        </TabsContent>

        <TabsContent value="mounts">
          <MountsPanel container={container} />
        </TabsContent>

        <TabsContent value="processes">
          <ProcessesPanel container={container} />
        </TabsContent>

        <TabsContent value="snapshots">
          <SnapshotsPanel container={container} />
        </TabsContent>

        <TabsContent value="backups">
          <BackupsPanel container={container} />
        </TabsContent>
      </Tabs>

      {cloning && <CloneContainerDialog container={container} onClose={() => setCloning(false)} />}

      {deleting && (
        <DeleteContainerDialog container={container} onClose={() => setDeleting(false)} onDeleted={onBack} />
      )}
    </div>
  );
}
