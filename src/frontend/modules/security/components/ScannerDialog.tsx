// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { Button } from '@resources/components/ui/Button';
import { Spinner } from '@resources/components/ui/Spinner';
import { Switch } from '@resources/components/ui/Switch';
import { PERM } from '@core/auth/permissionKeys';
import type { ScannerAdapter, ScannerConfig } from '../hooks/useScanners';
import { useScannerForm } from '../hooks/useScannerForm';
import { ScannerEnginePicker } from './ScannerEnginePicker';
import { ScannerFormFields } from './ScannerFormFields';

type ScannerDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Every installed engine — the picker offers all of them. */
  adapters: ScannerAdapter[];
  /** Set when editing an existing scanner; skips the engine step. */
  scanner?: ScannerConfig | null;
};

/**
 * Add or edit one vulnerability scanner.
 *
 * Two steps, mirroring the storage, communication and logging dialogs:
 * pick an engine, then fill its form. Editing opens straight on the
 * form, because the engine is the adapter binding — rebinding a saved
 * scanner to a different engine would silently reinterpret every stored
 * config key under another adapter's schema, so changing engine means
 * adding a new scanner and deleting the old one.
 *
 * Every engine stays on offer no matter how many scanners already use
 * it: a strict and a lenient instance of the same engine, or a
 * production and a staging one, is a normal setup rather than an edge case.
 */
export function ScannerDialog({ open: isOpen, onOpenChange, adapters, scanner }: ScannerDialogProps) {
  const { t } = useTranslation('security');
  const isEdit = !!scanner;

  const {
    step,
    setStep,
    adapter,
    name: itemName,
    setName,
    values,
    setValue,
    enabled,
    setEnabled,
    makeDefault,
    setMakeDefault,
    showErrors,
    fields,
    isMissing,
    saving,
    selectAdapter,
    submit,
  } = useScannerForm({
    open: isOpen,
    adapters,
    scanner,
    onSaved: () => onOpenChange(false),
    onSuccessToast: (edited) => toast.success(edited ? t('scanning.saved') : t('scanning.created')),
  });

  return (
    <Dialog open={isOpen} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>
            {isEdit
              ? t('scanning.editScanner', { name: scanner?.name ?? '' })
              : step === 'type'
                ? t('scanning.selectEngine')
                : t('scanning.configuration')}
          </DialogTitle>
          <DialogDescription>
            {step === 'type'
              ? t('scanning.selectEngineDescription')
              : (adapter?.description ?? t('scanning.configurationDescription'))}
          </DialogDescription>
        </DialogHeader>

        {step === 'type' && <ScannerEnginePicker adapters={adapters} onSelect={selectAdapter} />}

        {step === 'config' && (
          <div className="space-y-4">
            <ScannerFormFields
              name={itemName}
              onNameChange={setName}
              fields={fields}
              values={values}
              onValueChange={setValue}
              showErrors={showErrors}
              isMissing={isMissing}
            />
            <div className="space-y-3 border-t border-border pt-4">
              <label className="flex items-center gap-3 text-sm text-foreground">
                <Switch checked={enabled} onCheckedChange={setEnabled} />
                {t('scanning.enableScanner')}
              </label>
              <label className="flex items-center gap-3 text-sm text-foreground">
                <Switch
                  checked={makeDefault}
                  onCheckedChange={setMakeDefault}
                  // A disabled scanner cannot be the one that runs, so
                  // offering to make it the default would be offering a
                  // setting the backend will refuse to honour.
                  disabled={!enabled}
                />
                {t('scanning.makeDefault')}
              </label>
            </div>
          </div>
        )}

        <DialogFooter>
          {step === 'config' && !isEdit && (
            <Button variant="outline" onClick={() => setStep('type')}>
              {t('scanning.back')}
            </Button>
          )}
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('scanning.cancel')}
          </Button>
          {step === 'config' && (
            <Button permission={PERM.SETTINGS_MANAGE} onClick={submit} disabled={saving}>
              {saving ? <Spinner size="sm" /> : null}
              {t('scanning.save')}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
