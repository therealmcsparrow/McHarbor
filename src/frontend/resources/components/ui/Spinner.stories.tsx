// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
// i18n keys added by this file: none.

import { Spinner } from './Spinner';
import { Box, Flex } from './Layout';
import { Caption } from './Typography';

export default {
  title: 'Primitives / Spinner',
};

export const Sizes = () => (
  <Flex align="center" gap={6} className="p-8">
    <Flex direction="col" align="center" gap={2}>
      <Spinner size="sm" />
      <Caption tone="muted">sm</Caption>
    </Flex>
    <Flex direction="col" align="center" gap={2}>
      <Spinner size="md" />
      <Caption tone="muted">md</Caption>
    </Flex>
    <Flex direction="col" align="center" gap={2}>
      <Spinner size="lg" />
      <Caption tone="muted">lg</Caption>
    </Flex>
    <Flex direction="col" align="center" gap={2}>
      <Spinner size="xl" />
      <Caption tone="muted">xl</Caption>
    </Flex>
  </Flex>
);

export const Inline = () => (
  <Box className="p-8">
    <Flex align="center" gap={2}>
      <Spinner size="sm" />
      <span className="text-sm text-muted-foreground">Loading containers…</span>
    </Flex>
  </Box>
);
