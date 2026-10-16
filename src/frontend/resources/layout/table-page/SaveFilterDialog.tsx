// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useRef, useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@resources/components/ui/Button';
import { Checkbox } from '@resources/components/ui/Checkbox';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { Input } from '@resources/components/ui/Input';
import { describeApiError } from '@resources/utils/api-mutation';

type SaveFilterDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  mode: 'create' | 'rename';
  initialName: string;
  onSubmit: (name: string, isDefault: boolean) => Promise<void>;
};

export function SaveFilterDialog({ open: isOpen, onOpenChange, mode, initialName, onSubmit }: SaveFilterDialogProps) {
  const { t } = useTranslation('common');
  const [itemName, setName] = useState(initialName);
  const [isDefault, setIsDefault] = useState(false);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);

  const inputRef = useRef<HTMLInputElement>(null);
  const resetKey = isOpen ? initialName : null;
  const [appliedResetKey, setAppliedResetKey] = useState<string | null>(null);
  if (appliedResetKey !== resetKey) {
    setAppliedResetKey(resetKey);
    if (isOpen) {
      setName(initialName);
      setIsDefault(false);
      setError('');
    }
  }

  const trimmed = itemName.trim();
  const tooLong = trimmed.length > 80;

  const submit = (evt: FormEvent) => {
    evt.preventDefault();
    if (!trimmed || tooLong) return;
    setBusy(true);
    onSubmit(trimmed, isDefault)
      .then(() => onOpenChange(false))
      .catch((err: unknown) => setError(err instanceof Error ? err.message : describeApiError({})))
      .finally(() => setBusy(false));
  };

  return (
    <Dialog open={isOpen} onOpenChange={onOpenChange}>
      <DialogContent
        className="sm:max-w-md"
        onOpenAutoFocus={(evt) => {
          evt.preventDefault();
          inputRef.current?.focus();
        }}
      >
        <form onSubmit={submit}>
          <DialogHeader>
            <DialogTitle>{mode === 'create' ? t('tableViews.saveCurrent') : t('tableViews.rename')}</DialogTitle>
          </DialogHeader>
          <DialogBody className="flex flex-col gap-3">
            <Input
              ref={inputRef}
              value={itemName}
              onChange={(evt) => setName(evt.target.value)}
              aria-label={t('tableViews.nameLabel')}
              placeholder={t('tableViews.nameLabel')}
              aria-invalid={Boolean(error) || tooLong}
              variant="outline"
            />
            {tooLong && <p className="text-xs text-destructive">{t('tableViews.nameTooLong')}</p>}
            {error && <p className="text-xs text-destructive">{error}</p>}
            {mode === 'create' && (
              <label className="flex items-center gap-2 text-sm text-foreground">
                <Checkbox checked={isDefault} onCheckedChange={(checked) => setIsDefault(checked === true)} />
                {t('tableViews.useAsDefault')}
              </label>
            )}
          </DialogBody>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              {t('actions.cancel')}
            </Button>
            <Button type="submit" disabled={!trimmed || tooLong || busy}>
              {t('actions.save')}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
