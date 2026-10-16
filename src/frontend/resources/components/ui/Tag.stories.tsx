// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
// i18n keys added by this file: none.

import { IconBox } from '@tabler/icons-react';
import { Tag } from './Tag';
import { Box, Flex } from './Layout';

export default {
  title: 'Primitives / Tag',
};

export const Tones = () => (
  <Flex wrap gap={2} className="p-8">
    <Tag tone="neutral">neutral</Tag>
    <Tag tone="primary">primary</Tag>
    <Tag tone="muted">muted</Tag>
    <Tag tone="success">success</Tag>
    <Tag tone="warning">warning</Tag>
    <Tag tone="destructive">destructive</Tag>
    <Tag tone="info">info</Tag>
  </Flex>
);

export const Sizes = () => (
  <Flex align="center" gap={2} className="p-8">
    <Tag size="sm" tone="primary">
      sm
    </Tag>
    <Tag size="md" tone="primary">
      md
    </Tag>
    <Tag size="lg" tone="primary">
      lg
    </Tag>
  </Flex>
);

export const WithIcon = () => (
  <Flex wrap gap={2} className="p-8">
    <Tag tone="info" icon={<IconBox className="size-3" />}>
      container
    </Tag>
    <Tag tone="primary" icon={<IconBox className="size-3" />}>
      stack
    </Tag>
  </Flex>
);

export const Dismissible = () => (
  <Box className="p-8">
    <Tag tone="muted" dismissible onDismiss={() => undefined}>
      removable tag
    </Tag>
  </Box>
);
