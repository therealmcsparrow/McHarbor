// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
// i18n keys added by this file: none.

import { useState } from 'react';
import { Checkbox } from './Checkbox';
import { Box, Flex } from './Layout';

export default {
  title: 'Primitives / Checkbox',
};

export const Basic = () => {
  const [checked, setChecked] = useState(false);
  return (
    <Flex align="center" gap={3} className="p-8">
      <Checkbox checked={checked} onCheckedChange={(value) => setChecked(value === true)} />
      <span className="text-sm text-foreground">Accept terms</span>
    </Flex>
  );
};

export const Checked = () => (
  <Box className="p-8">
    <Checkbox defaultChecked />
  </Box>
);

export const Indeterminate = () => (
  <Box className="p-8">
    <Checkbox checked="indeterminate" />
  </Box>
);

export const Disabled = () => (
  <Flex direction="col" gap={3} className="p-8">
    <Flex align="center" gap={3}>
      <Checkbox disabled />
      <span className="text-sm text-muted-foreground">Off &amp; disabled</span>
    </Flex>
    <Flex align="center" gap={3}>
      <Checkbox disabled defaultChecked />
      <span className="text-sm text-muted-foreground">On &amp; disabled</span>
    </Flex>
  </Flex>
);
