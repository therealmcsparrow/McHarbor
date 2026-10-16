// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
// i18n keys added by this file: none.

import { useState } from 'react';
import { NumberInput } from './NumberInput';
import { Box, Stack } from './Layout';

export default {
  title: 'Primitives / NumberInput',
};

export const Basic = () => {
  const [value, setValue] = useState(10);
  return (
    <Box className="max-w-sm p-8">
      <NumberInput value={value} onChange={setValue} min={0} max={100} />
    </Box>
  );
};

export const WithStep = () => {
  const [value, setValue] = useState(1.5);
  return (
    <Box className="max-w-sm p-8">
      <NumberInput value={value} onChange={setValue} min={0} max={10} step={0.5} />
    </Box>
  );
};

export const Small = () => {
  const [value, setValue] = useState(30);
  return (
    <Box className="max-w-[180px] p-8">
      <NumberInput size="sm" value={value} onChange={setValue} min={5} max={300} step={5} />
    </Box>
  );
};

export const WithSuffix = () => {
  const [value, setValue] = useState(512);
  return (
    <Box className="max-w-sm p-8">
      <NumberInput value={value} onChange={setValue} min={128} step={128} suffix="MiB" />
    </Box>
  );
};

export const Nullable = () => {
  const [value, setValue] = useState<number | null>(null);
  return (
    <Box className="max-w-sm p-8">
      <NumberInput nullable value={value} onChange={setValue} min={1} max={4094} placeholder="VLAN" />
    </Box>
  );
};

export const Disabled = () => (
  <Stack gap={4} className="max-w-sm p-8">
    <NumberInput value={42} onChange={() => undefined} disabled />
    <NumberInput size="sm" value={8} onChange={() => undefined} disabled />
  </Stack>
);
