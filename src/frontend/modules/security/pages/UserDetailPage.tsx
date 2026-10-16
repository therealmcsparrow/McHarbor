// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// One user, in full.
//
// Built on the container detail page's shape, for the same reason it
// exists there: an object with six unrelated faces does not fit in a rail
// beside the list it came from. Breadcrumb in the topbar where it keeps
// the same coordinates on every page, hero card carrying identity, state
// and actions with the rail inside it, and a bare panel underneath —
// every tab already brings its own card.

import { useLayoutEffect, useMemo, useState } from 'react';
import { useNavigate, useParams } from 'react-router';
import { useTranslation } from 'react-i18next';
import { Card } from '@resources/components/ui/Card';
import { CardSkeleton } from '@resources/components/ui/Skeleton';
import { ErrorState } from '@resources/components/ui/ErrorState';
import { HeaderBreadcrumb } from '@resources/layout/HeaderBreadcrumb';
import type { BreadcrumbSibling } from '@resources/layout/Breadcrumb';
import { useHeaderSlot } from '@resources/stores/headerSlot';
import { useTabFromUrl } from '@resources/hooks/useTabFromUrl';
import { StatusIndicator } from '@resources/components/ui/StatusIndicator';
import { useUser, useUsers, useUserGroups, useUserRoles } from '@resources/hooks/useUsers';
import { UserDetailHero } from '../components/UserDetailHero';
import { UserDetailTabs } from '../components/UserDetailTabs';
import { userDetailTabIDs, type UserDetailTabId } from '../components/user-detail-tabs';
import { userAccountState } from '../components/user-status';
import { ResetPasswordDialog } from '../components/ResetPasswordDialog';
import { DisableAccountDialog } from '../components/DisableAccountDialog';
import { UserOverviewTab } from '../components/tabs/UserOverviewTab';
import { UserGroupsTab } from '../components/tabs/UserGroupsTab';
import { UserRolesTab } from '../components/tabs/UserRolesTab';
import { UserOverridesTab } from '../components/tabs/UserOverridesTab';
import { UserSessionsTab } from '../components/tabs/UserSessionsTab';
import { UserAccountTab } from '../components/tabs/UserAccountTab';

export default function UserDetailPage() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const { t } = useTranslation('security');
  const { data: user, isLoading, isError, error } = useUser(id);
  const { data: allUsers = [] } = useUsers();
  const { data: groups = [] } = useUserGroups(id ?? '');
  const { data: roles = [] } = useUserRoles(id ?? '');
  const [activeTab, setActiveTab] = useTabFromUrl<UserDetailTabId>(userDetailTabIDs, 'overview', {
    storageKey: 'mcharbor-tabs:user-detail',
  });
  const [resetOpen, setResetOpen] = useState(false);
  const [disableOpen, setDisableOpen] = useState(false);

  const setHeaderActive = useHeaderSlot((store) => store.setActive);
  useLayoutEffect(() => {
    setHeaderActive(true);
    return () => setHeaderActive(false);
  }, [setHeaderActive]);

  // Peers for the breadcrumb leaf switcher, so moving between two accounts
  // does not mean a round trip through the directory. The list is already
  // cached by the users tab in the common case.
  const siblings: BreadcrumbSibling[] = useMemo(
    () =>
      allUsers.map((peer) => ({
        id: peer.id,
        label: peer.displayName || peer.username,
        to: `/security/users/${peer.id}`,
        adornment: (
          <StatusIndicator
            tone={userAccountState(peer) === 'active' ? 'success' : 'danger'}
            variant="dot"
            size="sm"
            ariaLabel={peer.isActive ? t('users.active') : t('users.inactive')}
          />
        ),
      })),
    [allUsers, t],
  );

  if (isLoading) {
    return (
      <div className="flex h-full flex-col gap-4">
        <CardSkeleton lines={2} className="min-h-24" />
        <CardSkeleton lines={8} className="min-h-64" />
      </div>
    );
  }

  if (isError || !user) {
    return (
      <div>
        <ErrorState
          variant="page"
          title={t('users.detail.notFoundTitle')}
          error={error}
          description={t('users.detail.notFoundDescription')}
          primaryAction={{
            label: t('users.detail.backToUsers'),
            onClick: () => navigate('/security?tab=users'),
          }}
        />
      </div>
    );
  }

  const itemName = user.displayName || user.username;

  return (
    <div className="flex h-full min-h-0 flex-col gap-4">
      <HeaderBreadcrumb
        crumbs={[
          { label: t('title'), to: '/security' },
          { label: t('tabs.users'), to: '/security?tab=users' },
          { label: itemName },
        ]}
        siblings={siblings}
        currentId={user.id}
      />

      <Card className="flex-none overflow-visible">
        <div className="px-5 pt-4 pb-3">
          <UserDetailHero
            user={user}
            onResetPassword={() => setResetOpen(true)}
            onDisable={() => setDisableOpen(true)}
          />
        </div>
        <UserDetailTabs
          activeTab={activeTab}
          onTabChange={setActiveTab}
          ariaLabel={t('users.detail.tabsLabel')}
          tabBadges={{ groups: groups.length, roles: roles.length }}
        />
      </Card>

      <div
        id="user-detail-panel"
        role="tabpanel"
        aria-labelledby={`user-tab-${activeTab}`}
        tabIndex={0}
        className="flex min-h-0 flex-1 flex-col overflow-y-auto"
      >
        {activeTab === 'overview' && <UserOverviewTab user={user} onNavigate={setActiveTab} />}
        {activeTab === 'groups' && <UserGroupsTab userId={user.id} />}
        {activeTab === 'roles' && <UserRolesTab userId={user.id} />}
        {activeTab === 'permissions' && <UserOverridesTab userId={user.id} userDisplayName={itemName} />}
        {activeTab === 'sessions' && <UserSessionsTab userId={user.id} />}
        {activeTab === 'account' && <UserAccountTab user={user} />}
      </div>

      <ResetPasswordDialog open={resetOpen} onOpenChange={setResetOpen} userId={user.id} username={user.username} />
      <DisableAccountDialog
        open={disableOpen}
        onOpenChange={setDisableOpen}
        userId={user.id}
        username={user.username}
      />
    </div>
  );
}
