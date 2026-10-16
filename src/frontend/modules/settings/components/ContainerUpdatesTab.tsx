// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// ContainerUpdatesTab — the Updates > Containers section: the image update
// policies that decide which containers get pulled and recreated, and on what
// schedule. Was the bottom half of the old single `UpdatesTab`.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconPlus } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { PERM } from '@core/auth/permissionKeys';
import {
  useUpdatePolicies,
  useCreateUpdatePolicy,
  useUpdateUpdatePolicy,
  useDeleteUpdatePolicy,
  type CreatePolicyInput,
} from '../hooks/useUpdates';
import { UpdatePolicyCard } from './UpdatePolicyCard';
import { CreateUpdatePolicyForm } from './CreateUpdatePolicyForm';

export function ContainerUpdatesTab() {
  const { t } = useTranslation('settings');
  const { data: policies = [] } = useUpdatePolicies();
  const createPolicy = useCreateUpdatePolicy();
  const updatePolicy = useUpdateUpdatePolicy();
  const deletePolicy = useDeleteUpdatePolicy();
  const [showDialog, setShowDialog] = useState(false);

  const handleCreate = (input: CreatePolicyInput) =>
    createPolicy.mutateAsync(input).then((res) => {
      setShowDialog(false);
      return res;
    });

  const handleToggle = (id: string, enabled: boolean) => {
    updatePolicy.mutate({ id, enabled: !enabled });
  };

  const handleSaveContainers = (id: string, containerMatch: string) => {
    updatePolicy.mutate({ id, containerMatch });
  };

  const handleDelete = (id: string) => {
    deletePolicy.mutate(id);
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h3 className="text-sm font-semibold text-foreground">{t('updates.title')}</h3>
          <p className="text-sm text-muted-foreground">{t('updates.description')}</p>
        </div>
        <Button permission={PERM.UPDATES_MANAGE} onClick={() => setShowDialog(true)}>
          <IconPlus className="size-4" />
          {t('updates.addPolicy')}
        </Button>
      </div>

      {policies.length === 0 && !showDialog && (
        <p className="py-8 text-center text-sm text-muted-foreground">{t('updates.noPolicies')}</p>
      )}

      <div className="space-y-3">
        {policies.map((policy) => (
          <UpdatePolicyCard
            key={policy.id}
            policy={policy}
            saving={updatePolicy.isPending}
            onToggle={handleToggle}
            onDelete={handleDelete}
            onSaveContainers={handleSaveContainers}
          />
        ))}
      </div>

      {showDialog && (
        <CreateUpdatePolicyForm
          creating={createPolicy.isPending}
          onCreate={handleCreate}
          onCancel={() => setShowDialog(false)}
        />
      )}
    </div>
  );
}
