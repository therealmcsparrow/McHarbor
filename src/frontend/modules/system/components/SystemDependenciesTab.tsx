// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { IconPackage } from '@tabler/icons-react';
import packageJson from '../../../package.json';
import type { SystemInfo } from '../types';
import { SystemInfoBlock, SystemInfoRow } from './SystemInfoBlock';

function cleanVersion(version: string): string {
  return version.replace(/^[\^~>=<]+/, '');
}

export function SystemDependenciesTab({ info }: { info: SystemInfo }) {
  const { t } = useTranslation('system');
  const frontendDependencies = useMemo(
    () =>
      Object.entries(packageJson.dependencies ?? {})
        .map(([name, version]) => ({ name, version: cleanVersion(version) }))
        .sort((a, b) => a.name.localeCompare(b.name)),
    []
  );

  return (
    <div className="grid grid-cols-1 gap-4 xl:grid-cols-2">
      <SystemInfoBlock
        title={t('dependencies.backend')}
        description={t('dependencies.backendDescription')}
        icon={IconPackage}
      >
        {info.dependencies.map((dependency) => (
          <SystemInfoRow
            key={dependency.name}
            label={dependency.name}
            value={<span className="font-mono text-xs">{dependency.version}</span>}
          />
        ))}
      </SystemInfoBlock>

      <SystemInfoBlock
        title={t('dependencies.frontend')}
        description={t('dependencies.frontendDescription')}
        icon={IconPackage}
      >
        {frontendDependencies.map((dependency) => (
          <SystemInfoRow
            key={dependency.name}
            label={dependency.name}
            value={<span className="font-mono text-xs">{dependency.version}</span>}
          />
        ))}
      </SystemInfoBlock>
    </div>
  );
}
