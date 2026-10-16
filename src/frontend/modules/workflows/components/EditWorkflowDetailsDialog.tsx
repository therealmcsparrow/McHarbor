// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { Textarea } from '@resources/components/ui/Textarea';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from '@resources/components/ui/Dialog';
import { useUpdateWorkflow } from '../hooks/useWorkflows';
import type { Workflow } from '../types';
import { PERM } from '@core/auth/permissionKeys';

type EditWorkflowDetailsDialogProps = {
  workflow: Workflow | null;
  onOpenChange: (open: boolean) => void;
};

export function EditWorkflowDetailsDialog({ workflow, onOpenChange }: EditWorkflowDetailsDialogProps) {
  const { t } = useTranslation('common');
  const [workflowName, setWorkflowName] = useState('');
  const [description, setDescription] = useState('');
  const updateWorkflow = useUpdateWorkflow({
    meta: {
      success: t('workflows.mutationUpdated'),
      error: t('workflows.mutationUpdateFailed'),
    },
  });

  const [syncedWorkflow, setSyncedWorkflow] = useState<Workflow | null>(null);
  if (workflow !== syncedWorkflow) {
    setSyncedWorkflow(workflow);
    if (workflow) {
      setWorkflowName(workflow.name);
      setDescription(workflow.description ?? '');
    }
  }

  const trimmedName = workflowName.trim();

  const handleSave = () => {
    if (!workflow || !trimmedName) return;
    updateWorkflow.mutate(
      { id: workflow.id, name: trimmedName, description: description.trim() },
      { onSuccess: () => onOpenChange(false) },
    );
  };

  return (
    <Dialog open={workflow !== null} onOpenChange={(next) => !next && onOpenChange(false)}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('workflows.editDetailsTitle')}</DialogTitle>
          <DialogDescription>{t('workflows.editDetailsDescription')}</DialogDescription>
        </DialogHeader>
        <div className="space-y-3">
          <div>
            <Label className="mb-2">{t('workflows.nameLabel')}</Label>
            <Input
              type="text"
              value={workflowName}
              onChange={(e) => setWorkflowName(e.target.value)}
              placeholder={t('workflows.namePlaceholder')}
              maxLength={120}
            />
          </div>
          <div>
            <Label className="mb-2">{t('workflows.descriptionLabel')}</Label>
            <Textarea
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              placeholder={t('workflows.descriptionPlaceholder')}
              rows={3}
            />
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('actions.cancel')}
          </Button>
          <Button
            permission={PERM.WORKFLOWS_MANAGE}
            onClick={handleSave}
            disabled={updateWorkflow.isPending || !trimmedName}
          >
            {updateWorkflow.isPending ? t('workflows.saving') : t('actions.save')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
