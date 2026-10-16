// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconRefresh } from '@tabler/icons-react';
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
import { Input } from '@resources/components/ui/Input';
import { PasswordInput } from '@resources/components/ui/PasswordInput';
import { Tooltip, TooltipContent, TooltipTrigger } from '@resources/components/ui/Tooltip';
import { useEnvironmentStore } from '@resources/stores/environment';
import { PERM } from '@core/auth/permissionKeys';
import { usePermission } from '@core/auth/usePermission';
import { SECRET_NAME_PATTERN, type Secret, type SecretKind, type SecretScope } from '../types';
import { SecretFormCreateFields } from './SecretFormCreateFields';
import { useCreateSecret, useGenerateSecretValue, useRotateSecret } from '@resources/hooks/useSecrets';

type SecretFormDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Null creates; a secret rotates that secret's value. */
  secret?: Secret | null;
  groupSuggestions?: string[];
  kindSuggestions?: string[];
};

/**
 * Create and rotate share a dialog because they differ in one field: rotate
 * cannot change the name or the scope, only the value.
 */
export function SecretFormDialog({
  open: isOpen,
  onOpenChange,
  secret,
  groupSuggestions = [],
  kindSuggestions = [],
}: SecretFormDialogProps) {
  const { t } = useTranslation('secrets');
  const envId = useEnvironmentStore((s) => s.currentId);
  const canManageGlobal = usePermission(PERM.SECRETS_MANAGE_GLOBAL);

  const createSecret = useCreateSecret();
  const rotateSecret = useRotateSecret();
  const generateValue = useGenerateSecretValue();

  const isRotate = Boolean(secret);
  const [itemName, setName] = useState('');
  const [description, setDescription] = useState('');
  const [kind, setKind] = useState<SecretKind>('env');
  const [scope, setScope] = useState<SecretScope>('environment');
  const [stackName, setStackName] = useState('');
  const [group, setGroup] = useState('');
  const [value, setValue] = useState('');
  const [autoApply, setAutoApply] = useState(false);

  const [seeded, setSeeded] = useState<{ o: boolean; s: Secret | null | undefined; g: boolean }>({
    o: false,
    s: undefined,
    g: false,
  });
  if (seeded.o !== isOpen || seeded.s !== secret || seeded.g !== canManageGlobal) {
    setSeeded({ o: isOpen, s: secret, g: canManageGlobal });
    if (isOpen) {
      setName(secret?.name ?? '');
      setDescription(secret?.description ?? '');
      setKind(secret?.kind ?? 'env');
      // A user without manage_global cannot save a global secret, so the form
      // does not open on one.
      setScope(secret?.scope ?? (canManageGlobal ? 'global' : 'environment'));
      setStackName(secret?.stackName ?? '');
      setGroup(secret?.group ?? '');
      setAutoApply(secret?.autoApply ?? false);
      setValue('');
    }
  }

  const nameValid = SECRET_NAME_PATTERN.test(itemName);
  const pending = createSecret.isPending || rotateSecret.isPending;
  const canSubmit = value.trim() !== '' && (isRotate || nameValid) && !pending;

  const handleGenerate = () => {
    generateValue.mutate(undefined, { onSuccess: (minted) => setValue(minted) });
  };

  const handleSubmit = () => {
    if (!canSubmit) return;
    if (isRotate && secret) {
      rotateSecret.mutate({ id: secret.id, value }, { onSuccess: () => onOpenChange(false) });
      return;
    }
    createSecret.mutate(
      {
        name: itemName,
        description,
        kind,
        scope,
        environmentId: scope === 'global' ? null : envId,
        stackName: scope === 'stack' ? stackName : null,
        group: group.trim(),
        value,
        autoApply,
      },
      { onSuccess: () => onOpenChange(false) },
    );
  };

  return (
    <Dialog open={isOpen} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{isRotate ? t('rotate.title') : t('create.title')}</DialogTitle>
          <DialogDescription>
            {isRotate ? t('rotate.description', { name: secret?.name ?? '' }) : t('create.description')}
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-3">
          {!isRotate && (
            <FormField label={t('fields.name')} description={t('fields.nameHint')}>
              <Input
                variant="outline"
                value={itemName}
                onChange={(eventItem) => setName(eventItem.target.value.toUpperCase())}
                placeholder={t('fields.namePlaceholder')}
                aria-invalid={itemName !== '' && !nameValid}
              />
            </FormField>
          )}

          <FormField label={t('fields.value')}>
            <div className="flex items-center gap-2">
              <PasswordInput
                className="flex-1"
                value={value}
                onChange={(evt) => setValue(evt.target.value)}
                placeholder={t('fields.valuePlaceholder')}
              />
              <Tooltip>
                <TooltipTrigger asChild>
                  <Button
                    variant="outline"
                    size="icon"
                    onClick={handleGenerate}
                    disabled={generateValue.isPending}
                    aria-label={t('fields.generate')}
                  >
                    <IconRefresh className="size-4" />
                  </Button>
                </TooltipTrigger>
                <TooltipContent>{t('fields.generate')}</TooltipContent>
              </Tooltip>
            </div>
          </FormField>

          {!isRotate && (
            <>
              <SecretFormCreateFields
                kind={kind}
                onKindChange={setKind}
                scope={scope}
                onScopeChange={setScope}
                stackName={stackName}
                onStackNameChange={setStackName}
                group={group}
                onGroupChange={setGroup}
                groupSuggestions={groupSuggestions}
                kindSuggestions={kindSuggestions}
                description={description}
                onDescriptionChange={setDescription}
                autoApply={autoApply}
                onAutoApplyChange={setAutoApply}
                canManageGlobal={canManageGlobal}
              />
            </>
          )}
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('actions.cancel', { ns: 'common' })}
          </Button>
          <Button permission={PERM.SECRETS_MANAGE} onClick={handleSubmit} disabled={!canSubmit}>
            {isRotate ? t('rotate.submit') : t('create.submit')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
