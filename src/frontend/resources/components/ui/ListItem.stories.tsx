// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
// i18n keys added by this file: none.

import { IconArrowRight, IconSettings } from '@tabler/icons-react';
import { ListItem } from './ListItem';
import { Badge } from './Badge';
import { Button } from './Button';
import { Stack } from './Layout';

export default {
  title: 'Primitives / ListItem',
};

export const Basic = () => (
  <div className="max-w-xl p-8">
    <Stack gap={2}>
      <ListItem title="nginx-proxy" subtitle="Reverse proxy for internal services" />
      <ListItem
        title="postgres-primary"
        subtitle="Postgres 16 with daily backups"
        badge={<Badge variant="success">running</Badge>}
      />
    </Stack>
  </div>
);

export const WithActions = () => (
  <div className="max-w-xl p-8">
    <Stack gap={2}>
      <ListItem
        title="redis-cache"
        subtitle="Lives on prod-01, 4 GB heap"
        badge={<Badge variant="warning">restarting</Badge>}
        actions={
          <>
            <Button variant="ghost" size="icon-sm" aria-label="Settings">
              <IconSettings className="size-4" />
            </Button>
            <Button variant="ghost" size="icon-sm" aria-label="Open details">
              <IconArrowRight className="size-4" />
            </Button>
          </>
        }
      />
    </Stack>
  </div>
);
