// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// StackDetailPage — built to the container detail page's grammar.
//
// The seven tabs were already here; what was missing was the frame around
// them. The action header portalled into the topbar, so the page opened with
// no title and the global chrome changed shape per route, and the tab bar was
// a hand-rolled `bg-muted` pill nav that drew the same idea as `SubNav` with a
// different set of tokens.
//
// Now: breadcrumb in the topbar (with a leaf switcher over the other stacks),
// a hero card carrying identity, state and actions, the `SubNav` rail inside
// that card, and one concern at a time in the panel below.

import { useEffect, useMemo } from 'react';
import { useParams, useNavigate } from 'react-router';
import { useTranslation } from 'react-i18next';
import { Card } from '@resources/components/ui/Card';
import { CardSkeleton } from '@resources/components/ui/Skeleton';
import { ErrorState } from '@resources/components/ui/ErrorState';
import { HeaderBreadcrumb } from '@resources/layout/HeaderBreadcrumb';
import { PageBody } from '@resources/layout/PageBody';
import { cn } from '@resources/utils/cn';
import { useTabCommands } from '@resources/hooks/useTabCommands';
import { useTabContextMenu } from '@resources/hooks/useTabContextMenu';
import { useTabFromUrl } from '@resources/hooks/useTabFromUrl';
import { useHeaderSlot } from '@resources/stores/headerSlot';
import { isProtectedStack } from '@core/utils/protection';
import { useStack, useStackAction } from '@resources/hooks/useStacks';
import { StackDetailDialogs, useStackDialogState } from '../components/StackDetailDialogs';
import { useStackEditState } from '../hooks/useStackEditState';
import { useStackSiblings } from '../hooks/useStackSiblings';
import { StackDetailTabs } from '../components/StackDetailTabs';
import { StackDetailTabContent } from '../components/StackDetailTabContent';
import { DETAIL_TAB_IDS, DETAIL_TAB_ICONS, type DetailTabId } from '../components/stackDetailTabConfig';
import { StackDetailHeader } from './StackDetailHeader';

export default function StackDetailPage() {
  const { name: stackParam } = useParams<{ name: string }>();
  const navigate = useNavigate();
  const { t } = useTranslation('stacks');
  const { data: stack, isLoading } = useStack(stackParam ?? '');
  const action = useStackAction();
  const stackTabIds = useMemo(
    () => (stack?.type === 'managed' ? DETAIL_TAB_IDS : DETAIL_TAB_IDS.filter((id) => id !== 'environment')),
    [stack?.type],
  );
  const [activeTab, setActiveTab] = useTabFromUrl<DetailTabId>(stackTabIds, 'overview', {
    storageKey: `mcharbor-tabs:stack:${stackParam ?? 'unknown'}`,
  });
  const tabCommands = useMemo(
    () =>
      stackTabIds.map((tabId) => ({
        id: tabId,
        label: t(`detail.${tabId}`),
        icon: DETAIL_TAB_ICONS[tabId],
      })),
    [stackTabIds, t],
  );

  useTabCommands({
    pageId: `stack:${stackParam ?? 'unknown'}`,
    pageLabel: stack?.name ?? t('title'),
    tabs: tabCommands,
    onTabChange: setActiveTab,
    keywords: ['stack', stack?.name ?? stackParam ?? ''],
  });
  const tabContextMenuItems = useTabContextMenu({
    tabs: tabCommands,
    activeTab,
    onTabChange: setActiveTab,
  });
  const dialogs = useStackDialogState();
  const setHeaderActive = useHeaderSlot((s) => s.setActive);

  const { editing, editDescription, setEditDescription, handleStartEdit, handleCancelEdit, handleSave, saving } =
    useStackEditState(stack);

  const siblings = useStackSiblings();

  useEffect(() => {
    setHeaderActive(true);
    return () => setHeaderActive(false);
  }, [setHeaderActive]);

  if (isLoading) {
    return (
      <div className="flex h-full flex-col gap-4">
        <CardSkeleton lines={2} className="min-h-24" />
        <CardSkeleton lines={8} className="min-h-64" />
      </div>
    );
  }

  if (!stack) {
    return (
      <div>
        <ErrorState
          variant="page"
          title={t('detail.stackNotFound')}
          primaryAction={{
            label: t('detail.backToStacks'),
            onClick: () => navigate('/stacks'),
          }}
        />
      </div>
    );
  }

  const isRunning = stack.status === 'running' || stack.status === 'partial';
  const isManaged = stack.type === 'managed';
  const locked = isProtectedStack(stack);

  return (
    <div className="flex h-full min-h-0 flex-col">
      <HeaderBreadcrumb
        crumbs={[{ label: t('title'), to: '/stacks' }, { label: stack.name }]}
        siblings={siblings}
        currentId={stack.name}
      />

      <PageBody
        className={cn('gap-4 p-6', activeTab === 'logs' || activeTab === 'compose' ? 'overflow-hidden' : undefined)}
      >
        <Card className="flex-none overflow-visible">
          <StackDetailHeader
            stackName={stack.name}
            status={stack.status}
            isManaged={isManaged}
            isRunning={isRunning}
            serviceCount={stack.services?.length ?? 0}
            locked={locked}
            targetsSelf={stack.targetsSelf}
            editing={editing}
            onAction={(a) => {
              if (locked) return;
              if (a === 'down') dialogs.setDownOpen(true);
              else action.mutate({ stack, action: a });
            }}
            onRemove={() => !locked && dialogs.setRemoveOpen(true)}
            onTakeOver={() => !locked && dialogs.setTakeOverOpen(true)}
            onLinkContainer={() => !locked && dialogs.setLinkOpen(true)}
            onEdit={() => !locked && handleStartEdit()}
            onSave={handleSave}
            onCancelEdit={handleCancelEdit}
            saving={saving}
          />

          <StackDetailTabs
            tabs={stackTabIds}
            activeTab={activeTab}
            onTabChange={setActiveTab}
            iconFor={(tabId) => DETAIL_TAB_ICONS[tabId]}
            labelFor={(tabId) => t(`detail.${tabId}`)}
            ariaLabel={t('detail.tabs.ariaLabel')}
            contextMenuFor={tabContextMenuItems}
          />
        </Card>

        <div
          id="stack-detail-panel"
          role="tabpanel"
          aria-labelledby={`stack-tab-${activeTab}`}
          tabIndex={0}
          className={cn(
            'flex min-h-0 flex-col',
            activeTab === 'logs' || activeTab === 'compose' ? 'flex-1' : undefined,
          )}
        >
          <StackDetailTabContent
            activeTab={activeTab}
            stack={stack}
            isManaged={isManaged}
            locked={locked}
            editing={editing}
            description={editDescription}
            onDescriptionChange={setEditDescription}
          />
        </div>
      </PageBody>

      <StackDetailDialogs stack={stack} locked={locked} dialogs={dialogs} />
    </div>
  );
}
