// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconCopy } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { Select } from '@resources/components/ui/Select';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from '@resources/components/ui/Dialog';
import { useCreateAPIKey, type CreateAPIKeyResult } from '../hooks/useAPIKeys';

type CreateAPIKeyDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onCreated: (result: CreateAPIKeyResult) => void;
};

export function CreateAPIKeyDialog({ open, onOpenChange, onCreated }: CreateAPIKeyDialogProps) {
  const { t } = useTranslation('security');
  const { t: tc } = useTranslation('common');
  const createKey = useCreateAPIKey();
  const [name, setName] = useState('');
  const [expiry, setExpiry] = useState('');

  const expiryOptions = [
    { value: '', label: t('apiKeys.expirationOptions.never') },
    { value: '30', label: t('apiKeys.expirationOptions.30days') },
    { value: '90', label: t('apiKeys.expirationOptions.90days') },
    { value: '365', label: t('apiKeys.expirationOptions.1year') },
  ];

  const handleSubmit = () => {
    const expiresAt = expiry
      ? new Date(Date.now() + parseInt(expiry) * 86400000).toISOString()
      : undefined;

    createKey.mutate({ name, scopes: [], expiresAt }, {
      onSuccess: (data) => {
        onCreated(data as CreateAPIKeyResult);
        setName('');
        setExpiry('');
      },
    });
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('apiKeys.createKey')}</DialogTitle>
        </DialogHeader>
        <div className="space-y-3 py-2">
          <Input placeholder={t('apiKeys.name')} value={name} onChange={(e) => setName(e.target.value)} />
          <Select value={expiry} onChange={setExpiry} options={expiryOptions} />
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>{tc('actions.cancel')}</Button>
          <Button onClick={handleSubmit} disabled={!name || createKey.isPending}>
            {createKey.isPending ? tc('actions.processing') : tc('actions.create')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

type APIKeyTokenDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  keyResult: CreateAPIKeyResult;
};

export function APIKeyTokenDialog({ open, onOpenChange, keyResult }: APIKeyTokenDialogProps) {
  const { t } = useTranslation('security');
  const { t: tc } = useTranslation('common');
  const [copied, setCopied] = useState(false);

  const handleCopy = async () => {
    await navigator.clipboard.writeText(keyResult.key);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('apiKeys.createKey')}</DialogTitle>
          <DialogDescription>{t('apiKeys.keyWarning')}</DialogDescription>
        </DialogHeader>

        <div className="space-y-3 py-2">
          <div className="flex items-center gap-2 rounded-lg border border-border bg-muted/50 p-3">
            <code className="flex-1 break-all text-sm font-mono">{keyResult.key}</code>
            <Button variant="outline" size="sm" onClick={handleCopy} aria-label={t('apiKeys.copyKey')}>
              <IconCopy className="size-4" />
            </Button>
          </div>
          {copied && (
            <p className="text-sm text-green-500">{t('apiKeys.keyCopied')}</p>
          )}
        </div>

        <DialogFooter>
          <Button onClick={() => onOpenChange(false)}>
            {tc('actions.close')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

