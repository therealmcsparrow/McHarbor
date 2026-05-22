// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { Textarea } from '@resources/components/ui/Textarea';
import { Label } from '@resources/components/ui/Label';
import { CodeEditor } from '@resources/components/CodeEditor';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from '@resources/components/ui/Dialog';
import { useCreateBlueprint, useUpdateBlueprint, type Blueprint, type BlueprintInput } from '../hooks/useBlueprints';

const CATEGORIES = [
  'database', 'web', 'security', 'infrastructure', 'messaging',
  'mail', 'development', 'media', 'productivity', 'networking', 'automation', 'other',
] as const;

type BlueprintFormDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  blueprint?: Blueprint | null;
};

export function BlueprintFormDialog({ open, onOpenChange, blueprint }: BlueprintFormDialogProps) {
  const { t } = useTranslation('common');
  const isEdit = !!blueprint;

  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [category, setCategory] = useState('other');
  const [version, setVersion] = useState('1.0');
  const [composeYaml, setComposeYaml] = useState('');

  const createMutation = useCreateBlueprint(t('blueprints.createSuccess'));
  const updateMutation = useUpdateBlueprint(t('blueprints.updateSuccess'));
  const isPending = createMutation.isPending || updateMutation.isPending;

  useEffect(() => {
    if (open && blueprint) {
      setName(blueprint.name);
      setDescription(blueprint.description);
      setCategory(blueprint.category || 'other');
      setVersion(blueprint.version || '1.0');
      setComposeYaml(blueprint.composeYaml);
    } else if (open && !blueprint) {
      setName('');
      setDescription('');
      setCategory('other');
      setVersion('1.0');
      setComposeYaml('services:\n  app:\n    image: \n    restart: unless-stopped\n    ports:\n      - "8080:80"\n');
    }
  }, [open, blueprint]);

  const handleSubmit = () => {
    if (!name.trim() || !composeYaml.trim()) return;

    const input: BlueprintInput = {
      name: name.trim(),
      description: description.trim(),
      category,
      icon: '',
      composeYaml,
      envVars: '[]',
      version: version.trim() || '1.0',
    };

    const onSuccess = () => onOpenChange(false);

    if (isEdit) {
      updateMutation.mutate({ id: blueprint.id, input }, { onSuccess });
    } else {
      createMutation.mutate(input, { onSuccess });
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-3xl">
        <DialogHeader>
          <DialogTitle>
            {isEdit ? t('blueprints.editTitle') : t('blueprints.createTitle')}
          </DialogTitle>
        </DialogHeader>

        <div className="space-y-4">
          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-1.5">
              <Label>{t('blueprints.nameLabel')}</Label>
              <Input
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder={t('blueprints.namePlaceholder')}
              />
            </div>
            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-1.5">
                <Label>{t('blueprints.categoryLabel')}</Label>
                <select
                  value={category}
                  onChange={(e) => setCategory(e.target.value)}
                  className="w-full rounded-lg border border-border bg-card px-3 py-2 text-sm text-foreground focus:border-primary focus:ring-primary"
                >
                  {CATEGORIES.map((cat) => (
                    <option key={cat} value={cat}>{cat}</option>
                  ))}
                </select>
              </div>
              <div className="space-y-1.5">
                <Label>{t('blueprints.versionLabel')}</Label>
                <Input
                  value={version}
                  onChange={(e) => setVersion(e.target.value)}
                  placeholder="1.0"
                />
              </div>
            </div>
          </div>

          <div className="space-y-1.5">
            <Label>{t('blueprints.descriptionLabel')}</Label>
            <Textarea
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              placeholder={t('blueprints.descriptionPlaceholder')}
              rows={2}
            />
          </div>

          <div className="space-y-1.5">
            <Label>{t('blueprints.composeLabel')}</Label>
            {open ? (
              <CodeEditor
                value={composeYaml}
                onChange={setComposeYaml}
                language="yaml"
                minHeight="300px"
              />
            ) : null}
          </div>
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('actions.cancel')}
          </Button>
          <Button onClick={handleSubmit} disabled={isPending || !name.trim() || !composeYaml.trim()}>
            {isPending ? t('actions.saving') : isEdit ? t('actions.save') : t('actions.create')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

