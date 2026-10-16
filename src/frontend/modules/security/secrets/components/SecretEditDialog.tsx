// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@resources/components/ui/Button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { FormField } from '@resources/components/ui/FormField';
import { Textarea } from '@resources/components/ui/Textarea';
import { PERM } from '@core/auth/permissionKeys';
import { useUpdateSecret } from '@resources/hooks/useSecrets';
import { SecretCreatableSelect } from './SecretCreatableSelect';
import type { Secret } from '../types';

type SecretEditDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  secret: Secret | null;
  groupSuggestions: string[];
};

export function SecretEditDialog({ open: isOpen, onOpenChange, secret, groupSuggestions }: SecretEditDialogProps) {
  const { t } = useTranslation('secrets');
  const updateSecret = useUpdateSecret();

  const [description, setDescription] = useState('');
  const [group, setGroup] = useState('');

  const [seeded, setSeeded] = useState<{ isOpen: boolean; secret: Secret | null | undefined }>({
    isOpen: false,
    secret: undefined,
  });
  if (seeded.isOpen !== isOpen || seeded.secret !== secret) {
    setSeeded({ isOpen, secret });
    if (isOpen) {
      setDescription(secret?.description ?? '');
      setGroup(secret?.group ?? '');
    }
  }

  const handleSubmit = () => {
    if (!secret) return;
    updateSecret.mutate({ id: secret.id, description, group: group.trim() }, { onSuccess: () => onOpenChange(false) });
  };

  return (
    <Dialog open={isOpen} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('edit.title')}</DialogTitle>
          <DialogDescription>{t('edit.description', { name: secret?.name ?? '' })}</DialogDescription>
        </DialogHeader>

        <div className="space-y-3">
          <FormField label={t('fields.group')} description={t('fields.groupHint')}>
            <SecretCreatableSelect
              value={group}
              onChange={setGroup}
              options={groupSuggestions.map((nameValue) => ({ value: nameValue, label: nameValue }))}
              ariaLabel={t('fields.group')}
              placeholder={t('groups.ungrouped')}
              searchPlaceholder={t('groups.searchPlaceholder')}
              createLabel={(itemName) => t('groups.create', { name: itemName })}
              emptyLabel={t('groups.none')}
              clearLabel={t('groups.clear')}
            />
          </FormField>

          <FormField label={t('fields.description')}>
            <Textarea
              value={description}
              onChange={(evt) => setDescription(evt.target.value)}
              rows={3}
              placeholder={t('fields.descriptionPlaceholder')}
            />
          </FormField>
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('actions.cancel', { ns: 'common' })}
          </Button>
          <Button permission={PERM.SECRETS_MANAGE} onClick={handleSubmit} disabled={updateSecret.isPending}>
            {t('edit.submit')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
