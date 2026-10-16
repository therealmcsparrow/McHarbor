// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Button } from '@resources/components/ui/Button';
import { InfoRow } from '@resources/components/ui/InfoRow';
import type { LogDisplayRecord } from '@resources/components/logs/logRecords';
import type { ContainerInspect } from '@core/types/docker';
import { ContextDrawer } from './ContextDrawer';
import { TimeTravelContext } from './TimeTravelContext';

type LogContextDrawerProps = {
  record: LogDisplayRecord | null;
  container: ContainerInspect;
  onClose: () => void;
};

export function LogContextDrawer({ record, container, onClose }: LogContextDrawerProps) {
  const { t } = useTranslation('containers');
  return (
    <ContextDrawer
      open={record !== null}
      onOpenChange={(isOpen) => !isOpen && onClose()}
      title={t('logs.lineDetailsTitle')}
      description={t('logs.lineDetailsDescription')}
      footer={<Button onClick={onClose}>{t('logs.openInLogs')}</Button>}
    >
      {record ? (
        <div className="space-y-1">
          <InfoRow label={t('logs.timestamp')}>{record.timestamp || t('logs.unknown')}</InfoRow>
          <InfoRow label={t('logs.severity')}>{record.level}</InfoRow>
          <InfoRow label={t('logs.stream')}>{record.stream}</InfoRow>
          <InfoRow label={t('logs.message')}>
            <span className="font-mono text-xs break-all whitespace-pre-wrap">{record.message}</span>
          </InfoRow>
          <TimeTravelContext container={container} timestamp={record.timestamp} />
        </div>
      ) : null}
    </ContextDrawer>
  );
}
