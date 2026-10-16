// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Select } from '@resources/components/ui/Select';
import type { AgentUpdateFilter } from './useAgentInventoryFilters';

type AgentInventoryFiltersProps = {
  update: AgentUpdateFilter;
  setUpdate: (value: AgentUpdateFilter) => void;
  platform: string;
  setPlatform: (value: string) => void;
  platforms: string[];
  version: string;
  setVersion: (value: string) => void;
  versions: string[];
};

export function AgentInventoryFilters({
  update,
  setUpdate,
  platform,
  setPlatform,
  platforms,
  version,
  setVersion,
  versions,
}: AgentInventoryFiltersProps) {
  const { t } = useTranslation('settings');
  return (
    <>
      <Select
        value={update}
        onChange={(value) => setUpdate(value === 'yes' || value === 'no' ? value : '')}
        ariaLabel={t('about.agents.updateAvailable')}
        searchable={false}
        className="w-48"
        options={[
          { value: '', label: t('about.agents.filter.anyUpdate') },
          { value: 'yes', label: `${t('about.agents.updateAvailable')}: ${t('labels.yes', { ns: 'common' })}` },
          { value: 'no', label: `${t('about.agents.updateAvailable')}: ${t('labels.no', { ns: 'common' })}` },
        ]}
      />
      <Select
        value={platform}
        onChange={setPlatform}
        ariaLabel={t('about.agents.platform')}
        searchable={false}
        className="w-40"
        options={[
          { value: '', label: t('about.agents.filter.allPlatforms') },
          ...platforms.map((value) => ({ value, label: value })),
        ]}
      />
      <Select
        value={version}
        onChange={setVersion}
        ariaLabel={t('about.agents.version')}
        searchable={false}
        className="w-40"
        options={[
          { value: '', label: t('about.agents.filter.allVersions') },
          ...versions.map((value) => ({ value, label: value })),
        ]}
      />
    </>
  );
}
