// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
// i18n keys added by this file: none.

import { SectionHeader } from './SectionHeader';
import { Button } from './Button';
import { Box } from './Layout';

export default {
  title: 'Primitives / SectionHeader',
};

export const Basic = () => (
  <Box className="max-w-md p-8">
    <SectionHeader title="Network configuration" />
  </Box>
);

export const WithDescription = () => (
  <Box className="max-w-md p-8">
    <SectionHeader title="Backup schedule" description="Backups run nightly at 02:00 in the container timezone." />
  </Box>
);

export const WithAction = () => (
  <Box className="max-w-md p-8">
    <SectionHeader
      title="Volumes"
      description="Persistent storage attached to one or more containers."
      action={<Button size="sm">Create volume</Button>}
    />
  </Box>
);
