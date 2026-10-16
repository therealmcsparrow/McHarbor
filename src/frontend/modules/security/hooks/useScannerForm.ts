// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import {
  useCreateScanner,
  useUpdateScanner,
  type ScannerAdapter,
  type ScannerConfig,
  type ScannerField,
} from './useScanners';

type ScannerFormOptions = {
  open: boolean;
  adapters: ScannerAdapter[];
  scanner?: ScannerConfig | null;
  onSaved: () => void;
  onSuccessToast: (edited: boolean) => void;
};

/**
 * Draft state for the add/edit scanner dialog: which engine, the field
 * values, and the two switches — plus the save call.
 */
export function useScannerForm({ open: isOpen, adapters, scanner, onSaved, onSuccessToast }: ScannerFormOptions) {
  const [step, setStep] = useState<'type' | 'config'>('type');
  const [adapter, setAdapter] = useState<ScannerAdapter | null>(null);
  const [itemName, setName] = useState('');
  const [values, setValues] = useState<Record<string, string>>({});
  const [enabled, setEnabled] = useState(true);
  const [makeDefault, setMakeDefault] = useState(false);
  // Required-field errors stay hidden until the first save attempt, so
  // a freshly opened form is not a wall of red before anything has been
  // typed.
  const [showErrors, setShowErrors] = useState(false);

  const create = useCreateScanner();
  const update = useUpdateScanner();
  const saving = create.isPending || update.isPending;

  const [seeded, setSeeded] = useState<{ isOpen: boolean; scanner: unknown; adapters: unknown }>({
    isOpen: false,
    scanner: undefined,
    adapters: undefined,
  });
  if (seeded.isOpen !== isOpen || seeded.scanner !== scanner || seeded.adapters !== adapters) {
    setSeeded({ isOpen, scanner, adapters });
    if (isOpen) seedFromScanner();
  }

  function seedFromScanner() {
    setShowErrors(false);
    setName(scanner?.name ?? '');
    // A scanner nobody enabled is a scanner that never runs, and the
    // reason to add one is to use it. Default the new-scanner form to
    // on; editing keeps whatever the row already says.
    setEnabled(scanner?.enabled ?? true);
    setMakeDefault(scanner?.isDefault ?? false);
    if (scanner) {
      setAdapter(adapters.find((a) => a.scannerType === scanner.scannerType) ?? null);
      setValues(Object.fromEntries(scanner.fields.map((field) => [field.key, field.value])));
      setStep('config');
      return;
    }
    setAdapter(null);
    setValues({});
    setStep('type');
  }

  // The stored row's fields carry `hasValue` for secrets; the adapter's
  // schema does not. Editing must use the row's copy or a stored secret
  // would read as an empty required field.
  const fields: ScannerField[] = scanner?.fields ?? adapter?.fields ?? [];

  /**
   * A required field is missing when it is empty — except a secret that
   * already has a stored value, where blank means "keep it" rather than
   * "clear it", the same rule the backend applies on save.
   */
  function isMissing(field: ScannerField): boolean {
    if (!field.required) return false;
    if (field.type === 'secret' && field.hasValue) return false;
    return (values[field.key] ?? '').trim() === '';
  }

  const incomplete = itemName.trim() === '' || fields.some(isMissing);

  function selectAdapter(item: ScannerAdapter) {
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
    // Pre-fill the name with the engine's own label. Most estates run
    // one of each, and the engine name is the right name for that
    // scanner — an operator who wants a "(strict)" suffix can still type it.
    setName(item.displayName);
    setEnabled(true);
    setMakeDefault(false);
    setShowErrors(false);
    setStep('config');
  }

  function submit() {
    if (!adapter && !scanner) return;
    // Saving with an empty server URL used to create a row that listed
    // as a configured engine and scanned nothing. Stop it here as well
    // as on the backend.
    if (incomplete) {
      setShowErrors(true);
      return;
    }
    const body = { name: itemName.trim(), enabled, makeDefault, config: values };
    const done = {
      onSuccess: () => {
        onSuccessToast(!!scanner);
        onSaved();
      },
    };
    if (scanner) {
      update.mutate({ id: scanner.id, ...body }, done);
      return;
    }
    create.mutate({ scannerType: adapter!.scannerType, ...body }, done);
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
    makeDefault,
    setMakeDefault,
    showErrors,
    fields,
    isMissing,
    saving,
    selectAdapter,
    submit,
  };
}
