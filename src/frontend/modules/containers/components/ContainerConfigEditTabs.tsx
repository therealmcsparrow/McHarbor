// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconContainer, IconCpu, IconFolder, IconShield, IconNetwork } from '@tabler/icons-react';
import { Section } from '@resources/components/ui/Form';
import { Box } from '@resources/components/ui/Layout';
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@resources/components/ui/Tabs';
import type { ContainerInspect } from '@core/types/docker';
import type { useContainerConfig } from '../hooks/useContainerConfig';
import { EnvironmentTab } from './tabs/EnvironmentTab';
import { LabelsTab } from './tabs/LabelsTab';
import { MountsTab } from './tabs/MountsTab';
import { NetworkTab } from './tabs/NetworkTab';
import { ResourcesTab } from './tabs/ResourcesTab';
import { SecurityTab } from './tabs/SecurityTab';

export type ConfigEditTabId = 'environment' | 'network' | 'mounts' | 'security' | 'resources';

type ContainerConfigEditTabsProps = {
  tab: ConfigEditTabId;
  onTabChange: (tab: ConfigEditTabId) => void;
  container: ContainerInspect;
  config: ReturnType<typeof useContainerConfig>;
};

// The four config sections of ContainerConfigEditShell, each wrapping the
// matching per-tab editor from the container detail page.
export function ContainerConfigEditTabs({ tab, onTabChange, container, config }: ContainerConfigEditTabsProps) {
  const { t } = useTranslation('containers');

  return (
    <Tabs value={tab} onValueChange={(v) => onTabChange(v as ConfigEditTabId)}>
      <TabsList>
        <TabsTrigger value="environment" className="gap-1.5">
          <IconContainer className="size-4" />
          {t('configEdit.sections.environment')}
        </TabsTrigger>
        <TabsTrigger value="network" className="gap-1.5">
          <IconNetwork className="size-4" />
          {t('configEdit.sections.network')}
        </TabsTrigger>
        <TabsTrigger value="mounts" className="gap-1.5">
          <IconFolder className="size-4" />
          {t('detail.mounts')}
        </TabsTrigger>
        <TabsTrigger value="security" className="gap-1.5">
          <IconShield className="size-4" />
          {t('configEdit.sections.security')}
        </TabsTrigger>
        <TabsTrigger value="resources" className="gap-1.5">
          <IconCpu className="size-4" />
          {t('configEdit.sections.resources')}
        </TabsTrigger>
      </TabsList>

      <Box className="mt-4">
        <TabsContent value="environment">
          <Section title={t('configEdit.sections.environment')}>
            <EnvironmentTab
              container={container}
              editing={config.editing}
              editData={config.data}
              onFieldChange={config.setField}
            />
            {/* EnvironmentTab ends in its own card and LabelsTab opens with
                one, so without a gap the two read as a single run-on block. */}
            <Box className="mt-6">
              <LabelsTab
                container={container}
                editing={config.editing}
                editData={config.data}
                onFieldChange={config.setField}
              />
            </Box>
          </Section>
        </TabsContent>

        <TabsContent value="network">
          <Section title={t('configEdit.sections.network')}>
            <NetworkTab
              container={container}
              editing={config.editing}
              allowLiveNetworkActionsInEdit
              layout="editor"
              editData={config.data}
              onFieldChange={config.setField}
            />
          </Section>
        </TabsContent>

        <TabsContent value="mounts">
          <Section title={t('detail.mounts')}>
            <MountsTab
              container={container}
              editing={config.editing}
              editData={config.data}
              onFieldChange={config.setField}
            />
          </Section>
        </TabsContent>

        <TabsContent value="security">
          <Section title={t('configEdit.sections.security')}>
            <SecurityTab
              container={container}
              editing={config.editing}
              editData={config.data}
              onFieldChange={config.setField}
            />
          </Section>
        </TabsContent>

        <TabsContent value="resources">
          <Section title={t('configEdit.sections.resources')}>
            <ResourcesTab
              container={container}
              editing={config.editing}
              editData={config.data}
              onFieldChange={config.setField}
            />
          </Section>
        </TabsContent>
      </Box>
    </Tabs>
  );
}
