// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { describe, expect, it } from 'vitest';
import type { ContainerInfo, ImageInfo, VolumeInfo } from '@core/types/docker';
import {
  canRunContainerUpdateOperation,
  isProtectedContainer,
  isProtectedImage,
  isProtectedStack,
  isProtectedVolume,
} from './protection';

function container(overrides: Partial<ContainerInfo>): ContainerInfo {
  return {
    Id: 'container-id',
    Names: ['/app'],
    Image: 'nginx:latest',
    ImageID: 'image-id',
    Command: '',
    Created: 0,
    Ports: [],
    Labels: {},
    State: 'running',
    Status: 'running',
    NetworkSettings: { Networks: {} },
    Mounts: [],
    ...overrides,
  };
}

function image(overrides: Partial<ImageInfo>): ImageInfo {
  return {
    Id: 'image-id',
    ParentId: '',
    RepoTags: ['nginx:latest'],
    RepoDigests: null,
    Created: 0,
    Size: 0,
    SharedSize: 0,
    VirtualSize: 0,
    Labels: null,
    Containers: 0,
    ...overrides,
  };
}

function volume(overrides: Partial<VolumeInfo>): VolumeInfo {
  return {
    Name: 'app-data',
    Driver: 'local',
    Mountpoint: '/var/lib/docker/volumes/app-data/_data',
    CreatedAt: '',
    Labels: null,
    Scope: 'local',
    Options: null,
    RefCount: 0,
    ...overrides,
  };
}

describe('isProtectedContainer', () => {
  it('protects containers by explicit flag, protected labels, compose labels, image, and name', () => {
    expect(isProtectedContainer(container({ Protected: true }))).toBe(true);
    expect(isProtectedContainer(container({ Labels: { 'com.mcharbor.protected': 'yes' } }))).toBe(true);
    expect(
      isProtectedContainer(
        container({
          Labels: {
            'com.docker.compose.project': 'mcharbor',
            'com.docker.compose.service': 'mcharbor',
          },
        }),
      ),
    ).toBe(true);
    expect(isProtectedContainer(container({ Image: 'ghcr.io/therealmcsparrow/mcharbor:latest' }))).toBe(true);
    expect(isProtectedContainer(container({ Names: ['/mcharbor-mcharbor-1'] }))).toBe(true);
    expect(isProtectedContainer(container({}))).toBe(false);
  });
});

describe('canRunContainerUpdateOperation', () => {
  it('allows McHarbor update operations while keeping generic protected containers blocked', () => {
    expect(canRunContainerUpdateOperation(container({ Protected: true }))).toBe(false);
    expect(canRunContainerUpdateOperation(container({ Labels: { 'com.mcharbor.protected': 'yes' } }))).toBe(false);
    expect(canRunContainerUpdateOperation(container({ Image: 'ghcr.io/therealmcsparrow/mcharbor:latest' }))).toBe(true);
    expect(canRunContainerUpdateOperation(container({ Names: ['/mcharbor-mcharbor-1'] }))).toBe(true);
    expect(canRunContainerUpdateOperation(container({}))).toBe(true);
  });
});

describe('isProtectedImage', () => {
  it('does not lock McHarbor images', () => {
    expect(isProtectedImage(image({ Protected: true }))).toBe(false);
    expect(isProtectedImage(image({ Labels: { 'com.mcharbor.protected': '1' } }))).toBe(false);
    expect(isProtectedImage(image({ RepoTags: ['therealmcsparrow/mcharbor:latest'] }))).toBe(false);
    expect(isProtectedImage(image({ RepoTags: null, RepoDigests: ['mcharbor@sha256:abc'] }))).toBe(false);
    expect(isProtectedImage(image({}))).toBe(false);
  });
});

describe('isProtectedVolume', () => {
  it('does not lock McHarbor volumes', () => {
    expect(isProtectedVolume(volume({ Protected: true }))).toBe(false);
    expect(isProtectedVolume(volume({ Labels: { 'com.mcharbor.protected': 'true' } }))).toBe(false);
    expect(
      isProtectedVolume(
        volume({
          Labels: {
            'com.docker.compose.project': 'mcharbor',
            'com.docker.compose.volume': 'mcharbor-data',
          },
        }),
      ),
    ).toBe(false);
    expect(isProtectedVolume(volume({ Name: 'mcharbor-data' }))).toBe(false);
    expect(isProtectedVolume(volume({ Name: 'mcharbor_mcharbor-data' }))).toBe(false);
    expect(isProtectedVolume(volume({}))).toBe(false);
  });
});

describe('isProtectedStack', () => {
  it('does not lock McHarbor stacks', () => {
    expect(isProtectedStack({ name: 'mcharbor' })).toBe(false);
    expect(isProtectedStack({ name: ' MCHARBOR ' })).toBe(false);
    expect(isProtectedStack({ name: 'app', protected: true })).toBe(false);
    expect(isProtectedStack({ name: 'app' })).toBe(false);
  });
});
