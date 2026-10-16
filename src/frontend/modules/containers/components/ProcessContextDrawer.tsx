// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { InfoRow } from '@resources/components/ui/InfoRow';
import type { ContainerService } from '../hooks/useContainerServices';
import { ContextDrawer } from './ContextDrawer';

export type SelectedProcessContext =
  | { kind: 'process'; titles: string[]; values: string[] }
  | { kind: 'service'; service: ContainerService; initSystem: string };

type ProcessContextDrawerProps = {
  context: SelectedProcessContext | null;
  onClose: () => void;
};

export function ProcessContextDrawer({ context, onClose }: ProcessContextDrawerProps) {
  const { t } = useTranslation('containers');
  return (
    <ContextDrawer
      open={context !== null}
      onOpenChange={(isOpen) => !isOpen && onClose()}
      title={context?.kind === 'service' ? t('processes.serviceDetailsTitle') : t('processes.processDetailsTitle')}
      description={
        context?.kind === 'service'
          ? t('processes.serviceDetailsDescription')
          : t('processes.processDetailsDescription')
      }
    >
      {context?.kind === 'service' ? (
        <div className="space-y-1">
          <InfoRow label={t('processes.serviceName')}>{context.service.name}</InfoRow>
          <InfoRow label={t('processes.serviceStatus')}>{context.service.status}</InfoRow>
          <InfoRow label={t('processes.serviceSubStatus')}>{context.service.sub || '-'}</InfoRow>
          <InfoRow label={t('processes.initSystemLabel')}>{context.initSystem || '-'}</InfoRow>
        </div>
      ) : context?.kind === 'process' ? (
        <div className="space-y-1">
          {context.values.map((value, index) => (
            <InfoRow
              key={`${context.titles[index] ?? 'column'}-${context.values[index] ?? ''}`}
              label={context.titles[index] ?? t('processes.column', { number: index + 1 })}
            >
              <span className="font-mono text-xs break-all">{value}</span>
            </InfoRow>
          ))}
        </div>
      ) : null}
    </ContextDrawer>
  );
}
