// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
// i18n keys added by this file: none.

import { Badge } from './Badge';

export default {
  title: 'Primitives / Badge',
};

export const Variants = () => (
  <div className="flex flex-wrap items-center gap-3 p-8">
    <Badge variant="default">Default</Badge>
    <Badge variant="secondary">Secondary</Badge>
    <Badge variant="destructive">Destructive</Badge>
    <Badge variant="outline">Outline</Badge>
    <Badge variant="success">Success</Badge>
    <Badge variant="warning">Warning</Badge>
    <Badge variant="info">Info</Badge>
    <Badge variant="neutral">Neutral</Badge>
  </div>
);

export const StatusRow = () => (
  <div className="flex flex-wrap items-center gap-3 p-8">
    <Badge variant="success">running</Badge>
    <Badge variant="warning">paused</Badge>
    <Badge variant="destructive">exited</Badge>
    <Badge variant="info">created</Badge>
    <Badge variant="neutral">restarting</Badge>
  </div>
);
