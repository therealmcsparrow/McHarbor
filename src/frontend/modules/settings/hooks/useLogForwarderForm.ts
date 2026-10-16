// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import {
  useCreateLogForwarder,
  useUpdateLogForwarder,
  type LogForwarder,
  type LogForwarderAdapter,
  type LogForwarderField,
} from './useLogForwarders';

type LogForwarderFormOptions = {
  open: boolean;
  adapters: LogForwarderAdapter[];
  forwarder?: LogForwarder | null;
  onSaved: () => void;
  onSuccessToast: (edited: boolean) => void;
};

/**
 * Draft state for the add/edit forwarder dialog: which adapter, the field
 * values, the enable switch — plus the save call.
 */
export function useLogForwarderForm({
  open: isOpen,
  adapters,
  forwarder,
  onSaved,
  onSuccessToast,
}: LogForwarderFormOptions) {
  const [step, setStep] = useState<'type' | 'config'>('type');
  const [adapter, setAdapter] = useState<LogForwarderAdapter | null>(null);
  const [itemName, setName] = useState('');
  const [values, setValues] = useState<Record<string, string>>({});
  const [enabled, setEnabled] = useState(false);
  // Required-field errors stay hidden until the first save attempt, so
  // a freshly opened form is not a wall of red before anything has been
  // typed.
  const [showErrors, setShowErrors] = useState(false);

  const create = useCreateLogForwarder();
  const update = useUpdateLogForwarder();
  const saving = create.isPending || update.isPending;

  const [seeded, setSeeded] = useState<{ isOpen: boolean; forwarder: unknown; adapters: unknown }>({
    isOpen: false,
    forwarder: undefined,
    adapters: undefined,
  });
  if (seeded.isOpen !== isOpen || seeded.forwarder !== forwarder || seeded.adapters !== adapters) {
    setSeeded({ isOpen, forwarder, adapters });
    if (isOpen) {
      setShowErrors(false);
      setName(forwarder?.name ?? '');
      setEnabled(forwarder?.enabled ?? false);
      if (forwarder) {
        setAdapter(adapters.find((a) => a.forwarderType === forwarder.forwarderType) ?? null);
        setValues(Object.fromEntries(forwarder.fields.map((field) => [field.key, field.value])));
        setStep('config');
      } else {
        setAdapter(null);
        setValues({});
        setStep('type');
      }
    }
  }

  // The stored row's fields carry `hasValue` for secrets; the adapter's
  // schema does not. Editing must use the row's copy or a stored secret
  // would read as an empty required field.
  const fields: LogForwarderField[] = forwarder?.fields ?? adapter?.fields ?? [];

  /**
   * A required field is missing when it is empty — except a secret that
   * already has a stored value, where blank means "keep it" rather than
   * "clear it", the same rule the backend applies on save.
   */
  function isMissing(field: LogForwarderField): boolean {
    if (!field.required) return false;
    if (field.type === 'secret' && field.hasValue) return false;
    return (values[field.key] ?? '').trim() === '';
  }

  const incomplete = itemName.trim() === '' || fields.some(isMissing);

  function selectAdapter(item: LogForwarderAdapter) {
    setAdapter(item);
    // Seed from the adapter's declared defaults so the form opens on a
    // working configuration rather than a wall of empty boxes.
    setValues(
      Object.fromEntries(
        item.fields.map((field) => [
          field.key,
          field.default !== undefined && field.default !== null ? String(field.default) : '',
        ]),
      ),
    );
    setName('');
    setEnabled(false);
    setShowErrors(false);
    setStep('config');
  }

  function submit() {
    if (!adapter && !forwarder) return;
    // Saving with an empty endpoint used to create a row that listed as
    // a configured sink and forwarded nothing. Stop it here as well as
    // on the backend.
    if (incomplete) {
      setShowErrors(true);
      return;
    }
    const body = { name: itemName.trim(), enabled, config: values };
    const done = {
      onSuccess: () => {
        onSuccessToast(!!forwarder);
        onSaved();
      },
    };
    if (forwarder) {
      update.mutate({ id: forwarder.id, ...body }, done);
      return;
    }
    create.mutate({ forwarderType: adapter!.forwarderType, ...body }, done);
  }

  function setValue(key: string, value: string) {
    setValues({ ...values, [key]: value });
  }

  return {
    step,
    setStep,
    adapter,
    name: itemName,
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
  };
}
