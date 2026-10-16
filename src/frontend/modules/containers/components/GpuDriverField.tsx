// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// The GPU driver picker.
//
// A plain text box asked the operator to remember whether this host calls
// its device plugin "nvidia", and told them nothing until the container
// failed to start. This offers the host's registered runtimes as a list
// while still accepting anything typed, because the runtime list is a
// strong hint and not a closed set: `docker info` reports runtimes
// (runc, nvidia, crun…), and a device-plugin driver usually — but not
// always — shares that name.
//
// Hence a combobox rather than a select: the list is the common case,
// free text is the escape hatch.

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { IconAlertTriangle } from '@tabler/icons-react';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { useDockerInfo } from '@resources/hooks/useDockerInfo';

/**
 * Runtimes that are never a GPU device driver. `runc` is the default OCI
 * runtime present on every host, so offering it as a GPU driver would put
 * a guaranteed-wrong answer at the top of the list.
 */
const NON_GPU_RUNTIMES = new Set(['runc', 'crun', 'youki', 'kata-runtime', 'runsc']);

type GpuDriverFieldProps = {
  value: string;
  onChange: (value: string) => void;
  /** Set on the create dialog, where the field sits in a denser form. */
  id?: string;
};

export function GpuDriverField({ value, onChange, id = 'gpu-driver' }: GpuDriverFieldProps) {
  const { t } = useTranslation('containers');
  const { data: info } = useDockerInfo();

  const suggestions = useMemo(() => {
    const runtimes = (info?.runtimes ?? []).filter((r) => !NON_GPU_RUNTIMES.has(r));
    // "nvidia" is offered even when the host does not report it as a
    // runtime: the NVIDIA container toolkit can be installed as a CDI
    // device plugin without registering a runtime of that name, and the
    // driver string is still "nvidia".
    return [...new Set([...runtimes, 'nvidia'])].sort();
  }, [info?.runtimes]);

  // Only warn once the host has actually answered. While `info` is
  // undefined the absence of a GPU runtime is unknown, not false, and a
  // warning that flashes on every open would train the operator to
  // ignore it.
  const hostReported = !!info;
  const hasGpuRuntime = (info?.runtimes ?? []).some((r) => !NON_GPU_RUNTIMES.has(r));

  return (
    <div>
      <Label htmlFor={id} className="mb-1 text-xs text-muted-foreground">
        {t('overview.gpuDriver')}
      </Label>
      <Input
        id={id}
        variant="outline"
        list={`${id}-runtimes`}
        value={value}
        onChange={(evt) => onChange(evt.target.value)}
        placeholder={t('placeholders.gpuDriver')}
      />
      {/*
        A datalist keeps the input free-text while offering the host's own
        runtimes — a Select would have forced a closed set, and this field
        genuinely accepts values the host cannot enumerate.
      */}
      <datalist id={`${id}-runtimes`}>
        {suggestions.map((runtime) => (
          <option key={runtime} value={runtime}>
            {runtime}
          </option>
        ))}
      </datalist>
      {hostReported && !hasGpuRuntime ? (
        <p className="mt-1 flex items-start gap-1.5 text-xs text-warning">
          <IconAlertTriangle className="mt-0.5 size-3.5 shrink-0" aria-hidden="true" />
          {t('resources.gpuNoRuntime')}
        </p>
      ) : (
        <p className="mt-1 text-xs text-muted-foreground">{t('resources.gpuDriverHelp')}</p>
      )}
    </div>
  );
}
