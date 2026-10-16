// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useId } from 'react';
import { useTranslation } from 'react-i18next';
import { IconRestore } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Field } from '@resources/components/ui/Form';
import { Flex, Stack } from '@resources/components/ui/Layout';
import { NumberInput } from '@resources/components/ui/NumberInput';
import { Text as TypographyText } from '@resources/components/ui/Typography';
import { toNumberOrNull, toNumberText } from '@resources/utils/numberText';
import { DEMO_CONFIG_GROUPS, type DemoConfigProblems } from '@resources/constants/demoConfig';
import type { DemoConfigForm } from '../hooks/useDemoConfigForm';
import { useDemoConfigLabels } from '../hooks/useDemoConfigLabels';
import type { DemoConfigField, DemoSizes } from '@resources/types/demo';

type DemoConfigFieldsProps = {
  form: DemoConfigForm;
  sizes: DemoSizes;
  problems: DemoConfigProblems;
};

export function DemoConfigFields({ form, sizes, problems }: DemoConfigFieldsProps) {
  const { t, i18n } = useTranslation('demo');
  const labels = useDemoConfigLabels();
  const prefix = useId();
  const format = (value: number) => new Intl.NumberFormat(i18n.language).format(value);

  function hint(field: DemoConfigField): string {
    const fallback = t('config.defaultHint', { value: format(sizes.defaults[field]) });
    return field === 'historyDays' ? `${t('config.historyHelp')} ${fallback}` : fallback;
  }

  return (
    <Stack gap={4}>
      <Flex align="center" justify="between" gap={3} wrap>
        <TypographyText size="sm" tone="muted">
          {t('config.intro')}
        </TypographyText>
        <Button variant="ghost" size="sm" onClick={form.restoreDefaults} disabled={form.isDefault}>
          <IconRestore className="size-4" aria-hidden />
          {t('config.restoreDefaults')}
        </Button>
      </Flex>
      {DEMO_CONFIG_GROUPS.map((group) => (
        <fieldset key={group.key} className="min-w-0 space-y-3">
          <legend className="mb-2 text-sm font-medium text-foreground">{labels.group[group.key]}</legend>
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            {group.fields.map((field) => {
              const id = `${prefix}-${field}`;
              const problem = problems[field];
              const message = problem ? labels.problem(problem) : undefined;
              return (
                <Field key={field} id={id} label={labels.field[field]} description={hint(field)} error={message}>
                  <NumberInput
                    id={id}
                    nullable
                    allowOutOfRange
                    inputMode="numeric"
                    min={sizes.limits[field].min}
                    max={sizes.limits[field].max}
                    step={1}
                    value={toNumberOrNull(form.draft[field])}
                    aria-invalid={message ? true : undefined}
                    aria-describedby={message ? `${id}-error` : `${id}-description`}
                    onChange={(next) => form.setField(field, toNumberText(next))}
                  />
                </Field>
              );
            })}
          </div>
        </fieldset>
      ))}
    </Stack>
  );
}
