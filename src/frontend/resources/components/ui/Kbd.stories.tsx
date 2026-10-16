// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
// i18n keys added by this file: none.

import { Kbd } from './Kbd';
import { Flex } from './Layout';

export default {
  title: 'Primitives / Kbd',
};

export const Sizes = () => (
  <Flex align="center" gap={4} className="p-8">
    <Kbd size="sm">⌘ K</Kbd>
    <Kbd size="md">⌘ K</Kbd>
    <Kbd size="lg">⌘ K</Kbd>
  </Flex>
);

export const Variants = () => (
  <Flex align="center" gap={4} className="p-8">
    <Kbd variant="default">Enter</Kbd>
    <Kbd variant="muted">Esc</Kbd>
    <Kbd variant="primary">⌘ S</Kbd>
    <Kbd variant="destructive">⌫</Kbd>
  </Flex>
);

export const Chord = () => (
  <Flex align="center" gap={2} className="p-8">
    <span className="text-sm text-muted-foreground">Press</span>
    <Kbd>Ctrl</Kbd>
    <span className="text-sm text-muted-foreground">+</span>
    <Kbd>Shift</Kbd>
    <span className="text-sm text-muted-foreground">+</span>
    <Kbd variant="primary">P</Kbd>
    <span className="text-sm text-muted-foreground">to open the command palette.</span>
  </Flex>
);
