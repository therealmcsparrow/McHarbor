// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// One group, in full.
//
// Built on the user detail page's shape, for the same reason it exists
// there: a group's members and roles are each a list that wants the full
// width, not a third of the page beside the directory. Breadcrumb in the
// topbar, hero card carrying identity, state and actions with the rail
// inside it, and a bare panel underneath — every tab brings its own card.

import { useLayoutEffect, useMemo, useState } from 'react';
import { useNavigate, useParams } from 'react-router';
import { useTranslation } from 'react-i18next';
import { IconLock } from '@tabler/icons-react';
import { Card } from '@resources/components/ui/Card';
import { CardSkeleton } from '@resources/components/ui/Skeleton';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import { ErrorState } from '@resources/components/ui/ErrorState';
import { HeaderBreadcrumb } from '@resources/layout/HeaderBreadcrumb';
import type { BreadcrumbSibling } from '@resources/layout/Breadcrumb';
import { useHeaderSlot } from '@resources/stores/headerSlot';
import { useTabFromUrl } from '@resources/hooks/useTabFromUrl';
import { useDeleteGroup, useGroup, useGroups } from '../hooks/useGroups';
import { GroupDetailHero } from '../components/GroupDetailHero';
import { GroupDetailTabs } from '../components/GroupDetailTabs';
import { groupDetailTabIDs, type GroupDetailTabId } from '../components/group-detail-tabs';
import { GroupFormDialog } from '../components/GroupFormDialog';
import { GroupOverviewTab } from '../components/tabs/GroupOverviewTab';
import { GroupMembersTab } from '../components/tabs/GroupMembersTab';
import { GroupRolesTab } from '../components/tabs/GroupRolesTab';

export default function GroupDetailPage() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const { t } = useTranslation('security');
  const { data: group, isLoading, isError, error } = useGroup(id ?? '');
  const { data: allGroups = [] } = useGroups();
  const deleteGroup = useDeleteGroup();
  const [activeTab, setActiveTab] = useTabFromUrl<GroupDetailTabId>(groupDetailTabIDs, 'overview', {
    storageKey: 'mcharbor-tabs:group-detail',
  });
  const [editOpen, setEditOpen] = useState(false);
  const [deleteOpen, setDeleteOpen] = useState(false);

  const setHeaderActive = useHeaderSlot((store) => store.setActive);
  useLayoutEffect(() => {
    setHeaderActive(true);
    return () => setHeaderActive(false);
  }, [setHeaderActive]);

  // Peers for the breadcrumb leaf switcher, so moving between two groups
  // does not mean a round trip through the directory.
  const siblings: BreadcrumbSibling[] = useMemo(
    () =>
      allGroups.map((peer) => ({
        id: peer.id,
        label: peer.name,
        to: `/security/groups/${peer.id}`,
        adornment: peer.isSystem ? (
          <>
            <IconLock aria-hidden="true" className="size-3 text-muted-foreground" />
            <span className="sr-only">{t('groups.system')}</span>
          </>
        ) : undefined,
      })),
    [allGroups, t],
  );

  if (isLoading) {
    return (
      <div className="flex h-full flex-col gap-4">
        <CardSkeleton lines={2} className="min-h-24" />
        <CardSkeleton lines={8} className="min-h-64" />
      </div>
    );
  }

  if (isError || !group) {
    return (
      <div>
        <ErrorState
          variant="page"
          title={t('groups.detail.notFoundTitle')}
          error={error}
          description={t('groups.detail.notFoundDescription')}
          primaryAction={{
            label: t('groups.detail.backToGroups'),
            onClick: () => navigate('/security?tab=groups'),
          }}
        />
      </div>
    );
  }

  return (
    <div className="flex h-full min-h-0 flex-col gap-4">
      <HeaderBreadcrumb
        crumbs={[
          { label: t('title'), to: '/security' },
          { label: t('tabs.groups'), to: '/security?tab=groups' },
          { label: group.name },
        ]}
        siblings={siblings}
        currentId={group.id}
      />

      <Card className="flex-none overflow-visible">
        <div className="px-5 pt-4 pb-3">
          <GroupDetailHero group={group} onEdit={() => setEditOpen(true)} onDelete={() => setDeleteOpen(true)} />
        </div>
        <GroupDetailTabs
          activeTab={activeTab}
          onTabChange={setActiveTab}
          ariaLabel={t('groups.detail.tabsLabel')}
          tabBadges={{
            members: group.members?.length ?? group.memberCount,
            roles: group.roles?.length ?? 0,
          }}
        />
      </Card>

      <div
        id="group-detail-panel"
        role="tabpanel"
        aria-labelledby={`group-tab-${activeTab}`}
        tabIndex={0}
        className="flex min-h-0 flex-1 flex-col overflow-y-auto"
      >
        {activeTab === 'overview' && <GroupOverviewTab group={group} onNavigate={setActiveTab} />}
        {activeTab === 'members' && <GroupMembersTab group={group} />}
        {activeTab === 'roles' && <GroupRolesTab group={group} />}
      </div>

      <GroupFormDialog open={editOpen} onOpenChange={setEditOpen} group={group} />
      <ConfirmDialog
        open={deleteOpen}
        onOpenChange={setDeleteOpen}
        title={t('groups.deleteGroup')}
        description={t('groups.deleteGroupConfirm')}
        onConfirm={() => {
          deleteGroup.mutate(group.id, {
            onSuccess: () => {
              setDeleteOpen(false);
              void navigate('/security?tab=groups');
            },
          });
        }}
        loading={deleteGroup.isPending}
      />
    </div>
  );
}
