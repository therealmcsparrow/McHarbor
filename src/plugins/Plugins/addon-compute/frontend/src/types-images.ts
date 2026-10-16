// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ImageKind } from './types';

export interface ImageCache {
  id: string;
  imageId: string;
  environmentId: string;
  localPath: string;
  sizeBytes?: number;
  verified: boolean;
  signed: boolean;
  state: 'pending' | 'downloading' | 'ready' | 'failed';
  progressPct: number;
  phase?: string;
  lastError?: string;
  lastUsedAt?: string;
}

export interface ComputeImage {
  id: string;
  kind: ImageKind;
  distro: string;
  release: string;
  arch: string;
  variant?: string;
  displayName: string;
  sourceUrl: string;
  checksumUrl?: string;
  checksum?: string;
  checksumAlgo: string;
  signatureUrl?: string;
  sizeBytes?: number;
  isBuiltin: boolean;
  initSystem?: string;
  netConfigFormat?: string;
  origin: ImageOrigin;
  libraryFile?: string;
  uploadedBy?: string;
  uploadedAt?: string;
  cache?: ImageCache;
  createdAt: string;
  updatedAt: string;
}

export type ImageOrigin = 'catalog' | 'upload';

export interface ImageLibrary {
  path: string;
  agentMount: string;
  writable: boolean;
  lastError?: string;
  maxSizeBytes: number;
  usedBytes: number;
  fileCount: number;
}
