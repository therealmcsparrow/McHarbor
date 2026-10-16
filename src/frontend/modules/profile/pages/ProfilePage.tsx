// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// ProfilePage — built to the container detail page's grammar.
//
// Before, this page was a two-column grid: a small identity card on the
// left and one tall scrolling column on the right holding four unrelated
// concerns stacked end to end — account fields, theme, language, password,
// MFA. Nothing told you how far down the page went, and the identity card
// carried the same three values the form beside it already showed.
//
// It now reads the way a container reads: a hero card at the top carrying
// identity and state, the tab rail inside that card so the card is one
// object, and one concern at a time in the panel below. `?tab=` addresses
// a face directly, exactly as it does on the container page.

import { useCallback, useState } from 'react';
import { useSearchParams } from 'react-router';
import { useTranslation } from 'react-i18next';
import { Card } from '@resources/components/ui/Card';
import { PageHeader } from '@resources/layout/PageHeader';
import { useAuth } from '@core/auth/useAuth';
import { useMFAStatus } from '@resources/hooks/useMFA';
import { ProfileTabs, isProfileTabId, type ProfileTabId } from '../components/ProfileTabs';
import { OverviewTab } from '../components/tabs/OverviewTab';
import { AccountTab } from '../components/tabs/AccountTab';
import { AppearanceTab } from '../components/tabs/AppearanceTab';
import { NotificationsTab } from '../components/tabs/NotificationsTab';
import { SecurityTab } from '../components/tabs/SecurityTab';
import { ProfileHeader } from './ProfileHeader';

export default function ProfilePage() {
  const { t } = useTranslation('common');
  const user = useAuth((s) => s.user);
  const [searchParams, setSearchParams] = useSearchParams();
  const mfaStatus = useMFAStatus();

  const requestedTab = searchParams.get('tab');
  const [activeTab, setActiveTab] = useState<ProfileTabId>(() =>
    isProfileTabId(requestedTab) ? requestedTab : 'overview',
  );

  // The URL stays authoritative: a pasted `?tab=security` link opens on
  // Security, and picking a tab rewrites the query without stacking a
  // history entry per click.
  const [seenRequestedTab, setSeenRequestedTab] = useState(requestedTab);
  if (requestedTab !== seenRequestedTab) {
    setSeenRequestedTab(requestedTab);
    if (isProfileTabId(requestedTab)) setActiveTab(requestedTab);
  }

  const handleTabChange = useCallback(
    (tab: ProfileTabId) => {
      setActiveTab(tab);
      setSearchParams(
        (current) => {
          current.set('tab', tab);
          return current;
        },
        { replace: true, preventScrollReset: true },
      );
    },
    [setSearchParams],
  );

  const displayName = user?.displayName || user?.username || t('profile.account.unknownUser');
  const systemAccount = !user || user.id === 'system';
  const identityProviderLocked = Boolean(user?.identityProviderId);
  const canEditProfile = Boolean(user && user.id !== 'system');
  const mfaEnabled = mfaStatus.isLoading ? undefined : Boolean(mfaStatus.data?.enabled);

  return (
    <div className="flex h-full min-h-0 flex-col gap-4">
      {/* Title and description stay in the topbar, where they keep the same
          coordinates on every page. The hero below carries identity. */}
      <PageHeader title={t('profile.title')} description={t('profile.description')} />

      <Card className="flex-none overflow-visible">
        <div className="px-5 pt-4 pb-3">
          <ProfileHeader
            displayName={displayName}
            username={user?.username ?? '-'}
            email={user?.email ?? null}
            identityProviderLocked={identityProviderLocked}
            systemAccount={systemAccount}
            mfaEnabled={mfaEnabled}
          />
        </div>
        <ProfileTabs
          activeTab={activeTab}
          onTabChange={handleTabChange}
          labelFor={(tab) => tabLabel(tab, t)}
          ariaLabel={t('profile.tabs.ariaLabel')}
        />
      </Card>

      <div className="flex min-h-0 flex-1 flex-col">
        <div
          id="profile-detail-panel"
          role="tabpanel"
          aria-labelledby={`profile-tab-${activeTab}`}
          tabIndex={0}
          className="flex min-h-0 flex-1 flex-col overflow-y-auto"
        >
          {activeTab === 'overview' && (
            <OverviewTab
              username={user?.username ?? '-'}
              displayName={user?.displayName ?? null}
              email={user?.email ?? null}
              identityProviderLocked={identityProviderLocked}
              systemAccount={systemAccount}
              mfaEnabled={mfaEnabled}
              onNavigate={handleTabChange}
            />
          )}
          {activeTab === 'account' && (
            <AccountTab
              username={user?.username ?? ''}
              displayName={user?.displayName ?? null}
              email={user?.email ?? null}
              canEdit={canEditProfile}
              identityProviderLocked={identityProviderLocked}
            />
          )}
          {activeTab === 'appearance' && <AppearanceTab />}
          {activeTab === 'notifications' && <NotificationsTab />}
          {activeTab === 'security' && !systemAccount && (
            <SecurityTab identityProviderLocked={identityProviderLocked} />
          )}
          {activeTab === 'security' && systemAccount && (
            <section className="mc-glass rounded-lg p-6 text-sm text-muted-foreground">
              {t('profile.account.systemAccount')}
            </section>
          )}
        </div>
      </div>
    </div>
  );
}

/**
 * Tab labels as literal `t('...')` calls rather than an interpolated key —
 * the i18n CI gate reads literals, and a template key is invisible to it.
 */
function tabLabel(tab: ProfileTabId, t: (key: string) => string): string {
  switch (tab) {
    case 'overview':
      return t('profile.tabs.overview');
    case 'account':
      return t('profile.tabs.account');
    case 'appearance':
      return t('profile.tabs.appearance');
    case 'notifications':
      return t('profile.tabs.notifications');
    case 'security':
      return t('profile.tabs.security');
  }
}
