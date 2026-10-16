// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { render, screen } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { ReadableScheduleField } from '@resources/components/ReadableScheduleField';

vi.mock('@resources/components/CronSchedulePreview', () => ({ CronSchedulePreview: () => null }));

function openWith(value: string) {
  const onChange = vi.fn();
  const view = render(<ReadableScheduleField value={value} onChange={onChange} />);
  const mode = screen.queryByPlaceholderText('0 3 * * *') ? 'advanced' : 'readable';
  view.unmount();
  return { onChange, mode };
}

function seeded(seed: number) {
  let state = seed;
  return (max: number) => {
    state = (state * 1103515245 + 12345) % 2147483648;
    return Math.floor((state / 2147483648) * max);
  };
}

function builderExpressions(count: number): string[] {
  const next = seeded(20260925);
  const out: string[] = [];
  for (let i = 0; i < count; i += 1) {
    const minute = next(60);
    const hour = next(24);
    const every = 2 + next(22);
    switch (next(8)) {
      case 0:
        out.push(`${minute} * * * *`);
        break;
      case 1:
        out.push(`${minute} */${every} * * *`);
        break;
      case 2:
        out.push(`${minute} ${hour} * * *`);
        break;
      case 3:
        out.push(`${minute} ${hour} */${every} * *`);
        break;
      case 4:
        out.push(`${minute} ${hour} * * ${next(7)}`);
        break;
      case 5:
        out.push(`${minute} ${hour} ${1 + next(31)} * *`);
        break;
      case 6:
        out.push(`${minute} ${hour} ${1 + next(28)} ${1 + next(12)} *`);
        break;
      default:
        out.push(`${minute} ${hour} ${1 + next(28)} */${2 + next(11)} *`);
    }
  }
  return out;
}

describe('ReadableScheduleField round-trip', () => {
  beforeEach(() => {
    vi.useFakeTimers({ toFake: ['Date'] });
    vi.setSystemTime(new Date(2026, 8, 25, 11, 0, 0));
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it.each(builderExpressions(200))('opening %s never rewrites it', (expression) => {
    const { onChange, mode } = openWith(expression);
    if (mode === 'readable') {
      for (const [emitted] of onChange.mock.calls) expect(emitted).toBe(expression);
    } else {
      expect(onChange).not.toHaveBeenCalled();
    }
  });

  it.each([
    ['a weekly schedule on a day other than today', '0 3 * * 1'],
    ['a weekly schedule on Sunday', '30 22 * * 0'],
    ['the 31st in a 30-day month', '0 3 31 * *'],
    ['a yearly schedule on 29 February', '0 3 29 2 *'],
    ['a yearly schedule on 31 December', '15 4 31 12 *'],
  ])('keeps %s intact', (_label, expression) => {
    const { onChange } = openWith(expression);
    for (const [emitted] of onChange.mock.calls) expect(emitted).toBe(expression);
  });

  it.each([
    '15 2,14 * * 1-5',
    '0 0 1 1 1',
    '*/5 * * * *',
    '0 */1 * * *',
    '0 3 1 */1 *',
    '0 */0 * * *',
    '0 3 * * 7',
    '0 24 * * *',
    'not a cron',
  ])('opens %s in Advanced mode and emits nothing', (expression) => {
    const { onChange, mode } = openWith(expression);
    expect(mode).toBe('advanced');
    expect(onChange).not.toHaveBeenCalled();
  });
});
