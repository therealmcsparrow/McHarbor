// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// SecuritySectionHero — the identity half of the security hero card.
//
// Same job as `ContainerDetailHeader`'s identity column, and the same
// shape as `SettingsSectionHero`: a tinted icon tile, an <h1> naming what
// you are looking at, and a metadata row carrying the group it belongs to
// and what the destination is for.
//
// The group badge is not redundant with the rail above it. Four of the
// five groups have no sub-rail, so the rail alone cannot tell you that
// "Roles" sits under Management while "Scanning" is a section in its own
// right.
//
// The markup is `SectionHero` in @resources; this file supplies the
// security namespace's key shapes.

import type { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { SectionHero } from '@resources/components/SectionHero';
import {
  groupForSecurityTab,
  securityGroupIcons,
  securityGroupLabelKey,
  securityTabDescriptionKeys,
  securityTabIcons,
  securityTabLabelKeys,
  type SecurityTabId,
} from './security-tab-groups';

type SecuritySectionHeroProps = {
  activeTab: SecurityTabId;
  /** Destination-scoped controls, rendered opposite the identity block. */
  actions?: ReactNode;
};

export function SecuritySectionHero({ activeTab, actions }: SecuritySectionHeroProps) {
  const { t } = useTranslation('security');
  const group = groupForSecurityTab(activeTab);

  return (
    <SectionHero
      icon={securityTabIcons[activeTab]}
      title={t(securityTabLabelKeys[activeTab])}
      badgeIcon={securityGroupIcons[group]}
      badgeLabel={t(securityGroupLabelKey(group))}
      description={t(securityTabDescriptionKeys[activeTab])}
      actions={actions}
    />
  );
}
