// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// DockerSectionHero — the identity half of the Docker hero card.
//
// Same job, and the same markup, as `SettingsSectionHero`: a tinted icon
// tile, an <h1> that names the section you are looking at, and a metadata
// row under it carrying the page it belongs to and what the section is for.
// On settings the badge names one of five groups; Docker has no groups, so
// the badge names the page — it is the "you are inside Docker" anchor that
// keeps the card reading as one object once you have scrolled past the
// topbar.
//
// The markup is `SectionHero` in @resources; this file supplies the
// docker namespace's key shapes.

import type { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { IconBrandDocker } from '@tabler/icons-react';
import { SectionHero } from '@resources/components/SectionHero';
import { dockerTabIcons, type DockerTabId } from './docker-tab-groups';

type DockerSectionHeroProps = {
  activeTab: DockerTabId;
  /** Section-scoped controls, rendered opposite the identity block. */
  actions?: ReactNode;
};

export function DockerSectionHero({ activeTab, actions }: DockerSectionHeroProps) {
  const { t } = useTranslation('docker');

  return (
    <SectionHero
      icon={dockerTabIcons[activeTab]}
      title={t(`tabs.${activeTab}`)}
      badgeIcon={IconBrandDocker}
      badgeLabel={t('title')}
      description={t(`sectionDescription.${activeTab}`)}
      actions={actions}
    />
  );
}
