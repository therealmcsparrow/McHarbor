// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { act, render } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

// The pages are what the selected cluster feeds; this test is about how a link sets it.
vi.mock('../../../plugins/Plugins/addon-kubernetes/frontend/src/pages/KubernetesPage', () => ({
  KubernetesPage: () => <div>browser</div>,
}));
vi.mock('../../../plugins/Plugins/addon-kubernetes/frontend/src/pages/ClustersPage', () => ({
  ClustersPage: () => <div>clusters</div>,
}));

import { App } from '../../../plugins/Plugins/addon-kubernetes/frontend/src/App';
import type { PluginNavContext } from '../../../plugins/Plugins/addon-kubernetes/frontend/src/index';

const STORAGE_KEY = 'mcharbor-plugin-kubernetes:cluster';

function ctx(search: string): PluginNavContext {
  return {
    slug: 'addon-kubernetes',
    path: '/plugins/addon-kubernetes/pods/default/web',
    splat: 'pods/default/web',
    search,
    navigate: () => {},
  };
}

describe('addon-kubernetes cluster links', () => {
  beforeEach(() => {
    window.localStorage.setItem(STORAGE_KEY, 'c1');
  });

  // The palette searches every cluster; a hit on another one arrives as ?cluster=<id>,
  // and without selecting it the page would open the item on the cluster last chosen.
  it('selects the cluster a link names, on mount and on a later navigation', () => {
    let push: (next: PluginNavContext) => void = () => {};
    render(
      <App
        initial={ctx('?cluster=c2')}
        subscribe={(listener) => {
          push = listener;
          return () => {};
        }}
      />,
    );
    expect(window.localStorage.getItem(STORAGE_KEY)).toBe('c2');

    act(() => push(ctx('?tab=logs&cluster=c3')));
    expect(window.localStorage.getItem(STORAGE_KEY)).toBe('c3');
  });

  it('leaves the selection alone when a link names no cluster', () => {
    render(<App initial={ctx('')} subscribe={() => () => {}} />);
    expect(window.localStorage.getItem(STORAGE_KEY)).toBe('c1');
  });
});
