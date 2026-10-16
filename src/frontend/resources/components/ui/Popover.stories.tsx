// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
// i18n keys added by this file: none.

import { Popover, PopoverTrigger, PopoverContent } from './Popover';
import { Button } from './Button';
import { Box, Flex } from './Layout';
import { Caption } from './Typography';

export default {
  title: 'Primitives / Popover',
};

export const Basic = () => (
  <div className="p-12">
    <Popover>
      <PopoverTrigger asChild>
        <Button variant="outline">Open popover</Button>
      </PopoverTrigger>
      <PopoverContent>
        <Flex direction="col" gap={2}>
          <Caption size="xs" tone="muted" className="tracking-wider uppercase">
            About this container
          </Caption>
          <p className="text-sm text-foreground">
            Created 3 days ago from <span className="font-mono">nginx:1.27</span>. Currently running on host{' '}
            <span className="font-mono">prod-01</span>.
          </p>
        </Flex>
      </PopoverContent>
    </Popover>
  </div>
);

export const WithActions = () => (
  <div className="p-12">
    <Popover>
      <PopoverTrigger asChild>
        <Button>Container actions</Button>
      </PopoverTrigger>
      <PopoverContent className="w-64">
        <Flex direction="col" gap={1}>
          <Caption size="xs" tone="muted" className="tracking-wider uppercase">
            Quick actions
          </Caption>
          <button type="button" className="rounded-md px-2 py-1.5 text-left text-sm hover:bg-muted/50">
            Restart
          </button>
          <button type="button" className="rounded-md px-2 py-1.5 text-left text-sm hover:bg-muted/50">
            View logs
          </button>
          <button type="button" className="rounded-md px-2 py-1.5 text-left text-sm hover:bg-muted/50">
            Open shell
          </button>
          <Box className="my-1 border-t border-border" />
          <button
            type="button"
            className="rounded-md px-2 py-1.5 text-left text-sm text-destructive hover:bg-destructive/10"
          >
            Remove
          </button>
        </Flex>
      </PopoverContent>
    </Popover>
  </div>
);

export const AlignStart = () => (
  <div className="p-12">
    <Popover>
      <PopoverTrigger asChild>
        <Button variant="ghost">Left-aligned popover</Button>
      </PopoverTrigger>
      <PopoverContent align="start">
        <p className="text-sm text-foreground">The content sits flush with the left edge of the trigger.</p>
      </PopoverContent>
    </Popover>
  </div>
);
