// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { SubNav, type SubNavItem } from '@resources/components/SubNav';

type Props = {
  sections: readonly SubNavItem<string>[];
  activeSection: string;
  onSelectSection: (anchor: string) => void;
  panelId: string;
};

export function ViewerHero({ sections, activeSection, onSelectSection, panelId }: Props) {
  const { t } = useTranslation('reporting');
  if (sections.length === 0) return null;

  return (
    <div className="border-b border-border bg-transparent px-5 py-3">
      <SubNav
        items={sections}
        value={activeSection}
        onChange={onSelectSection}
        ariaLabel={t('viewer.sections')}
        idPrefix="report-section-tab"
        controls={panelId}
      />
    </div>
  );
}
