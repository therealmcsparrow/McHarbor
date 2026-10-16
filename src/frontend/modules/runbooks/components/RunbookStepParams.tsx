// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Input } from '@resources/components/ui/Input';
import { NumberInput } from '@resources/components/ui/NumberInput';
import { Select } from '@resources/components/ui/Select';
import { Switch } from '@resources/components/ui/Switch';
import { RunbookTargetField } from './RunbookTargetField';
import { toNumberOrNull, toNumberText } from '@resources/utils/numberText';
import { useParamLabel } from '../hooks/useParamLabel';
import type { RunbookParamSpec, RunbookTargetKind } from '@resources/types/runbooks';

type RunbookStepParamsProps = {
  specs: RunbookParamSpec[];
  targets: RunbookTargetKind[] | undefined;
  values: Record<string, string>;
  envId: string;
  onChange: (values: Record<string, string>) => void;
};

type ParamFieldProps = {
  spec: RunbookParamSpec;
  targets: RunbookTargetKind[] | undefined;
  label: string;
  value: string;
  values: Record<string, string>;
  envId: string;
  onValue: (value: string) => void;
};

function ParamField({ spec, targets, label, value, values, envId, onValue }: ParamFieldProps) {
  const { t } = useTranslation('runbooks');

  if (spec.type === 'target' && spec.target) {
    const declared = targets?.find((target) => target.kind === spec.target);
    return (
      <RunbookTargetField
        kind={spec.target}
        value={value}
        onChange={onValue}
        envId={envId}
        cluster={values.cluster ?? ''}
        ariaLabel={label}
        emptyLabel={
          declared?.emptyLabelKey ? t(declared.emptyLabelKey, { ns: declared.labelNs }) : t('editor.fromRunSubject')
        }
      />
    );
  }
  if (spec.type === 'enum') {
    return (
      <Select
        ariaLabel={label}
        value={value}
        onChange={onValue}
        options={[
          { value: '', label: t('editor.paramDefault') },
          ...(spec.options ?? []).map((option) => ({ value: option, label: t(`paramOption.${option}`) })),
        ]}
      />
    );
  }
  if (spec.type === 'bool') {
    return (
      <Switch
        aria-label={label}
        checked={value === 'true'}
        onCheckedChange={(checked) => onValue(checked ? 'true' : 'false')}
      />
    );
  }
  if (spec.type === 'int') {
    return (
      <NumberInput
        aria-label={label}
        nullable
        inputMode="numeric"
        min={spec.min}
        max={spec.max}
        required={spec.required}
        value={toNumberOrNull(value)}
        onChange={(next) => onValue(toNumberText(next))}
      />
    );
  }
  return (
    <Input
      aria-label={label}
      type="text"
      required={spec.required}
      value={value}
      onChange={(evt) => onValue(evt.target.value)}
    />
  );
}

export function RunbookStepParams({ specs, targets, values, envId, onChange }: RunbookStepParamsProps) {
  const { t } = useTranslation('runbooks');
  const paramLabel = useParamLabel(targets);
  if (specs.length === 0) return null;

  return (
    <fieldset className="grid gap-3 sm:grid-cols-2">
      <legend className="mb-1 text-xs font-medium text-muted-foreground">{t('editor.params')}</legend>
      {specs.map((spec) => (
        <div key={spec.name} className="flex min-w-0 flex-col gap-1 text-sm text-foreground">
          <span className="text-xs text-muted-foreground">
            {paramLabel(spec)}
            {spec.required && spec.type !== 'target' ? ' *' : ''}
          </span>
          <ParamField
            spec={spec}
            targets={targets}
            label={paramLabel(spec)}
            value={values[spec.name] ?? ''}
            values={values}
            envId={envId}
            onValue={(next) => {
              const updated = { ...values, [spec.name]: next };
              if (next === '') delete updated[spec.name];
              onChange(updated);
            }}
          />
        </div>
      ))}
    </fieldset>
  );
}
