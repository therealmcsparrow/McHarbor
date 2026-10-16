// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
// i18n keys added by this file: none.

import { Label } from './Label';
import { Input } from './Input';
import { Checkbox } from './Checkbox';
import { Switch } from './Switch';
import { Box, Stack } from './Layout';

export default {
  title: 'Primitives / Label',
};

export const Standalone = () => (
  <Box className="p-8">
    <Label>Display name</Label>
  </Box>
);

export const WithInput = () => (
  <Box className="max-w-sm p-8">
    <Stack gap={1.5}>
      <Label htmlFor="label-input-1">Container name</Label>
      <Input id="label-input-1" placeholder="nginx-proxy" />
    </Stack>
  </Box>
);

export const WithCheckbox = () => (
  <Box className="max-w-sm p-8">
    <div className="flex items-center gap-3">
      <Checkbox id="label-checkbox" defaultChecked />
      <Label htmlFor="label-checkbox" className="cursor-pointer">
        Notify on container start
      </Label>
    </div>
  </Box>
);

export const WithSwitch = () => (
  <Box className="max-w-sm p-8">
    <div className="flex items-center gap-3">
      <Switch id="label-switch" defaultChecked />
      <Label htmlFor="label-switch" className="cursor-pointer">
        Enable metrics collection
      </Label>
    </div>
  </Box>
);
