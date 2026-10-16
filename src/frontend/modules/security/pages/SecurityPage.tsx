// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { PageHeader } from '@resources/layout/PageHeader';
import { PageActionsSlotProvider } from '@resources/layout/PageActionsSlot';
import { PluginTabPanel } from '@core/plugins/PluginTabPanel';
import { isPluginTabId, parsePluginTabId, usePluginPageTabs, type PluginTabId } from '@core/plugins/usePluginPageTabs';
import { Card } from '@resources/components/ui/Card';
import { useTabCommands } from '@resources/hooks/useTabCommands';
import { useTabFromUrl } from '@resources/hooks/useTabFromUrl';
import {
  SECURITY_GROUPS,
  groupForSecurityTab,
  securityTabIDs,
  securityTabIcons,
  securityTabLabelKeys,
  type SecurityGroupId,
  type SecurityTabId,
} from '../components/security-tab-groups';
import { SecuritySectionHero } from '../components/SecuritySectionHero';
import { SecurityTabs } from '../components/SecurityTabs';
import { UsersTab } from '../components/UsersTab';
import { GroupsTab } from '../components/GroupsTab';
import { RolesTab } from '../components/RolesTab';
import { APIKeysTab } from '../components/APIKeysTab';
import { ElevationsTab } from '../components/ElevationsTab';
import { IdentityTab } from '../components/IdentityTab';
import { ScannersTab } from '../components/ScannersTab';
import { HttpsTab } from '../components/HttpsTab';
import { PasswordSettingsSection } from '../components/PasswordSettingsSection';
import { SecretsTab } from '../secrets/components/SecretsTab';
import { PoliciesSection } from '@resources/components/security/PoliciesSection';
import { EndpointsSection } from '@resources/components/security/EndpointsSection';
import { EffectiveAccessTab } from '../components/access/EffectiveAccessTab';
import { ApprovalsTab } from '../components/governance/ApprovalsTab';
import { ApprovalRulesTab } from '../components/governance/ApprovalRulesTab';
import { PolicySimulatorTab } from '../components/governance/PolicySimulatorTab';
import { UserLifecycleTab } from '../components/governance/UserLifecycleTab';
import { BreakGlassTab } from '../components/governance/BreakGlassTab';
import { PolicyConflictWarnings } from '../components/governance/PolicyConflictWarnings';

