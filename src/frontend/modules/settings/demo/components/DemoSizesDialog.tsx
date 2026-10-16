// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { MutationConfirmDialog } from '@resources/components/ui/MutationConfirmDialog';
import { PERM } from '@core/auth/permissionKeys';
import type { DemoConfigProblems } from '@resources/constants/demoConfig';
import { useDemoConfigForm } from '../hooks/useDemoConfigForm';
import { DemoConfigRefusal, useDemoAction } from '@resources/hooks/useDemoMode';
import { useDemoSizesCopy } from '../hooks/useDemoActionCopy';
import type { DemoOutcome, DemoSizes, DemoSizesMode } from '@resources/types/demo';
import { DemoConfigFields } from './DemoConfigFields';

type DemoSizesDialogProps = {
  mode: DemoSizesMode;
  sizes: DemoSizes;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSuccess?: (outcome: DemoOutcome) => void;
};

export function DemoSizesDialog({ mode, sizes, open: isOpen, onOpenChange, onSuccess }: DemoSizesDialogProps) {
  const mutation = useDemoAction(mode === 'enter' ? 'enter' : 'reset');
  const copy = useDemoSizesCopy(mode);
  const form = useDemoConfigForm(sizes, mutation.reset);
  const refusal = mutation.error instanceof DemoConfigRefusal ? mutation.error : undefined;
  const problems: DemoConfigProblems = refusal
    ? { [refusal.field]: { kind: 'range', min: refusal.min, max: refusal.max }, ...form.problems }
    : form.problems;

  return (
    <MutationConfirmDialog
      open={isOpen}
      onOpenChange={onOpenChange}
      title={copy.title}
      description={copy.description}
      confirmLabel={copy.label}
      variant={mode === 'resize' ? 'destructive' : 'default'}
      permission={PERM.SYSTEM_DEMO}
      mutation={mutation}
      variables={form.config ? { config: form.config } : undefined}
      disabled={!form.config}
      onSuccess={onSuccess}
    >
      <DemoConfigFields form={form} sizes={sizes} problems={problems} />
    </MutationConfirmDialog>
  );
}
