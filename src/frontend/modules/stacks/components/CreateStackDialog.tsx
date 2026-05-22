// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
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
import { useEnvironmentStore } from '@resources/stores/environment';
import { api } from '@core/api/client';

type CreateStackDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

export function CreateStackDialog({ open, onOpenChange }: CreateStackDialogProps) {
  const { t } = useTranslation('stacks');
  const { t: tc } = useTranslation('common');

  const [stackName, setStackName] = useState('');
  const [composeFile, setComposeFile] = useState('');
  const [description, setDescription] = useState('');

  const handleCreate = () => {
    if (!stackName.trim() || !composeFile.trim()) return;
    const envId = useEnvironmentStore.getState().currentId;
    api
      .post('/stacks' + (envId ? `?env=${envId}` : ''), {
        name: stackName.trim(),
        compose: composeFile,
        description: description.trim() || undefined,
        autoStart: true,
      })
      .then(() => {
        onOpenChange(false);
        setStackName('');
        setComposeFile('');
        setDescription('');
      });
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>{t('create.title')}</DialogTitle>
          <DialogDescription>{t('create.description')}</DialogDescription>
        </DialogHeader>
        <div className="space-y-3">
          <div>
            <Label className="mb-2">{t('create.name')}</Label>
            <Input
              type="text"
              value={stackName}
              onChange={(e) => setStackName(e.target.value)}
              placeholder={t('create.namePlaceholder')}
            />
          </div>
          <div>
            <Label className="mb-2">{t('create.descriptionLabel')}</Label>
            <Input
              type="text"
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              placeholder={t('create.descriptionPlaceholder')}
            />
          </div>
          <div>
            <Label className="mb-2">{t('create.composeFile')}</Label>
            <textarea
              value={composeFile}
              onChange={(e) => setComposeFile(e.target.value)}
              placeholder={`services:\n  web:\n    image: nginx:latest\n    ports:\n      - '80:80'`}
              rows={12}
              className="py-2.5 px-4 block w-full bg-card border border-border rounded-lg font-mono text-sm text-foreground placeholder:text-muted-foreground focus:border-primary focus:ring-primary"
            />
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {tc('actions.cancel')}
          </Button>
          <Button onClick={handleCreate} disabled={!stackName.trim() || !composeFile.trim()}>
            {t('create.submit')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
