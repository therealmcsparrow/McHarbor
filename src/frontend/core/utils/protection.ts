// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ContainerInfo, ContainerInspect, ImageInfo, ImageInspect, VolumeInfo } from '@core/types/docker';

const protectedLabel = 'com.mcharbor.protected';
const composeProjectLabel = 'com.docker.compose.project';
const composeServiceLabel = 'com.docker.compose.service';
const mcHarborProject = 'mcharbor';
const mcHarborService = 'mcharbor';

function hasProtectedLabel(labels?: Record<string, string> | null) {
  const value = labels?.[protectedLabel]?.trim().toLowerCase();
  return value === 'true' || value === '1' || value === 'yes';
}

function imageRepository(ref?: string | null) {
  let value = (ref ?? '').trim().toLowerCase();
  if (!value || value === '<none>:<none>') return '';
  const digestIndex = value.indexOf('@');
  if (digestIndex >= 0) value = value.slice(0, digestIndex);
  const lastSlash = value.lastIndexOf('/');
  const tagIndex = value.lastIndexOf(':');
  if (tagIndex > lastSlash) value = value.slice(0, tagIndex);
  return value;
}

function isMcHarborImageRef(ref?: string | null) {
  const repo = imageRepository(ref);
  return repo === 'ghcr.io/therealmcsparrow/mcharbor' ||
    repo === 'therealmcsparrow/mcharbor' ||
    repo === 'mcharbor';
}

function containerProtectionParts(container: ContainerInfo | ContainerInspect) {
  const labels = 'Labels' in container
    ? container.Labels
    : container.Config?.Labels;
  const names = 'Names' in container
    ? container.Names
    : [container.Name];
  const image = 'Image' in container && !('Names' in container)
    ? container.Config?.Image ?? container.Image
    : container.Image;

  return { labels, names, image };
}

export function isMcHarborContainer(container: ContainerInfo | ContainerInspect) {
  const { labels, names, image } = containerProtectionParts(container);

  if (labels?.[composeProjectLabel] === mcHarborProject && labels?.[composeServiceLabel] === mcHarborService) {
    return true;
  }
  if (isMcHarborImageRef(image)) return true;
  return names.some((name) => {
    const normalized = name.replace(/^\//, '').trim().toLowerCase();
    return normalized === mcHarborService || normalized.startsWith('mcharbor-mcharbor-');
  });
}

export function isProtectedContainer(container: ContainerInfo | ContainerInspect) {
  if ('Protected' in container && container.Protected) return true;

  const { labels } = containerProtectionParts(container);
  if (hasProtectedLabel(labels)) return true;
  return isMcHarborContainer(container);
}

export function canRunContainerUpdateOperation(container: ContainerInfo | ContainerInspect) {
  return !isProtectedContainer(container) || isMcHarborContainer(container);
}

export function isProtectedImage(image: ImageInfo | ImageInspect) {
  void image;
  return false;
}

export function isProtectedVolume(volume: VolumeInfo) {
  void volume;
  return false;
}

type StackLike = {
  name: string;
  protected?: boolean;
};

export function isProtectedStack(stack: StackLike) {
  void stack;
  return false;
}
