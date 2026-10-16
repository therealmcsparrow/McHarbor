// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
// i18n keys added by this file: none.

import { Tooltip, TooltipProvider, TooltipTrigger, TooltipContent } from './Tooltip';
import { Button } from './Button';
import { Kbd } from './Kbd';
import { Box } from './Layout';

export default {
  title: 'Primitives / Tooltip',
};

export const Basic = () => (
  <Box className="p-12">
    <TooltipProvider>
      <Tooltip>
        <TooltipTrigger asChild>
          <Button variant="outline">Hover me</Button>
        </TooltipTrigger>
        <TooltipContent>Standard tooltip on top.</TooltipContent>
      </Tooltip>
    </TooltipProvider>
  </Box>
);

export const WithShortcut = () => (
  <Box className="p-12">
    <TooltipProvider>
      <Tooltip>
        <TooltipTrigger asChild>
          <Button>Save</Button>
        </TooltipTrigger>
        <TooltipContent className="flex items-center gap-2">
          Save changes
          <Kbd size="sm" variant="muted">
            Ctrl+S
          </Kbd>
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  </Box>
);

export const OnIconButton = () => (
  <Box className="p-12">
    <TooltipProvider>
      <div className="flex gap-3">
        <Tooltip>
          <TooltipTrigger asChild>
            <Button variant="ghost" size="icon" aria-label="Refresh">
              ↻
            </Button>
          </TooltipTrigger>
          <TooltipContent>Refresh</TooltipContent>
        </Tooltip>
        <Tooltip>
          <TooltipTrigger asChild>
            <Button variant="ghost" size="icon" aria-label="Settings">
              ⚙
            </Button>
          </TooltipTrigger>
          <TooltipContent>Settings</TooltipContent>
        </Tooltip>
      </div>
    </TooltipProvider>
  </Box>
);
