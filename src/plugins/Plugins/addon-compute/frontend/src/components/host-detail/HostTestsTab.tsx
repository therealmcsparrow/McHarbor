// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Button } from '@resources/components/ui/Button';
import { COMPUTE_PERM } from '../../permissions';
import { CapabilityPanel } from '../CapabilityPanel';
import { HostReadinessChecklist } from './HostReadinessChecklist';
import { readinessRows } from './hostReadiness';
import type { OverviewEntry } from '../../types';

interface HostTestsTabProps {
  entry: OverviewEntry;
  onSetup: (environmentId: string) => void;
}

export function HostTestsTab({ entry, onSetup }: HostTestsTabProps) {
  const { t } = useTranslation('compute');

  return (
    <div className="space-y-4">
      <HostReadinessChecklist rows={readinessRows(entry)} />

      <CapabilityPanel
        environmentId={entry.environmentId}
        capability={entry.capability}
        probedAt={entry.capability?.probedAt}
        available={entry.available}
        unavailableReason={entry.unavailableReason}
        agentVersionCurrent={entry.capability?.agentVersion}
      />

      <Button variant="outline" permission={COMPUTE_PERM.MANAGE} onClick={() => onSetup(entry.environmentId)}>
        {t('actions.runWizard')}
      </Button>
    </div>
  );
}
