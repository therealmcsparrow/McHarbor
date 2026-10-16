// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from '@resources/components/ui/Dialog';
import type { Workflow } from '../types';
import type { UseMutationResult } from '@tanstack/react-query';
import { PERM } from '@core/auth/permissionKeys';

type CreateWorkflowDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  createWorkflow: UseMutationResult<Workflow, Error, { name: string; description?: string }>;
};

export function CreateWorkflowDialog({ open: isOpen, onOpenChange, createWorkflow }: CreateWorkflowDialogProps) {
  const { t } = useTranslation('common');
  const navigate = useNavigate();
  const [workflowName, setWorkflowName] = useState('');
  const [description, setDescription] = useState('');

  const handleCreate = () => {
    if (!workflowName.trim()) return;
    createWorkflow.mutate(
      { name: workflowName.trim(), description: description.trim() },
      {
        onSuccess: (workflow) => {
          onOpenChange(false);
          setWorkflowName('');
          setDescription('');
          if (workflow?.id) void navigate(`/workflows/${workflow.id}`);
        },
      },
    );
  };

  return (
    <Dialog open={isOpen} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('workflows.createTitle')}</DialogTitle>
          <DialogDescription>{t('workflows.createDescription')}</DialogDescription>
        </DialogHeader>
        <div className="space-y-3">
          <div>
            <Label className="mb-2">{t('workflows.nameLabel')}</Label>
            <Input
              type="text"
              value={workflowName}
              onChange={(e) => setWorkflowName(e.target.value)}
              placeholder={t('workflows.namePlaceholder')}
            />
          </div>
          <div>
            <Label className="mb-2">{t('workflows.descriptionLabel')}</Label>
            <Input
              type="text"
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              placeholder={t('workflows.descriptionPlaceholder')}
            />
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('actions.cancel')}
          </Button>
          <Button
            permission={PERM.WORKFLOWS_MANAGE}
            onClick={handleCreate}
            disabled={createWorkflow.isPending || !workflowName.trim()}
          >
            {createWorkflow.isPending ? t('workflows.creating') : t('actions.create')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
