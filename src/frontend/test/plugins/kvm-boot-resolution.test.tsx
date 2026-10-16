// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useState } from 'react';
import { screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterAll, beforeAll, describe, expect, it, vi } from 'vitest';
import { BootResolutionField } from '../../../plugins/Plugins/addon-kvm/frontend/src/components/vm-settings/BootResolutionField';
import type { Draft } from '../../../plugins/Plugins/addon-kvm/frontend/src/components/vm-settings/settingsDraft';
import {
  CUSTOM_CHOICE,
  DEFAULT_CHOICE,
  isValidResolution,
  resolutionChoice,
  resolutionFromChoice,
} from '../../../plugins/Plugins/addon-kvm/frontend/src/bootResolution';
import vmBundle from '../../../plugins/Plugins/addon-kvm/i18n/en/vm.json';
import { i18n } from '../i18n';
import { renderWithProviders } from '../render';

type Size = Pick<Draft, 'displayType' | 'displayWidth' | 'displayHeight'>;

let latest: Size = { displayType: '', displayWidth: 0, displayHeight: 0 };

function Harness({ initial, locked = false }: { initial: Size; locked?: boolean }) {
  const [draft, setDraft] = useState<Draft>({ ...({} as Draft), ...initial });
  useEffect(() => {
    latest = draft;
  }, [draft]);
  return (
    <BootResolutionField
      draft={draft}
      locked={locked}
      set={(field, value) => setDraft((current) => ({ ...current, [field]: value }))}
    />
  );
}

describe('addon-kvm boot resolution', () => {
  beforeAll(() => {
    i18n.addResourceBundle('en', 'vm', vmBundle, true, true);
    // The dropdown's popover measures itself; jsdom has no ResizeObserver.
    vi.stubGlobal(
      'ResizeObserver',
      class {
        observe() {}
        unobserve() {}
        disconnect() {}
      },
    );
  });

  afterAll(() => {
    vi.unstubAllGlobals();
  });

  it('maps stored sizes to dropdown entries and back', () => {
    expect(resolutionChoice(0, 0)).toBe(DEFAULT_CHOICE);
    expect(resolutionChoice(1920, 1080)).toBe('1920x1080');
    expect(resolutionChoice(1921, 1080)).toBe(CUSTOM_CHOICE);
    expect(resolutionFromChoice(DEFAULT_CHOICE)).toEqual({ width: 0, height: 0 });
    expect(resolutionFromChoice('2560x1440')).toEqual({ width: 2560, height: 1440 });
    expect(resolutionFromChoice(CUSTOM_CHOICE)).toBeNull();
    // One side alone is refused by the backend, so it is never "valid" here either.
    expect(isValidResolution(1920, 0)).toBe(false);
    expect(isValidResolution(639, 480)).toBe(false);
  });

  it('picking a preset writes both sides, and the default clears both', async () => {
    const user = userEvent.setup();
    renderWithProviders(<Harness initial={{ displayType: '', displayWidth: 0, displayHeight: 0 }} />);

    await user.click(screen.getByRole('button', { name: 'Boot resolution' }));
    await user.click(await screen.findByText('1920 × 1080'));
    expect(latest).toMatchObject({ displayWidth: 1920, displayHeight: 1080 });

    await user.click(screen.getByRole('button', { name: 'Boot resolution' }));
    await user.click(await screen.findByText('QEMU default'));
    expect(latest).toMatchObject({ displayWidth: 0, displayHeight: 0 });
  });

  it('a custom size outside the limits names the range', async () => {
    renderWithProviders(<Harness initial={{ displayType: 'virtio-gpu', displayWidth: 500, displayHeight: 400 }} />);

    expect(screen.getByRole('spinbutton', { name: 'Width' })).toBeInTheDocument();
    expect(screen.getByText(/Use a width of 640 to 7680 and a height of 480 to 4320 pixels/)).toBeInTheDocument();
  });

  it('an adapter that takes no resolution disables the field and says why', () => {
    renderWithProviders(<Harness initial={{ displayType: 'std', displayWidth: 0, displayHeight: 0 }} />);

    expect(screen.getByRole('button', { name: 'Boot resolution' })).toBeDisabled();
    expect(screen.getByText('Only virtio-gpu and QXL take a boot resolution.')).toBeInTheDocument();
  });
});
