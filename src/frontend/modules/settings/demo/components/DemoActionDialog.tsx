// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { MutationConfirmDialog } from '@resources/components/ui/MutationConfirmDialog';
import { PERM } from '@core/auth/permissionKeys';
import { useDemoAction } from '@resources/hooks/useDemoMode';
import { useDemoActionCopy } from '../hooks/useDemoActionCopy';
import type { DemoAction, DemoOutcome } from '@resources/types/demo';

type DemoActionDialogProps = {
  action: DemoAction;
  active: boolean;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSuccess?: (outcome: DemoOutcome) => void;
};

export function DemoActionDialog({ action, active, open: isOpen, onOpenChange, onSuccess }: DemoActionDialogProps) {
  const mutation = useDemoAction(action);
  const copy = useDemoActionCopy(action, active);

  return (
    <MutationConfirmDialog
      open={isOpen}
      onOpenChange={onOpenChange}
      title={copy.title}
      description={copy.description}
      confirmLabel={copy.label}
      variant={action === 'reset' ? 'destructive' : 'default'}
      permission={PERM.SYSTEM_DEMO}
      mutation={mutation}
      variables={undefined}
      onSuccess={onSuccess}
    />
  );
}
