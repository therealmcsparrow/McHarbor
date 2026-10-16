// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { useIsNative, useIsSandboxed } from '@resources/hooks/useInstallKind';
import type { ReactNode } from 'react';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import type { StorageLocationInput } from '@resources/hooks/useStorageLocations';

type StorageLocationFormFieldsProps = {
  data: StorageLocationInput;
  isEdit?: boolean;
  onChange: (patch: Partial<StorageLocationInput>) => void;
  // isEdit is reserved for future per-type gating (e.g. locking
  // the location type after first save). It's accepted but unused
  // in the host's local-only fallback.
};

type FieldProps = {
  id: string;
  label: string;
  value: string | number | undefined;
  onChange: (value: string) => void;
  type?: string;
  placeholder?: string;
  helper?: string;
};

function Field({ id, label, value, onChange, type = 'text', placeholder, helper }: FieldProps) {
  return (
    <div>
      <Label htmlFor={id} className="mb-1 text-xs text-muted-foreground">
        {label}
      </Label>
      <Input
        id={id}
        type={type}
        value={value ?? ''}
        onChange={(changeEvent) => onChange(changeEvent.target.value)}
        placeholder={placeholder}
        variant="outline"
      />
      {helper && <p className="mt-1 text-xs text-muted-foreground">{helper}</p>}
    </div>
  );
}

function InfoPanel({ children }: { children: ReactNode }) {
  return (
    <div className="rounded-lg border border-border bg-muted/30 p-3 text-xs leading-5 text-muted-foreground">
      {children}
    </div>
  );
}

export function StorageLocationFormFields({ data, onChange }: StorageLocationFormFieldsProps) {
  const { t } = useTranslation('settings');
  const native = useIsNative();
  const sandboxed = useIsSandboxed();

  // The host only renders the `local` form. Every other LocationType
  // is plugin-owned and surfaces via ProviderFormOutlet in
  // StorageLocationDialog.tsx (which wraps THIS component as a
  // fallback). The if/else chain that used to handle every
  // built-in provider is gone — keep the local branch and the
  // helper pieces only.

  if (data.locationType === 'local') {
    return (
      <div className="space-y-4">
        <InfoPanel>{t('storage.localPathHint')}</InfoPanel>
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <Field
            id="location-local-path"
            label={t('storage.localPathLabel')}
            value={data.basePath}
            onChange={(basePath) => onChange({ basePath })}
            placeholder={t('storage.examples.localPath')}
            helper={t(
              sandboxed
                ? 'storage.localPathHelperNative'
                : native
                  ? 'storage.localPathHelperNativeRoot'
                  : 'storage.localPathHelper',
            )}
          />
        </div>
      </div>
    );
  }

  // Plugin-owned destination type. The provider-aware form is
  // rendered by the plugin's ConfigForm.tsx via ProviderFormOutlet
  // in StorageLocationDialog.tsx. The host's fallback is a neutral
  // placeholder so the dialog never crashes when a plugin's bundle
  // fails to load.
  return (
    <div className="rounded-lg border border-border bg-muted/30 p-3 text-xs text-muted-foreground">
      {t('storage.pluginOwnedFormHint')}
    </div>
  );
}
