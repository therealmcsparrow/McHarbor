// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
// i18n keys added by this file: none.

import { Input } from './Input';

export default {
  title: 'Primitives / Input',
};

export const Variants = () => (
  <div className="flex max-w-md flex-col gap-4 p-8">
    <Input variant="default" placeholder="Default input" />
    <Input variant="outline" placeholder="Outline input" />
    <Input variant="default" placeholder="Disabled" disabled />
    <Input variant="default" placeholder="Invalid" aria-invalid="true" />
    <Input variant="default" placeholder="Read-only" readOnly value="Read-only value" />
  </div>
);

export const WithLabel = () => (
  <div className="flex max-w-md flex-col gap-4 p-8">
    <div className="space-y-1.5">
      <label htmlFor="input-story-name" className="text-xs font-medium text-muted-foreground">
        Container name
      </label>
      <Input id="input-story-name" placeholder="nginx-proxy" />
    </div>
    <div className="space-y-1.5">
      <label htmlFor="input-story-search" className="text-xs font-medium text-muted-foreground">
        Search
      </label>
      <Input id="input-story-search" placeholder="Search containers…" type="search" />
    </div>
  </div>
);
