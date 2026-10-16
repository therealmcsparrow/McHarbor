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
import { FormField } from '@resources/components/ui/FormField';
import { Input } from '@resources/components/ui/Input';
import { useEnvironmentStore } from '@resources/stores/environment';
import { PERM } from '@core/auth/permissionKeys';
import { useCreateSecret } from '@resources/hooks/useSecrets';
import { secretReference } from '@core/types/secrets';
import { SECRET_NAME_PATTERN } from '@resources/utils/secret-name';

type PromoteToSecretDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** The environment variable being promoted. */
  envKey: string;
  /** Its current plaintext value. */
  value: string;
  /** The stack the variable belongs to — the promoted secret is scoped to it. */
  stackName: string;
  /** Called with the `${secret:NAME}` reference to write back into the editor. */
  onPromoted: (reference: string) => void;
};

/**
 * Moves one environment variable's value into the vault and replaces it with a
 * reference.
 *
 * Deliberately per-variable and explicit. An automatic sweep of every existing
 * env var into the vault would silently change who can see what — the value
 * stops being readable in the ordinary editor and reading it again costs an
 * approved elevation. That is a decision for the operator, one variable at a
 * time, with the consequence stated.
 */
export function PromoteToSecretDialog({
  open: isOpen,
  onOpenChange,
  envKey,
  value,
  stackName,
  onPromoted,
}: PromoteToSecretDialogProps) {
  const { t } = useTranslation('secrets');
  const envId = useEnvironmentStore((s) => s.currentId);
  const createSecret = useCreateSecret();

  const [itemName, setName] = useState('');
  const seedKey = `${isOpen}|${envKey}`;
  const [lastSeedKey, setLastSeedKey] = useState('');
  if (lastSeedKey !== seedKey) {
    setLastSeedKey(seedKey);
    if (isOpen) setName(envKey.toUpperCase());
  }

  const nameValid = SECRET_NAME_PATTERN.test(itemName);

  const handlePromote = () => {
    if (!nameValid) return;
    createSecret.mutate(
      {
        name: itemName,
        kind: 'env',
        scope: 'stack',
        environmentId: envId,
        stackName,
        value,
      },
      {
        onSuccess: () => {
          onPromoted(secretReference(itemName));
          onOpenChange(false);
        },
      },
    );
  };

  return (
    <Dialog open={isOpen} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('actions.promoteTitle', { key: envKey })}</DialogTitle>
          <DialogDescription>{t('actions.promoteBody')}</DialogDescription>
        </DialogHeader>

        <div className="space-y-3">
          <Alert variant="warning" title={t('actions.promote')}>
            {t('actions.promoteBody')}
          </Alert>
          <FormField label={t('fields.name')} description={t('fields.nameHint')}>
            <Input
              variant="outline"
              value={itemName}
              onChange={(evt) => setName(evt.target.value.toUpperCase())}
              aria-invalid={itemName !== '' && !nameValid}
            />
          </FormField>
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('actions.cancel', { ns: 'common' })}
          </Button>
          <Button
            permission={PERM.SECRETS_MANAGE}
            onClick={handlePromote}
            disabled={!nameValid || createSecret.isPending}
          >
            {t('actions.promoteConfirm')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
