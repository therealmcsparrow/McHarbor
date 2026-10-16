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
import type { LogForwarder, LogForwarderAdapter } from '../hooks/useLogForwarders';
import { useLogForwarderForm } from '../hooks/useLogForwarderForm';
import { LogForwarderFormFields } from './LogForwarderFormFields';
import { LogForwarderTypePicker } from './LogForwarderTypePicker';

type LogForwarderDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Every installed adapter — the picker offers all of them. */
  adapters: LogForwarderAdapter[];
  /** Set when editing an existing forwarder; skips the type step. */
  forwarder?: LogForwarder | null;
};

/**
 * Add or edit one audit log forwarder.
 *
 * Two steps, mirroring StorageLocationDialog: pick a type, then fill its
 * form. Editing opens straight on the form, because the type is the
 * adapter binding — rebinding a saved forwarder to a different transport
 * would silently reinterpret every stored config key, so changing type
 * means adding a new forwarder and deleting the old one.
 *
 * Every adapter stays on offer no matter how many forwarders already use
 * it: two SIEM collectors or two syslog daemons is a normal setup, not
 * an edge case.
 */
export function LogForwarderDialog({ open: isOpen, onOpenChange, adapters, forwarder }: LogForwarderDialogProps) {
  const { t } = useTranslation('settings');
  const isEdit = !!forwarder;

  const {
    step,
    setStep,
    adapter,
    name: forwarderName,
    setName,
    values,
    setValue,
    enabled,
    setEnabled,
    showErrors,
    fields,
    isMissing,
    saving,
    selectAdapter,
    submit,
  } = useLogForwarderForm({
    open: isOpen,
    adapters,
    forwarder,
    onSaved: () => onOpenChange(false),
    onSuccessToast: (edited) => toast.success(edited ? t('logging.saved') : t('logging.created')),
  });

  return (
    <Dialog open={isOpen} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>
            {isEdit
              ? t('logging.editForwarder', { name: forwarder?.name ?? '' })
              : step === 'type'
                ? t('logging.selectType')
                : t('logging.configuration')}
          </DialogTitle>
          <DialogDescription>
            {step === 'type'
              ? t('logging.selectTypeDescription')
              : (adapter?.description ?? t('logging.configurationDescription'))}
          </DialogDescription>
        </DialogHeader>

        {step === 'type' && <LogForwarderTypePicker adapters={adapters} onSelect={selectAdapter} />}

        {step === 'config' && (
          <div className="space-y-4">
            <LogForwarderFormFields
              name={forwarderName}
              onNameChange={setName}
              fields={fields}
              values={values}
              onValueChange={setValue}
              showErrors={showErrors}
              isMissing={isMissing}
            />
            <label className="flex items-center gap-3 text-sm text-foreground">
              <Switch checked={enabled} onCheckedChange={setEnabled} />
              {t('logging.enableForwarding')}
            </label>
          </div>
        )}

        <DialogFooter>
          {step === 'config' && !isEdit && (
            <Button variant="outline" onClick={() => setStep('type')}>
              {t('logging.back')}
            </Button>
          )}
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('logging.cancel')}
          </Button>
          {step === 'config' && (
            <Button permission={PERM.SETTINGS_MANAGE} onClick={submit} disabled={saving}>
              {saving ? <Spinner size="sm" /> : null}
              {t('logging.save')}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
