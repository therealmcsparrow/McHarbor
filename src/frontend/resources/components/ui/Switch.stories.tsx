// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
// i18n keys added by this file: none.

import { useState } from 'react';
import { Switch } from './Switch';
import { Box, Flex } from './Layout';
import { Caption } from './Typography';

export default {
  title: 'Primitives / Switch',
};

export const Basic = () => {
  const [on, setOn] = useState(false);
  return (
    <Flex align="center" gap={3} className="p-8">
      <Switch checked={on} onCheckedChange={setOn} />
      <Caption>{on ? 'On' : 'Off'}</Caption>
    </Flex>
  );
};

export const Checked = () => (
  <Box className="p-8">
    <Switch defaultChecked />
  </Box>
);

export const Disabled = () => (
  <Flex direction="col" gap={3} className="p-8">
    <Flex align="center" gap={3}>
      <Switch disabled />
      <Caption tone="muted">Off &amp; disabled</Caption>
    </Flex>
    <Flex align="center" gap={3}>
      <Switch disabled defaultChecked />
      <Caption tone="muted">On &amp; disabled</Caption>
    </Flex>
  </Flex>
);

export const WithLabel = () => {
  const [autoUpdate, setAutoUpdate] = useState(true);
  return (
    <Flex align="center" justify="between" gap={3} className="max-w-md rounded-lg border border-border p-4">
      <Box className="min-w-0">
        <p className="text-sm font-medium text-foreground">Auto-update agents</p>
        <p className="mt-0.5 text-xs text-muted-foreground">Refresh remote agent binaries nightly.</p>
      </Box>
      <Switch checked={autoUpdate} onCheckedChange={setAutoUpdate} />
    </Flex>
  );
};
