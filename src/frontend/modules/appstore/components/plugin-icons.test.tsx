// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { render } from '@testing-library/react';
import type { ReactElement } from 'react';
import { describe, expect, it } from 'vitest';
import { PluginIcon } from './plugin-icons';

function src(ui: ReactElement): string | null {
  return render(ui).container.querySelector('img')?.getAttribute('src') ?? null;
}

describe('plugin store icons', () => {
  it('loads a remote catalog entry through the host, never from the catalog host directly', () => {
    expect(src(<PluginIcon icon="plugin.svg" slug="acme" fromCatalog />)).toMatch(
      /^\/api\/appstore\/plugins\/acme\/icon(\?variant=dark)?$/,
    );
  });

  it('keeps a local or installed plugin on the loader route', () => {
    expect(src(<PluginIcon icon="plugin.svg" slug="acme" />)).toMatch(
      /^\/api\/plugins\/loader\/icon\?slug=acme&name=plugin\.svg(&variant=dark)?$/,
    );
  });
});
