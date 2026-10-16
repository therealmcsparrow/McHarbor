// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
// i18n keys added by this file: none.

import { Alert } from './Alert';
import { Button } from './Button';

export default {
  title: 'Primitives / Alert',
};

export const Variants = () => (
  <div className="flex flex-col gap-3 p-8">
    <Alert
      variant="info"
      title="Heads up"
      description="Docker 28 will be required to run this image after the next restart."
    />
    <Alert variant="success" title="Backup complete" description="All 12 containers snapshotted to the local volume." />
    <Alert
      variant="warning"
      title="Stream mode"
      description="High event volume — consider switching to poll mode on slow links."
    />
    <Alert
      variant="destructive"
      title="Backend offline"
      description="Could not reach the McHarbor API on localhost:5474."
    />
  </div>
);

export const WithAction = () => (
  <div className="flex flex-col gap-3 p-8">
    <Alert
      variant="warning"
      title="Unsaved changes"
      description="You have 3 pending edits that will be lost if you leave now."
      action={<Button size="sm">Review</Button>}
    />
  </div>
);

export const NoTitle = () => (
  <div className="flex flex-col gap-3 p-8">
    <Alert variant="info" description="Inline alert with no title — for short one-line callouts only." />
  </div>
);
