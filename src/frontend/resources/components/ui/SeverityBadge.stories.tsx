// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
// i18n keys added by this file: none.

import { SeverityBadge } from './SeverityBadge';

export default {
  title: 'Primitives / SeverityBadge',
};

export const AllLevels = () => (
  <div className="flex flex-wrap items-center gap-3 p-8">
    <SeverityBadge level="info" />
    <SeverityBadge level="success" />
    <SeverityBadge level="warning" />
    <SeverityBadge level="error" />
    <SeverityBadge level="critical" />
  </div>
);

export const WithCustomLabels = () => (
  <div className="flex flex-wrap items-center gap-3 p-8">
    <SeverityBadge level="info" label="Build queued" />
    <SeverityBadge level="success" label="Image pulled" />
    <SeverityBadge level="warning" label="Slow response" />
    <SeverityBadge level="error" label="Build failed" />
    <SeverityBadge level="critical" label="Backend offline" />
  </div>
);
