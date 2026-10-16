// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
// i18n keys added by this file: none.

import { Avatar } from './Avatar';
import { Flex, Stack } from './Layout';
import { Caption } from './Typography';

export default {
  title: 'Primitives / Avatar',
};

export const Sizes = () => (
  <Flex align="center" gap={4} className="p-8">
    <Stack align="center" gap={1}>
      <Avatar name="Jane Doe" size="xs" />
      <Caption tone="muted">xs</Caption>
    </Stack>
    <Stack align="center" gap={1}>
      <Avatar name="Jane Doe" size="sm" />
      <Caption tone="muted">sm</Caption>
    </Stack>
    <Stack align="center" gap={1}>
      <Avatar name="Jane Doe" size="md" />
      <Caption tone="muted">md</Caption>
    </Stack>
    <Stack align="center" gap={1}>
      <Avatar name="Jane Doe" size="lg" />
      <Caption tone="muted">lg</Caption>
    </Stack>
    <Stack align="center" gap={1}>
      <Avatar name="Jane Doe" size="xl" />
      <Caption tone="muted">xl</Caption>
    </Stack>
  </Flex>
);

export const WithImage = () => (
  <Flex gap={3} className="p-8">
    <Avatar
      src="data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 24 24'%3E%3Crect width='24' height='24' fill='%237c3aed'/%3E%3C/svg%3E"
      alt="Avatar"
    />
    <Avatar name="Maria Sanchez" />
  </Flex>
);

export const Initials = () => (
  <Flex align="center" gap={3} className="p-8">
    <Avatar name="Ada Lovelace" />
    <Avatar name="Grace Hopper" />
    <Avatar name="Linus" />
    <Avatar />
  </Flex>
);
