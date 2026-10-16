// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import {
  forwardRef,
  useState,
  type ChangeEvent,
  type FocusEvent as ReactFocusEvent,
  type InputHTMLAttributes,
  type ReactNode,
} from 'react';
import { useTranslation } from 'react-i18next';
import { IconMinus, IconPlus } from '@tabler/icons-react';
import { cn } from '@resources/utils/cn';
import { PlainButton } from './PlainButton';

type BaseProps = Omit<
  InputHTMLAttributes<HTMLInputElement>,
  'type' | 'onChange' | 'size' | 'value' | 'defaultValue' | 'min' | 'max' | 'step'
> & {
  min?: number;
  max?: number;
  step?: number;
  size?: 'default' | 'sm';
  suffix?: ReactNode;
  allowOutOfRange?: boolean;
};

type RequiredValueProps = BaseProps & {
  nullable?: false;
  value: number;
  onChange: (value: number) => void;
};

type NullableValueProps = BaseProps & {
  nullable: true;
  value: number | null;
  onChange: (value: number | null) => void;
};

export type NumberInputProps = RequiredValueProps | NullableValueProps;

const sizeConfig = {
  default: {
    input: 'mc-control px-4 text-sm',
    suffix: 'mc-control px-3 text-xs',
    button: 'mc-control w-9',
    icon: 'size-3.5',
  },
  sm: {
    input: 'mc-control px-3 text-xs',
    suffix: 'mc-control px-2 text-xs',
    button: 'mc-control w-7',
    icon: 'size-3',
  },
};

function decimalsOf(n: number | undefined): number {
  if (n === undefined || !Number.isFinite(n)) return 0;
  const text = String(n);
  const dot = text.indexOf('.');
  return dot < 0 ? 0 : text.length - dot - 1;
}

function normalize(value: number | null | undefined): number | null {
  return value === null || value === undefined || Number.isNaN(value) ? null : value;
}

function format(value: number | null): string {
  return value === null ? '' : String(value);
}

function parseDraft(draft: string): number | null {
  if (draft.trim() === '') return null;
  const num = Number(draft);
  return Number.isFinite(num) ? num : null;
}

const NumberInput = forwardRef<HTMLInputElement, NumberInputProps>(
  (
    {
      className,
      value,
      onChange,
      min,
      max,
      step = 1,
      disabled,
      readOnly,
      size = 'default',
      suffix,
      nullable,
      allowOutOfRange = false,
      onBlur,
      ...props
    },
    ref,
  ) => {
    const { t } = useTranslation('common');
    const s = sizeConfig[size];
    const emit = onChange as (next: number | null) => void;
    const current = normalize(value);
    const [draft, setDraft] = useState(() => format(current));
    const [seen, setSeen] = useState(current);
    if (!Object.is(seen, current)) {
      setSeen(current);
      if (parseDraft(draft) !== current) setDraft(format(current));
    }

    const precision = Math.max(decimalsOf(step), decimalsOf(min), decimalsOf(max));
    const clamp = (n: number) => {
      let next = Number(n.toFixed(precision));
      if (min !== undefined && next < min) next = min;
      if (max !== undefined && next > max) next = max;
      return next;
    };
    const locked = disabled || readOnly;

    const stepBy = (direction: 1 | -1) => {
      const next = current === null ? clamp(min ?? 0) : clamp(current + direction * step);
      if (next === current) return;
      setDraft(format(next));
      emit(next);
    };

    const handleChange = (e: ChangeEvent<HTMLInputElement>) => {
      const raw = e.target.value;
      if (raw === '' && e.target.validity?.badInput) return;
      setDraft(raw);
      if (raw.trim() === '') {
        if (nullable) emit(null);
        return;
      }
      const num = Number(raw);
      if (!Number.isFinite(num)) return;
      if (!allowOutOfRange && min !== undefined && num < min) return;
      if (!allowOutOfRange && max !== undefined && num > max) return;
      emit(num);
    };

    const handleBlur = (e: ReactFocusEvent<HTMLInputElement>) => {
      const typed = parseDraft(draft);
      let settled = current;
      if (typed !== null && typed !== current && !allowOutOfRange) {
        const bounded = Math.min(max ?? Infinity, Math.max(min ?? -Infinity, typed));
        if (bounded !== current) {
          settled = bounded;
          emit(bounded);
        }
      }
      setDraft(format(settled));
      onBlur?.(e);
    };

    return (
      <div className={cn('flex items-center', className)}>
        <input
          ref={ref}
          type="number"
          value={draft}
          onChange={handleChange}
          onBlur={handleBlur}
          min={min}
          max={max}
          step={step}
          disabled={disabled}
          readOnly={readOnly}
          className={cn(
            'mc-field mc-field-seg w-full min-w-0 [appearance:textfield] rounded-l-lg border text-foreground disabled:opacity-50 [&::-webkit-inner-spin-button]:appearance-none [&::-webkit-outer-spin-button]:appearance-none',
            s.input,
          )}
          {...props}
        />
        {suffix !== undefined && suffix !== null && suffix !== '' && (
          <span
            className={cn(
              'mc-field mc-field-seg flex shrink-0 items-center border border-l-0 whitespace-nowrap text-muted-foreground',
              s.suffix,
            )}
          >
            {suffix}
          </span>
        )}
        <PlainButton
          type="button"
          onClick={() => stepBy(-1)}
          disabled={locked || (current !== null && min !== undefined && current <= min)}
          aria-label={t('numberInput.decrease')}
          className={cn(
            'mc-field mc-field-seg mc-field-btn flex shrink-0 items-center justify-center border border-l-0 text-muted-foreground transition-colors disabled:pointer-events-none disabled:opacity-50',
            s.button,
          )}
        >
          <IconMinus className={s.icon} />
        </PlainButton>
        <PlainButton
          type="button"
          onClick={() => stepBy(1)}
          disabled={locked || (current !== null && max !== undefined && current >= max)}
          aria-label={t('numberInput.increase')}
          className={cn(
            'mc-field mc-field-seg mc-field-btn flex shrink-0 items-center justify-center rounded-r-lg border border-l-0 text-muted-foreground transition-colors disabled:pointer-events-none disabled:opacity-50',
            s.button,
          )}
        >
          <IconPlus className={s.icon} />
        </PlainButton>
      </div>
    );
  },
);
NumberInput.displayName = 'NumberInput';

export { NumberInput };
