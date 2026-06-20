// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import {
  IconInfoCircle,
  IconPackage,
  IconServer,
} from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { SystemInfoBlock, SystemInfoRow } from './SystemInfoBlock';
import packageJson from '../../../package.json';
import type { SystemInfo } from '../types';

export function SystemOverviewTab({ info }: { info: SystemInfo }) {
  const { t } = useTranslation('system');

  return (
    <div className="space-y-4">
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-3">
        <SystemInfoBlock
          title={t('overview.application')}
          description={t('overview.applicationDescription')}
          icon={IconInfoCircle}
        >
          <SystemInfoRow label={t('fields.name')} value="McHarbor" />
          <SystemInfoRow label={t('fields.backendVersion')} value={`v${info.version}`} />
          <SystemInfoRow label={t('fields.frontendVersion')} value={`v${info.version}`} />
        </SystemInfoBlock>

        <SystemInfoBlock
          title={t('overview.runtime')}
          description={t('overview.runtimeDescription')}
          icon={IconServer}
        >
          <SystemInfoRow label={t('fields.platform')} value={info.platform} />
          <SystemInfoRow label={t('fields.goVersion')} value={info.goVersion} />
          <SystemInfoRow
            label={t('fields.status')}
            value={<Badge variant="default">{t('status.available')}</Badge>}
          />
        </SystemInfoBlock>

        <SystemInfoBlock
          title={t('overview.dependencies')}
          description={t('overview.dependenciesDescription')}
          icon={IconPackage}
        >
          <SystemInfoRow
            label={t('fields.backendDependencies')}
            value={info.dependencies.length}
          />
          <SystemInfoRow
            label={t('fields.frontendDependencies')}
            value={Object.keys(packageJson.dependencies ?? {}).length}
          />
        </SystemInfoBlock>
      </div>
    </div>
  );
}