export default function SecurityPage() {
  const { t } = useTranslation('security');
  const [urlTab, setUrlTab] = useTabFromUrl<SecurityTabId | PluginTabId>(securityTabIDs, 'https', {
    storageKey: 'mcharbor-tabs:security',
    accepts: isPluginTabId,
  });

  const pluginTabs = usePluginPageTabs('security');
  // Only honour a plugin tab the installed plugins still declare.
  const pluginTab = isPluginTabId(urlTab) && pluginTabs.some((tab) => tab.id === urlTab) ? urlTab : null;
  const showBuiltIn = pluginTab === null;
  const [lastBuiltInTab, setLastBuiltInTab] = useState<SecurityTabId>('https');
  if (!isPluginTabId(urlTab) && urlTab !== lastBuiltInTab) setLastBuiltInTab(urlTab);
  const activeTab: SecurityTabId = isPluginTabId(urlTab) ? lastBuiltInTab : urlTab;
  const [actionsSlot, setActionsSlot] = useState<HTMLDivElement | null>(null);
  const setActiveTab = (tab: SecurityTabId) => setUrlTab(tab);
  const setPluginTab = (id: string) => setUrlTab(id as PluginTabId);

  // Leaving Management and coming back used to drop you on Users every
  // time, because the sub-tab was re-derived from the active tab and fell
  // back to a constant. Remember where you were instead.
  const lastManagementTab = useRef<SecurityTabId>('users');
  useEffect(() => {
    if (groupForSecurityTab(activeTab) === 'management') {
      lastManagementTab.current = activeTab;
    }
  }, [activeTab]);
  const entryTabFor = useCallback(
    (group: SecurityGroupId): SecurityTabId =>
      group === 'management' ? lastManagementTab.current : SECURITY_GROUPS[group][0]!,
    [],
  );

  const tabCommands = useMemo(
    () =>
      securityTabIDs.map((tabId) => ({
        id: tabId,
        label: t(securityTabLabelKeys[tabId]),
        icon: securityTabIcons[tabId],
      })),
    [t],
  );

  useTabCommands({
    pageId: 'security',
    pageLabel: t('title'),
    tabs: tabCommands,
    onTabChange: setActiveTab,
    keywords: ['security'],
  });

  return (
    <div className="flex min-h-0 flex-col gap-4">
      <PageHeader
        title={t('title')}
        description={<SecuritySectionHero activeTab={activeTab} />}
        actions={<div ref={setActionsSlot} className="flex items-center gap-2" />}
      />

      {/* The hero, the same one the container detail page opens with:
          identity at the top of the content and the navigation inside the
          same card, so the card reads as one object — this is the section,
          this is where it sits, these are its siblings. */}
      <Card className="flex-none overflow-visible">
        <SecurityTabs
          activeTab={activeTab}
          onTabChange={setActiveTab}
          pluginTabs={pluginTabs}
          activePluginTab={pluginTab}
          onPluginTabChange={setPluginTab}
          entryTabFor={entryTabFor}
          ariaLabel={t('nav.sections')}
        />
      </Card>

      {/* Bare canvas, as on the container detail page: every security
          destination already brings its own surface — a TablePage, an
          `mc-glass` section, a card list. A wrapper here would nest one
          card inside another. */}
      <PageActionsSlotProvider element={actionsSlot}>
        <div
          id="security-detail-panel"
          role="tabpanel"
          aria-labelledby={`security-tab-${activeTab}`}
          className="flex min-w-0 flex-col gap-6"
        >
          {pluginTab !== null &&
            (() => {
              const parsed = parsePluginTabId(pluginTab);
              if (!parsed) return null;
              return (
                <PluginTabPanel
                  slug={parsed.slug}
                  tabKey={parsed.tabKey}
                  context={{ pageId: 'security', tabKey: parsed.tabKey }}
                />
              );
            })()}

          {showBuiltIn && activeTab === 'https' && <HttpsTab />}
          {showBuiltIn && activeTab === 'users' && <UsersTab />}
          {showBuiltIn && activeTab === 'groups' && <GroupsTab />}
          {showBuiltIn && activeTab === 'roles' && <RolesTab />}
          {showBuiltIn && activeTab === 'policies' && <PolicyConflictWarnings subject="policy" />}
          {showBuiltIn && activeTab === 'policies' && <PoliciesSection />}
          {showBuiltIn && activeTab === 'scim' && <EndpointsSection />}
          {showBuiltIn && activeTab === 'api-keys' && <APIKeysTab />}
          {showBuiltIn && activeTab === 'elevations' && <ElevationsTab />}
          {showBuiltIn && activeTab === 'access' && <EffectiveAccessTab />}
          {showBuiltIn && activeTab === 'approvals' && <ApprovalsTab />}
          {showBuiltIn && activeTab === 'approval-rules' && <ApprovalRulesTab />}
          {showBuiltIn && activeTab === 'simulator' && <PolicySimulatorTab />}
          {showBuiltIn && activeTab === 'lifecycle' && <UserLifecycleTab />}
          {showBuiltIn && activeTab === 'break-glass' && <BreakGlassTab />}
          {showBuiltIn && activeTab === 'identity' && <IdentityTab />}
          {showBuiltIn && activeTab === 'scanning' && <ScannersTab />}
          {showBuiltIn && activeTab === 'password' && <PasswordSettingsSection />}
          {showBuiltIn && activeTab === 'secrets' && <SecretsTab />}
        </div>
      </PageActionsSlotProvider>
    </div>
  );
}
