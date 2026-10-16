// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { beforeEach, describe, expect, it } from 'vitest';
import { useContainersViewStore } from './containers/stores/containers-view';
import { useEnvironmentsViewStore } from './environments/stores/environments-view';
import { useImagesViewStore } from './images/stores/images-view';
import { useNetworksViewStore } from './networks/stores/networks-view';
import { useStacksViewStore } from './stacks/stores/stacks-view';
import { useVolumesViewStore } from './volumes/stores/volumes-view';

describe('module view stores', () => {
  beforeEach(() => {
    useContainersViewStore.setState({ viewMode: 'table' });
    useEnvironmentsViewStore.setState({ viewMode: 'table' });
    useImagesViewStore.setState({ viewMode: 'table' });
    useNetworksViewStore.setState({ viewMode: 'table' });
    useStacksViewStore.setState({ viewMode: 'table' });
    useVolumesViewStore.setState({ viewMode: 'table' });
  });

  it('updates table/card view preferences for all list modules', () => {
    useContainersViewStore.getState().setViewMode('card');
    useEnvironmentsViewStore.getState().setViewMode('card');
    useImagesViewStore.getState().setViewMode('card');
    useNetworksViewStore.getState().setViewMode('card');
    useStacksViewStore.getState().setViewMode('card');
    useVolumesViewStore.getState().setViewMode('card');

    expect(useContainersViewStore.getState().viewMode).toBe('card');
    expect(useEnvironmentsViewStore.getState().viewMode).toBe('card');
    expect(useImagesViewStore.getState().viewMode).toBe('card');
    expect(useNetworksViewStore.getState().viewMode).toBe('card');
    expect(useStacksViewStore.getState().viewMode).toBe('card');
    expect(useVolumesViewStore.getState().viewMode).toBe('card');
  });
});
