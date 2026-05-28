// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconCpu, IconServer } from '@tabler/icons-react';
import type { SystemInfo } from '../types';
import { SystemInfoBlock, SystemInfoRow } from './SystemInfoBlock';

function browserValue(key: 'language' | 'platform' | 'userAgent'): string {
  if (typeof window === 'undefined') {
    return '-';
  }

  return window.navigator[key] || '-';
}

export function SystemRuntimeTab({ info }: { info: SystemInfo }) {
  const { t } = useTranslation('system');

  return (
    <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
      <SystemInfoBlock
        title={t('runtime.backend')}
        description={t('runtime.backendDescription')}
        icon={IconServer}
      >
        <SystemInfoRow label={t('fields.platform')} value={info.platform} />
        <SystemInfoRow label={t('fields.goVersion')} value={info.goVersion} />
        <SystemInfoRow label={t('fields.backendVersion')} value={`v${info.version}`} />
      </SystemInfoBlock>

      <SystemInfoBlock
        title={t('runtime.client')}
        description={t('runtime.clientDescription')}
        icon={IconCpu}
      >
        <SystemInfoRow label={t('fields.language')} value={browserValue('language')} />
        <SystemInfoRow label={t('fields.clientPlatform')} value={browserValue('platform')} />
        <SystemInfoRow
          label={t('fields.userAgent')}
          value={<span className="font-mono text-xs">{browserValue('userAgent')}</span>}
        />
      </SystemInfoBlock>
    </div>
  );
}
