// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  IconUsers,
  IconUsersGroup,
  IconShieldCheck,
  IconKey,
  IconFingerprint,
  IconBug,
} from '@tabler/icons-react';
import { PageHeader } from '@resources/layout/PageHeader';
import { Button } from '@resources/components/ui/Button';
import { cn } from '@resources/utils/cn';
import { UsersTab } from '../components/UsersTab';
import { GroupsTab } from '../components/GroupsTab';
import { RolesTab } from '../components/RolesTab';
import { APIKeysTab } from '../components/APIKeysTab';
import { IdentityTab } from '../components/IdentityTab';
import { ScannersTab } from '../components/ScannersTab';

type Section = 'management' | 'identity' | 'scanning';

const MANAGEMENT_TABS = [
  { id: 'users', icon: IconUsers },
  { id: 'groups', icon: IconUsersGroup },
  { id: 'roles', icon: IconShieldCheck },
  { id: 'api-keys', icon: IconKey },
] as const;

type ManagementTabId = (typeof MANAGEMENT_TABS)[number]['id'];

const TAB_LABELS: Record<ManagementTabId, string> = {
  users: 'tabs.users',
  groups: 'tabs.groups',
  roles: 'tabs.roles',
  'api-keys': 'tabs.apiKeys',
};

export default function SecurityPage() {
  const { t } = useTranslation('security');
  const [section, setSection] = useState<Section>('management');
  const [managementTab, setManagementTab] = useState<ManagementTabId>('users');

  return (
    <div className="space-y-6">
      <PageHeader title={t('title')} description={t('description')} />

      {/* Section selector */}
      <div className="flex gap-1 rounded-lg border border-border bg-muted/30 p-1">
        <Button
          variant="ghost"
          size="sm"
          onClick={() => setSection('management')}
          className={cn(
            'flex items-center gap-2 rounded-md px-3 py-1.5 text-sm font-medium transition-colors',
            section === 'management'
              ? 'bg-background text-foreground shadow-sm'
              : 'text-muted-foreground hover:text-foreground'
          )}
        >
          <IconUsersGroup className="size-4" />
          {t('sections.management')}
        </Button>
        <Button
          variant="ghost"
          size="sm"
          onClick={() => setSection('identity')}
          className={cn(
            'flex items-center gap-2 rounded-md px-3 py-1.5 text-sm font-medium transition-colors',
            section === 'identity'
              ? 'bg-background text-foreground shadow-sm'
              : 'text-muted-foreground hover:text-foreground'
          )}
        >
          <IconFingerprint className="size-4" />
          {t('sections.identity')}
        </Button>
        <Button
          variant="ghost"
          size="sm"
          onClick={() => setSection('scanning')}
          className={cn(
            'flex items-center gap-2 rounded-md px-3 py-1.5 text-sm font-medium transition-colors',
            section === 'scanning'
              ? 'bg-background text-foreground shadow-sm'
              : 'text-muted-foreground hover:text-foreground'
          )}
        >
          <IconBug className="size-4" />
          {t('sections.scanning')}
        </Button>
      </div>

      {/* Management sub-tabs */}
      {section === 'management' && (
        <>
          <div className="flex gap-1 rounded-lg border border-border bg-muted/30 p-1">
            {MANAGEMENT_TABS.map((tab) => (
              <Button
                key={tab.id}
                variant="ghost"
                size="sm"
                onClick={() => setManagementTab(tab.id)}
                className={cn(
                  'flex items-center gap-2 rounded-md px-3 py-1.5 text-sm font-medium transition-colors',
                  managementTab === tab.id
                    ? 'bg-background text-foreground shadow-sm'
                    : 'text-muted-foreground hover:text-foreground'
                )}
              >
                <tab.icon className="size-4" />
                {t(TAB_LABELS[tab.id])}
              </Button>
            ))}
          </div>

          {managementTab === 'users' && <UsersTab />}
          {managementTab === 'groups' && <GroupsTab />}
          {managementTab === 'roles' && <RolesTab />}
          {managementTab === 'api-keys' && <APIKeysTab />}
        </>
      )}

      {/* Identity section */}
      {section === 'identity' && <IdentityTab />}

      {/* Scanning section */}
      {section === 'scanning' && <ScannersTab />}
    </div>
  );
}
