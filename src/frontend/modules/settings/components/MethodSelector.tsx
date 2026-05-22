// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Button } from '@resources/components/ui/Button';

type MethodOption<T extends string> = {
  key: T;
  labelKey: string;
  descriptionKey: string;
};

type MethodSelectorProps<T extends string> = {
  methods: MethodOption<T>[];
  selected: T;
  onChange: (method: T) => void;
};

export function MethodSelector<T extends string>({ methods, selected, onChange }: MethodSelectorProps<T>) {
  const { t } = useTranslation('settings');

  return (
    <div>
      <p className="mb-2 text-xs font-medium text-muted-foreground">{t('communications.selectMethod')}</p>
      <div className="grid grid-cols-2 gap-2">
        {methods.map((m) => (
          <Button
            key={m.key}
            variant={selected === m.key ? 'default' : 'outline'}
            onClick={() => onChange(m.key)}
            className="flex h-auto flex-col items-start gap-0.5 p-3 text-left"
          >
            <span className="text-xs font-medium">{t(m.labelKey)}</span>
            <span className={`text-[10px] leading-tight ${selected === m.key ? 'text-primary-foreground/70' : 'text-muted-foreground'}`}>
              {t(m.descriptionKey)}
            </span>
          </Button>
        ))}
      </div>
    </div>
  );
}
