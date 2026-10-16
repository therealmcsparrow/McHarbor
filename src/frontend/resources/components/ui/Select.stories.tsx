// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
// i18n keys added by this file: none (Select reads `common.select.*`).

import { useState } from 'react';
import { Select, type SelectOption } from './Select';
import { Box, Stack } from './Layout';

const environmentOptions: SelectOption[] = [
  { value: 'local', label: 'Local Docker Desktop' },
  { value: 'prod', label: 'Production cluster' },
  { value: 'staging', label: 'Staging cluster' },
  { value: 'homelab', label: 'Homelab NAS' },
];

const driverOptions: SelectOption[] = [
  { value: 'bridge', label: 'bridge', description: 'Default NAT driver' },
  { value: 'host', label: 'host', description: 'Host networking' },
  { value: 'overlay', label: 'overlay', description: 'Swarm multi-host' },
  { value: 'macvlan', label: 'macvlan', description: 'MAC-level bridge' },
  { value: 'none', label: 'none', description: 'No networking' },
];

export default {
  title: 'Primitives / Select',
};

export const Basic = () => {
  const [value, setValue] = useState('local');
  return (
    <Box className="max-w-sm p-8">
      <Select value={value} onChange={setValue} options={environmentOptions} />
    </Box>
  );
};

export const WithDescriptions = () => {
  const [value, setValue] = useState('bridge');
  return (
    <Box className="max-w-sm p-8">
      <Select value={value} onChange={setValue} options={driverOptions} />
    </Box>
  );
};

export const OutlineVariant = () => {
  const [value, setValue] = useState('prod');
  return (
    <Box className="max-w-sm p-8">
      <Select variant="outline" value={value} onChange={setValue} options={environmentOptions} />
    </Box>
  );
};

export const Disabled = () => (
  <Box className="max-w-sm p-8">
    <Stack gap={3}>
      <Select value="staging" onChange={() => undefined} options={environmentOptions} disabled />
      <Select value="bridge" onChange={() => undefined} options={driverOptions} />
    </Stack>
  </Box>
);
