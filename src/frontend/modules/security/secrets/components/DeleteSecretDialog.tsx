// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useState } from 'react';
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
import { Spinner } from '@resources/components/ui/Spinner';
import { PERM } from '@core/auth/permissionKeys';
import { useDeleteSecret, useSecretReferences } from '@resources/hooks/useSecrets';
import type { Secret } from '../types';

type DeleteSecretDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  secret: Secret | null;
};

/**
 * Deleting a secret that something still references breaks that thing's next
 * deploy, so the dialog shows the reference list first and asks the user to
 * type the name before it will force past it.
 */
export function DeleteSecretDialog({ open: isOpen, onOpenChange, secret }: DeleteSecretDialogProps) {
  const { t } = useTranslation('secrets');
  const deleteSecret = useDeleteSecret();
  const { data: references = [], isLoading } = useSecretReferences(isOpen ? (secret?.id ?? null) : null);

  const [typedName, setTypedName] = useState('');
  const { reset } = deleteSecret;
  const [wasOpen, setWasOpen] = useState(isOpen);
  if (wasOpen !== isOpen) {
    setWasOpen(isOpen);
    if (!isOpen) setTypedName('');
  }
  useEffect(() => {
    if (isOpen) reset();
  }, [isOpen, reset]);

  const inUse = references.length > 0;
  const confirmed = !inUse || typedName === secret?.name;

  const handleDelete = () => {
    if (!secret || !confirmed) return;
    deleteSecret.mutate({ id: secret.id, force: inUse }, { onSuccess: () => onOpenChange(false) });
  };

  return (
    <Dialog open={isOpen} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('delete.title', { name: secret?.name ?? '' })}</DialogTitle>
          <DialogDescription>{t('delete.description')}</DialogDescription>
        </DialogHeader>

        <div className="space-y-3">
          {isLoading ? (
            <div className="flex justify-center py-4">
              <Spinner />
            </div>
          ) : inUse ? (
            <>
              <Alert variant="destructive" title={t('delete.inUseTitle', { count: references.length })}>
                {t('delete.inUseBody')}
              </Alert>
              <ul className="max-h-40 space-y-1 overflow-auto rounded border border-border p-2">
                {references.map((reference) => (
                  <li key={reference.id} className="font-mono text-xs text-muted-foreground">
                    {reference.resourceType} / {reference.resourceId}
                    {reference.field ? ` · ${reference.field}` : ''}
                  </li>
                ))}
              </ul>
              <FormField
                label={t('delete.confirmLabel')}
                description={t('delete.confirmHint', { name: secret?.name ?? '' })}
              >
                <Input
                  variant="outline"
                  value={typedName}
                  onChange={(evt) => setTypedName(evt.target.value)}
                  placeholder={secret?.name}
                />
              </FormField>
            </>
          ) : (
            <p className="text-sm text-muted-foreground">{t('delete.notReferenced')}</p>
          )}
          {deleteSecret.isError && (
            <p
              role="alert"
              className="rounded-md border border-destructive/40 bg-destructive/10 p-3 text-xs break-words whitespace-pre-wrap text-destructive"
            >
              {deleteSecret.error.message}
            </p>
          )}
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('actions.cancel', { ns: 'common' })}
          </Button>
          <Button
            permission={PERM.SECRETS_DELETE}
            variant="destructive"
            onClick={handleDelete}
            disabled={!confirmed || deleteSecret.isPending || Boolean(secret?.system)}
          >
            {t('actions.delete', { ns: 'common' })}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
