// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useCallback, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { PageHeader } from '@resources/layout/PageHeader';
import { Alert } from '@resources/components/ui/Alert';
import { Card } from '@resources/components/ui/Card';
import { useCurrentUserPermissions, hasPermission } from '@core/auth/useCurrentUserPermissions';
import { useTabCommands } from '@resources/hooks/useTabCommands';
import { useTabFromUrl } from '@resources/hooks/useTabFromUrl';
import {
  DOCKER_PANEL_ID,
  DOCKER_TAB_ID_PREFIX,
  DOCKER_TAB_PERMISSIONS,
  dockerTabIDs,
  dockerTabIcons,
  type DockerTabId,
} from '../components/docker-tab-groups';
import { DockerSectionHero } from '../components/DockerSectionHero';
import { DockerTabs } from '../components/DockerTabs';
import { DockerInfoTab } from '../components/DockerInfoTab';
import { RegistriesTab } from '../components/RegistriesTab';
import { ResourcesTab } from '../components/ResourcesTab';

export default function DockerPage() {
  const { t } = useTranslation('docker');
  const { t: tCommon } = useTranslation('common');
  const [activeTab, setActiveTab] = useTabFromUrl<DockerTabId>(dockerTabIDs, 'info', {
    storageKey: 'mcharbor-tabs:docker',
  });
  const labelFor = useCallback((tab: DockerTabId) => t(`tabs.${tab}`), [t]);
  const tabCommands = useMemo(
    () =>
      dockerTabIDs.map((tabId) => ({
        id: tabId,
        label: labelFor(tabId),
        icon: dockerTabIcons[tabId],
      })),
    [labelFor],
  );

  useTabCommands({
    pageId: 'docker',
    pageLabel: t('title'),
    tabs: tabCommands,
    onTabChange: setActiveTab,
    keywords: ['docker'],
  });

  // The rail renders a gated section disabled, but the active tab also comes
  // from the URL and from the remembered tab in localStorage, so the panel has
  // to answer for itself: without the key it names the key instead of mounting
  // a tab whose every request would 403.
  const { data: permissions } = useCurrentUserPermissions();
  const required = DOCKER_TAB_PERMISSIONS[activeTab];
  const deniedPermission = required && !hasPermission(permissions, required) ? required : undefined;

  return (
    <div className="flex min-h-0 flex-col gap-4">
      {/* Orientation stays in the topbar, where it keeps the same
          coordinates on every page and does not scroll away. */}
      <PageHeader title={t('title')} description={<DockerSectionHero activeTab={activeTab} />} />

      {/* The same hero card settings uses: identity at the top of the
          content, the tab navigation inside the same card, so the card
          reads as one object — this is the section, these are its
          siblings. */}
      <Card className="flex-none overflow-visible">
        <DockerTabs
          activeTab={activeTab}
          onTabChange={setActiveTab}
          labelFor={labelFor}
          ariaLabel={t('nav.sections')}
        />
      </Card>

      {/* The body keeps a surface of its own, matching settings: Info and
          Resources are plain definition stacks that would read as loose
          rows on the bare canvas. */}
      <Card
        id={DOCKER_PANEL_ID}
        role="tabpanel"
        aria-labelledby={`${DOCKER_TAB_ID_PREFIX}-${activeTab}`}
        className="min-w-0 p-5"
      >
        {deniedPermission ? (
          <Alert
            variant="warning"
            title={t('permission.denied', { ns: 'common' })}
            description={tCommon('permission.requires', {
              permission: deniedPermission,
            })}
          />
        ) : (
          <>
            {activeTab === 'info' && <DockerInfoTab />}
            {activeTab === 'registries' && <RegistriesTab />}
            {activeTab === 'resources' && <ResourcesTab />}
          </>
        )}
      </Card>
    </div>
  );
}
