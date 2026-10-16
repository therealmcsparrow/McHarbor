// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
// i18n keys added by this file: none.

import { EmptyState, InlineEmpty, NoResults } from './EmptyState';
import { IconBox, IconSearch } from '@tabler/icons-react';

export default {
  title: 'Primitives / EmptyState',
};

export const Default = () => (
  <div className="max-w-xl p-8">
    <EmptyState
      icon={<IconBox className="size-5" />}
      title="No containers yet"
      description="Pull an image from Docker Hub or create one from a Compose file to get started."
      primaryAction={{ label: 'Pull image', onClick: () => undefined }}
      secondaryAction={{ label: 'Import compose', onClick: () => undefined }}
    />
  </div>
);

export const Compact = () => (
  <div className="max-w-md p-8">
    <EmptyState
      variant="compact"
      icon={<IconBox className="size-4" />}
      title="No items"
      description="This widget has nothing to show yet."
    />
  </div>
);

export const Card = () => (
  <div className="max-w-md p-8">
    <EmptyState
      variant="default"
      icon={<IconBox className="size-5" />}
      title="Nothing scheduled"
      description="Backup schedules you create will appear here."
    />
  </div>
);

export const InlineEmptyStory = () => (
  <div className="max-w-md rounded-md border border-border p-8">
    <InlineEmpty icon={<IconSearch className="size-4" />} message="No matching plugins" />
  </div>
);

export const NoResultsStory = () => (
  <div className="max-w-xl p-8">
    <NoResults searchTerm="nginx" onClear={() => undefined} />
  </div>
);
