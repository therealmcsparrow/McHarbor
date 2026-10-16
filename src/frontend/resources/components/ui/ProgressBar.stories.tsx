// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
// i18n keys added by this file: none.

import { ProgressBar } from './ProgressBar';

export default {
  title: 'Primitives / ProgressBar',
};

export const Sizes = () => (
  <div className="max-w-md space-y-4 p-8">
    <ProgressBar value={25} size="sm" />
    <ProgressBar value={50} size="md" />
    <ProgressBar value={75} size="lg" />
  </div>
);

export const WithLabel = () => (
  <div className="max-w-md space-y-4 p-8">
    <ProgressBar value={33} showLabel />
    <ProgressBar value={66} showLabel />
    <ProgressBar value={100} showLabel />
  </div>
);

export const Indeterminate = () => (
  <div className="max-w-md space-y-4 p-8">
    <ProgressBar value={undefined as unknown as number} />
    <ProgressBar value={undefined as unknown as number} size="sm" />
  </div>
);
