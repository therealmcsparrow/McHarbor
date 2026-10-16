// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMutation } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';
import { RecreateConfirmDialog } from './RecreateConfirmDialog';
import { describeDestructiveDialog } from '../../../test/destructive';

function RecreateHarness({
  onOpenChange,
  onSuccess,
}: {
  onOpenChange: (open: boolean) => void;
  onSuccess: () => void;
}) {
  const recreate = useMutation({
    mutationFn: () => api.post('/containers/c1/recreate', {}).then(assertSuccess),
  });
  return (
    <RecreateConfirmDialog
      open
      onOpenChange={onOpenChange}
      changedFields={['image']}
      loading={recreate.isPending}
      error={recreate.error?.message ?? null}
      secretFileModeOffered={false}
      secretFileMode={false}
      onSecretFileModeChange={() => undefined}
      onConfirm={() =>
        recreate.mutate(undefined, {
          onSuccess: () => {
            onOpenChange(false);
            onSuccess();
          },
        })
      }
    />
  );
}

describeDestructiveDialog({
  name: 'RecreateConfirmDialog',
  method: 'post',
  path: '*/api/containers/c1/recreate',
  permission: 'containers.manage',
  confirmLabel: 'Recreate',
  body: {},
  render: (props) => <RecreateHarness {...props} />,
});
