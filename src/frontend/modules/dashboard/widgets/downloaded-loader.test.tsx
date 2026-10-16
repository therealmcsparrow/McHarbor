// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { Suspense } from 'react';
import { MemoryRouter } from 'react-router';
import { screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import type { WidgetCatalogDefinition } from '@resources/hooks/useWidgetCatalog';
import { renderWithProviders } from '../../../test/render';
import { downloadedWidgetGlobal, isLoadableDownloaded, toDownloadedDefinition } from './downloaded-loader';

const base: WidgetCatalogDefinition = {
  key: 'clock',
  label: 'Clock',
  category: 'plugin',
  description: 'Shows the time',
  icon: 'IconClock',
  source: 'downloaded',
  component: 'bundle.js',
  enabled: true,
  defaultSize: { w: 3, h: 2 },
  minSize: { w: 2, h: 1 },
  version: '2026.09.01',
  verified: true,
};

function bundleScripts(): HTMLScriptElement[] {
  return Array.from(document.head.querySelectorAll('script')).filter((s) => s.src.includes('/api/widgets/'));
}

describe('downloaded widget loader', () => {
  afterEach(() => {
    for (const s of bundleScripts()) s.remove();
  });

  it.each([
    ['unverified', { verified: false }],
    ['missing the verified flag', { verified: undefined }],
    ['a built-in', { source: 'builtin' }],
    ['a key that could leave the route', { key: '../plugins/loader' }],
  ])('refuses a definition that is %s and never adds a script tag', (_label, patch) => {
    const def = { ...base, ...patch };
    expect(isLoadableDownloaded(def)).toBe(false);
    expect(toDownloadedDefinition(def)).toBeNull();
    expect(bundleScripts()).toHaveLength(0);
  });

  it('registers a verified widget under its own id and mounts it from the same-origin bundle route', async () => {
    const def = toDownloadedDefinition(base);
    expect(def?.id).toBe('downloaded:clock');
    expect(def?.labelKey).toBe('widgets.clock.label');
    const Widget = def!.component;
    renderWithProviders(
      <MemoryRouter>
        <Suspense fallback={null}>
          <Widget colSpan={3} typeId="downloaded:clock" />
        </Suspense>
      </MemoryRouter>,
    );
    await waitFor(() => expect(bundleScripts()).toHaveLength(1));
    const [script] = bundleScripts();
    if (!script) throw new Error('no bundle script was added');
    expect(new URL(script.src).pathname).toBe('/api/widgets/clock/bundle');
    window[downloadedWidgetGlobal('clock')] = {
      mount: (el: HTMLElement) => {
        el.textContent = 'tick';
      },
    };
    script.dispatchEvent(new Event('load'));
    expect(await screen.findByText('tick')).toBeInTheDocument();
  });
});
