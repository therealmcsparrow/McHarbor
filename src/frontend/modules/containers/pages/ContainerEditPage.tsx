// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
// i18n keys used by this file:
//   containers:configEdit.{crumb,badge,backToDetails,notFound,
//   protected.{unlock,lock,locked,unlocked,description},
//   unlockConfirm.{title,body,confirm}}

import { useCallback, useEffect, useState } from 'react';
import { useNavigate, useParams } from 'react-router';
import { useTranslation } from 'react-i18next';
import { IconAlertTriangle, IconShield } from '@tabler/icons-react';
import { CardSkeleton } from '@resources/components/ui/Skeleton';
import { ErrorState } from '@resources/components/ui/ErrorState';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import { HeaderBreadcrumb } from '@resources/layout/HeaderBreadcrumb';
import { useHeaderSlot } from '@resources/stores/headerSlot';
import { isMcHarborContainer, isProtectedContainer } from '@core/utils/protection';
import { useAIContext } from '@resources/ai';
import { useContainer } from '../hooks/useContainers';
import { ContainerConfigEditShell } from '../components/ContainerConfigEditShell';
import { useContainerConfig } from '../hooks/useContainerConfig';
import { ContainerEditHeader } from './ContainerEditHeader';
import { Box } from '@resources/components/ui/Layout';
import { Text as TypographyText } from '@resources/components/ui/Typography';

/**
 * ContainerEditPage - a dedicated, full-page edit surface for a
 * single container. It mounts the shared `ContainerConfigEditShell`
 * (the settings-style chrome over the per-tab form components) so the
 * focused "edit config" experience matches the look + feel of the
 * other settings tabs.
 *
 * Chrome-wise it follows `ContainerDetailPage` exactly: the breadcrumb
 * goes to the topbar (`HeaderBreadcrumb`), identity and actions sit in
 * the page-head band (`ContainerEditHeader`), and the route turns the
 * shell's padding off so the page supplies its own `p-6`. It used to hand-roll a
 * full-bleed `border-b` strip with its own text breadcrumb instead,
 * which is why it read as a different application than the pages
 * either side of it.
 *
 * The in-place edit flow on `ContainerDetailPage` is unchanged — this
 * page is an alternative reach for users who want a focused editor
 * without the noise of the detail page's other tabs.
 */
export default function ContainerEditPage() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const { t } = useTranslation('containers');
  const { data: container, isLoading, isError, error, refetch } = useContainer(id ?? '');
  const [editUnlocked, setEditUnlocked] = useState(false);
  const [unlockConfirmOpen, setUnlockConfirmOpen] = useState(false);
  const setHeaderActive = useHeaderSlot((store) => store.setActive);
  const unlockProtected = editUnlocked && container !== undefined && isMcHarborContainer(container);
  // Held here rather than in the form shell: Save and Cancel sit in the page
  // header, which renders beside the shell, not inside it.
  const config = useContainerConfig(container, { unlockProtected });

  const handleBack = useCallback(() => {
    if (id) void navigate(`/containers/${id}`);
    else void navigate('/containers');
  }, [id, navigate]);

  const handleCancel = useCallback(() => {
    config.cancel();
    handleBack();
  }, [config, handleBack]);

  // Activate the page header slot so the AppLayout can render
  // context-aware chrome (breadcrumbs, top-bar actions).
  useEffect(() => {
    setHeaderActive(true);
    return () => setHeaderActive(false);
  }, [setHeaderActive]);

  // Publish the focused container to the central AI context registry.
  useAIContext(
    container && id
      ? {
          source: 'container-edit',
          type: 'container',
          resourceId: container.Id,
          resourceName: (container.Name ?? '').replace(/^\//, ''),
          priority: 8,
          metadata: {
            image: container.Config?.Image,
            state: container.State?.Status,
          },
        }
      : null,
  );

  if (isLoading) {
    return (
      <Box className="flex h-full min-h-0 flex-col gap-4 p-6">
        <CardSkeleton lines={2} className="min-h-24" />
        <CardSkeleton lines={8} className="min-h-64" />
      </Box>
    );
  }

  if (isError) {
    return (
      <ErrorState
        variant="page"
        title={t('configEdit.notFound')}
        error={error}
        onRetry={() => {
          void refetch();
        }}
      />
    );
  }

  if (!container) {
    return (
      <ErrorState
        variant="page"
        title={t('containerNotFound')}
        primaryAction={{
          label: t('configEdit.backToDetails'),
          onClick: handleBack,
        }}
      />
    );
  }

  const containerName = (container.Name ?? '').replace(/^\//, '');
  const isProtected = isProtectedContainer(container);
  const isMcHarborTarget = isMcHarborContainer(container);
  const editLocked = isProtected && !(isMcHarborTarget && editUnlocked);

  return (
    /* The route sets `padded: false` so the page-head band runs edge to
       edge; the padding comes back here, exactly as on the detail page. */
    <div className="flex h-full min-h-0 flex-col gap-4 p-6">
      {/* Orientation goes in the topbar, where it keeps the same coordinates
          on every page and does not scroll away underneath you. */}
      <HeaderBreadcrumb
        crumbs={[
          { label: t('title'), to: '/containers' },
          { label: containerName, to: `/containers/${container.Id}` },
          { label: t('configEdit.crumb') },
        ]}
      />

      <ContainerEditHeader
        container={container}
        isProtected={isProtected}
        canUnlock={isProtected && isMcHarborTarget}
        editLocked={editLocked}
        editUnlocked={editUnlocked}
        onBack={handleBack}
        editing={config.editing}
        dirty={config.dirty}
        saving={config.isSaving}
        onSave={() => void config.save()}
        onCancel={handleCancel}
        onToggleLock={() => {
          if (editUnlocked) {
            setEditUnlocked(false);
          } else {
            setUnlockConfirmOpen(true);
          }
        }}
      />

      {isProtected && editLocked && (
        <div className="flex flex-none items-start gap-2 rounded-lg border border-warning/30 bg-warning/10 px-3 py-2 text-warning">
          <IconShield className="mt-0.5 size-4 shrink-0" />
          <TypographyText size="xs">{t('configEdit.protected.description')}</TypographyText>
        </div>
      )}

      {config.changes.hasConfigChanges && (
        <div className="flex flex-none items-center gap-1.5 text-xs text-warning">
          <IconAlertTriangle className="size-3.5" />
          {t('configEdit.validationWarning')}
        </div>
      )}

      <Box className="flex min-h-0 flex-1 flex-col overflow-hidden">
        <ContainerConfigEditShell container={container} config={config} autoStart={!editLocked} />
      </Box>

      <ConfirmDialog
        open={unlockConfirmOpen}
        onOpenChange={setUnlockConfirmOpen}
        variant="caution"
        title={t('configEdit.unlockConfirm.title')}
        description={t('configEdit.unlockConfirm.body')}
        confirmLabel={t('configEdit.unlockConfirm.confirm')}
        onConfirm={() => {
          setEditUnlocked(true);
          setUnlockConfirmOpen(false);
        }}
      />
    </div>
  );
}
