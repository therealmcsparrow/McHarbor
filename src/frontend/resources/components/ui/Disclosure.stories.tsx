// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
// i18n keys added by this file: none.

import { IconShieldHalfFilled } from '@tabler/icons-react';
import { Disclosure } from './Disclosure';
import { Badge } from './Badge';
import { Box, Stack } from './Layout';

export default {
  title: 'Primitives / Disclosure',
};

export const Basic = () => (
  <Box className="max-w-xl p-8">
    <Disclosure title="Advanced settings">
      <p className="text-sm text-muted-foreground">
        Toggle these only if you know what you're doing. Defaults work for the vast majority of installs.
      </p>
    </Disclosure>
  </Box>
);

export const DefaultOpen = () => (
  <Box className="max-w-xl p-8">
    <Disclosure title="Resource limits" defaultOpen>
      <p className="text-sm text-muted-foreground">Defaults to 1 GB RAM and 1 CPU per container.</p>
    </Disclosure>
  </Box>
);

export const WithIconAndBadge = () => (
  <Box className="max-w-xl p-8">
    <Stack gap={3}>
      <Disclosure
        title="Security options"
        description="AppArmor / SELinux labelling rules"
        icon={<IconShieldHalfFilled className="size-4" />}
        trailing={<Badge variant="warning">3 overrides</Badge>}
      >
        <p className="text-sm text-muted-foreground">Custom security profiles take effect on next container start.</p>
      </Disclosure>
    </Stack>
  </Box>
);

export const Disabled = () => (
  <Box className="max-w-xl p-8">
    <Disclosure title="Read-only configuration" disabled>
      <p className="text-sm text-muted-foreground">The body is hidden because the header is disabled.</p>
    </Disclosure>
  </Box>
);
