// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { forwardRef, type InputHTMLAttributes, type ReactNode } from 'react';
import { cn } from '@resources/utils/cn';
import { cssVars } from '@resources/utils/cssVars';

type SliderTone = 'safe' | 'caution' | 'unsafe';

type SliderZone = {
  upTo: number;
  tone: SliderTone;
};

type SliderProps = Omit<InputHTMLAttributes<HTMLInputElement>, 'type' | 'onChange' | 'value' | 'size'> & {
  value: number;
  onChange: (value: number) => void;
  min: number;
  max: number;
  step?: number;
  /** Rendered above the track, right-aligned — usually the formatted value. */
  valueLabel?: ReactNode;
  /** Rendered under the ends of the track, for the bounds. */
  minLabel?: ReactNode;
  maxLabel?: ReactNode;
  zones?: readonly SliderZone[];
};

const TONE_COLOR: Record<SliderTone, string> = {
  safe: 'var(--status-success)',
  caution: 'var(--status-warning)',
  unsafe: 'var(--status-danger)',
};

const Slider = forwardRef<HTMLInputElement, SliderProps>(
  (
    { className, value, onChange, min, max, step = 1, disabled, valueLabel, minLabel, maxLabel, zones, ...props },
    ref,
  ) => {
    const span = max - min;
    const filled = span > 0 ? Math.min(100, Math.max(0, ((value - min) / span) * 100)) : 0;

    const pct = (at: number) => (span > 0 ? Math.min(100, Math.max(0, ((at - min) / span) * 100)) : 0);

    const track =
      zones && zones.length > 0 && span > 0
        ? `linear-gradient(to right, ${zones
            .map((zone, index, all) => {
              const previous = all[index - 1];
              const start = previous ? pct(previous.upTo) : 0;
              const end = index === all.length - 1 ? 100 : pct(zone.upTo);
              return `${TONE_COLOR[zone.tone]} ${start}% ${end}%`;
            })
            .join(', ')})`
        : undefined;

    return (
      <div className={cn('space-y-1.5', className)}>
        {valueLabel !== undefined && (
          <div className="flex justify-end text-sm font-medium text-foreground">{valueLabel}</div>
        )}
        <input
          ref={ref}
          type="range"
          value={value}
          min={min}
          max={max}
          step={step}
          disabled={disabled}
          onChange={(evt) => onChange(Number(evt.target.value))}
          className={cn(
            'mc-slider h-1.5 w-full cursor-pointer appearance-none rounded-full bg-muted',
            track && 'mc-slider-zoned',
            'focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none',
            'disabled:cursor-not-allowed disabled:opacity-50',
          )}
          style={cssVars({
            'mc-slider-filled': `${filled}%`,
            ...(track ? { 'mc-slider-track': track } : {}),
          })}
          {...props}
        />
        {(minLabel !== undefined || maxLabel !== undefined) && (
          <div className="flex justify-between text-[11px] text-muted-foreground">
            <span>{minLabel}</span>
            <span>{maxLabel}</span>
          </div>
        )}
      </div>
    );
  },
);

Slider.displayName = 'Slider';

export { Slider };
export type { SliderProps, SliderTone, SliderZone };
