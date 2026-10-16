// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert } from '@resources/components/ui/Alert';
import { Button } from '@resources/components/ui/Button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { Select } from '@resources/components/ui/Select';
import { PERM } from '@core/auth/permissionKeys';
import { useGitRepos } from '@resources/hooks/useGitRepos';
import { useEnvironmentStore } from '@resources/stores/environment';
import { useCreateSource } from '../hooks/useSources';
import { refusalText } from '../hooks/refusal';

type CreateSourceDialogProps = {
  onOpenChange: (open: boolean) => void;
};

export function CreateSourceDialog({ onOpenChange }: CreateSourceDialogProps) {
  const { t } = useTranslation('reconciler');
  const envId = useEnvironmentStore((s) => s.currentId) ?? '';
  const repos = useGitRepos();
  const create = useCreateSource();
  const [form, setForm] = useState({ name: '', repoId: '', branch: 'main', path: 'desired' });
  const ready = form.name.trim() !== '' && form.repoId !== '' && envId !== '';

  return (
    <Dialog open onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('sources.addTitle')}</DialogTitle>
          <DialogDescription>{t('sources.addDescription')}</DialogDescription>
        </DialogHeader>
        <div className="space-y-3">
          <div>
            <Label htmlFor="src-name" className="mb-2">
              {t('sources.name')}
            </Label>
            <Input id="src-name" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} />
          </div>
          <div>
            <Label className="mb-2">{t('sources.repository')}</Label>
            <Select
              value={form.repoId}
              onChange={(repoId) => setForm({ ...form, repoId })}
              ariaLabel={t('sources.repository')}
              placeholder={t('sources.repositoryPlaceholder')}
              options={(repos.data ?? []).map((r) => ({ value: r.id, label: `${r.name} (${r.url})` }))}
            />
          </div>
          <div className="grid grid-cols-2 gap-3">
            <div>
              <Label htmlFor="src-branch" className="mb-2">
                {t('sources.branch')}
              </Label>
              <Input
                id="src-branch"
                value={form.branch}
                onChange={(e) => setForm({ ...form, branch: e.target.value })}
              />
            </div>
            <div>
              <Label htmlFor="src-path" className="mb-2">
                {t('sources.path')}
              </Label>
              <Input id="src-path" value={form.path} onChange={(e) => setForm({ ...form, path: e.target.value })} />
            </div>
          </div>
          <p className="text-xs text-muted-foreground">{t('sources.formatHint')}</p>
          {create.error && <Alert variant="destructive">{refusalText(create.error, t)}</Alert>}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('actions.cancel', { ns: 'common' })}
          </Button>
          <Button
            permission={PERM.RECONCILER_MANAGE}
            disabled={!ready || create.isPending}
            onClick={() =>
              !create.isPending &&
              create.mutate(
                {
                  name: form.name.trim(),
                  repoId: form.repoId,
                  branch: form.branch.trim(),
                  path: form.path.trim(),
                  envId,
                  enabled: true,
                },
                { onSuccess: () => onOpenChange(false) },
              )
            }
          >
            {create.isPending ? t('actions.processing', { ns: 'common' }) : t('actions.create', { ns: 'common' })}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
