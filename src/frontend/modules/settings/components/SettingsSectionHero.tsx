// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// SettingsSectionHero — the identity half of the settings hero card.
//
// Same job as `ContainerDetailHeader`'s identity column: a tinted icon
// tile, an <h1> that names what you are looking at, and a metadata row
// under it. On a container that is the name plus state, image and id. On
// settings it is the active section plus the group it belongs to and what
// the section is for, because "which of the thirteen am I in" is the
// question a settings page actually has to answer.
//
// The markup is `SectionHero` in @resources; this file supplies the
// settings namespace's key shapes.

import type { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { SectionHero } from '@resources/components/SectionHero';
import {
  groupForSettingsTab,
  settingsGroupIcons,
  settingsGroupLabelKey,
  settingsTabIcons,
  type SettingsTabId,
} from './settings-tab-groups';

type SettingsSectionHeroProps = {
  activeTab: SettingsTabId;
  /** Section-scoped controls, rendered opposite the identity block. */
  actions?: ReactNode;
};

export function SettingsSectionHero({ activeTab, actions }: SettingsSectionHeroProps) {
  const { t } = useTranslation('settings');
  const group = groupForSettingsTab(activeTab);

  return (
    <SectionHero
      icon={settingsTabIcons[activeTab]}
      title={t(`tabs.${activeTab}`)}
      badgeIcon={settingsGroupIcons[group]}
      badgeLabel={t(settingsGroupLabelKey(group))}
      description={t(`sectionDescription.${activeTab}`)}
      actions={actions}
    />
  );
}
