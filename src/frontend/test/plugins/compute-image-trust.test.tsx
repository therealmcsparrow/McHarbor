// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { screen } from '@testing-library/react';
import { beforeAll, describe, expect, it } from 'vitest';
import { ImageTrustBadges } from '../../../plugins/Plugins/addon-compute/frontend/src/components/ImageBadges';
import type { ComputeImage } from '../../../plugins/Plugins/addon-compute/frontend/src/types-images';
import computeBundle from '../../../plugins/Plugins/addon-compute/i18n/en/compute.json';
import { i18n } from '../i18n';
import { renderWithProviders } from '../render';

function image(overrides: Partial<ComputeImage>): ComputeImage {
  return {
    id: 'img-x',
    kind: 'cloud_image',
    distro: 'debian',
    release: '13',
    arch: 'x86_64',
    displayName: 'Debian 13',
    sourceUrl: 'https://example.org/debian.qcow2',
    checksumUrl: 'https://example.org/SHA512SUMS',
    checksumAlgo: 'sha512',
    isBuiltin: true,
    origin: 'catalog',
    createdAt: '2026-10-01T00:00:00Z',
    updatedAt: '2026-10-01T00:00:00Z',
    ...overrides,
  };
}

describe('addon-compute image trust badges', () => {
  beforeAll(() => {
    i18n.addResourceBundle('en', 'compute', computeBundle, true, true);
  });

  it('marks a checksum-only image as not signed, and nothing else', () => {
    renderWithProviders(<ImageTrustBadges image={image({})} />);
    expect(screen.getByText('Not signed')).toBeInTheDocument();
    expect(screen.queryByText('This source publishes no checksum')).not.toBeInTheDocument();
  });

  it('does not call an image unsigned when the catalog names a signature or a host verified one', () => {
    const { rerender } = renderWithProviders(
      <ImageTrustBadges image={image({ signatureUrl: 'https://example.org/SHA512SUMS.gpg' })} />,
    );
    expect(screen.queryByText('Not signed')).not.toBeInTheDocument();

    rerender(
      <ImageTrustBadges
        image={image({
          cache: {
            id: 'c',
            imageId: 'img-x',
            environmentId: 'env',
            localPath: '/x',
            verified: true,
            signed: true,
            state: 'ready',
            progressPct: 100,
          },
        })}
      />,
    );
    expect(screen.queryByText('Not signed')).not.toBeInTheDocument();
  });

  it('leaves uploads and checksum-less entries to their own badges', () => {
    const { rerender } = renderWithProviders(<ImageTrustBadges image={image({ origin: 'upload' })} />);
    expect(screen.queryByText('Not signed')).not.toBeInTheDocument();
    rerender(<ImageTrustBadges image={image({ checksumUrl: undefined })} />);
    expect(screen.queryByText('Not signed')).not.toBeInTheDocument();
  });
});
