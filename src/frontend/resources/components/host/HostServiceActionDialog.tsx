// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Alert } from '@resources/components/ui/Alert';
import { MutationConfirmDialog } from '@resources/components/ui/MutationConfirmDialog';
import { useHostServiceAction } from '@resources/hooks/useHostServices';
import type { HostTarget } from '@resources/hooks/useHostUpdates';
import { hostPermission } from '@resources/hooks/useHostUpdates';

export type HostServiceConfirmAction = 'stop' | 'restart' | 'disable';

export type HostServiceActionTarget = {
  unit: string;
  action: HostServiceConfirmAction;
  isAgent?: boolean;
  restartsMcHarbor?: boolean;
};

type HostServiceActionDialogProps = {
  host: HostTarget;
  target: HostServiceActionTarget | null;
  onOpenChange: (open: boolean) => void;
  onSuccess?: () => void;
};

export function HostServiceActionDialog({ host, target, onOpenChange, onSuccess }: HostServiceActionDialogProps) {
  const { t } = useTranslation('environments');
  const mutation = useHostServiceAction(host);
  const action = target?.action ?? 'stop';
  const unit = target?.unit ?? '';
  const agentRestart = action === 'restart' && Boolean(target?.isAgent);
  const mcharborRestart = action === 'restart' && Boolean(target?.restartsMcHarbor);

  return (
    <MutationConfirmDialog
      open={target !== null}
      onOpenChange={onOpenChange}
      title={t(`hostServices.confirm.${action}.title`, { unit })}
      description={t(`hostServices.confirm.${action}.description`, { unit })}
      confirmLabel={t(`actions.${action}`, { ns: 'common' })}
      variant={action === 'restart' ? 'caution' : 'destructive'}
      permission={hostPermission(host, 'servicesManage')}
      mutation={mutation}
      variables={{ unit, action }}
      onSuccess={() => onSuccess?.()}
    >
      {agentRestart && <Alert variant="warning" description={t('hostServices.confirm.agentRestartWarning')} />}
      {mcharborRestart && (
        <Alert
          variant="warning"
          description={t('hostServices.confirm.mcharborRestartWarning', {
            unit,
          })}
        />
      )}
    </MutationConfirmDialog>
  );
}
