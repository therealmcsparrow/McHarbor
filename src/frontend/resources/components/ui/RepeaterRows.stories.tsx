// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
// i18n keys added by this file: none (RepeaterRows uses caller-supplied labels).

import { useState } from 'react';
import { IconNetwork } from '@tabler/icons-react';
import { RepeaterRows } from './RepeaterRows';
import { Input } from './Input';
import { NumberInput } from './NumberInput';
import { Box } from './Layout';

type PortProtocol = 'tcp' | 'udp';
type Port = { host: number; container: number; protocol: PortProtocol };

const protocolOptions: PortProtocol[] = ['tcp', 'udp'];

export default {
  title: 'Primitives / RepeaterRows',
};

export const PortMappings = () => {
  const [ports, setPorts] = useState<Port[]>([{ host: 8080, container: 80, protocol: 'tcp' }]);
  return (
    <Box className="max-w-xl p-8">
      <RepeaterRows
        items={ports}
        onChange={setPorts}
        addLabel="Add port mapping"
        onAdd={() => ({ host: 0, container: 0, protocol: 'tcp' as PortProtocol })}
        renderRow={(item, _index, helpers) => (
          <div className="mc-glass grid grid-cols-[1fr_1fr_1fr_auto] items-center gap-2 rounded-lg p-4">
            <NumberInput
              value={item.host}
              min={0}
              max={65535}
              onChange={(host) => {
                const idx = ports.indexOf(item);
                const next = [...ports];
                next[idx] = { ...item, host };
                setPorts(next);
              }}
              placeholder="Host"
            />
            <NumberInput
              value={item.container}
              min={0}
              max={65535}
              onChange={(container) => {
                const idx = ports.indexOf(item);
                const next = [...ports];
                next[idx] = { ...item, container };
                setPorts(next);
              }}
              placeholder="Container"
            />
            <select
              className="mc-glass h-10 rounded-lg px-3 text-sm"
              value={item.protocol}
              onChange={(e) => {
                const idx = ports.indexOf(item);
                const next = [...ports];
                next[idx] = { ...item, protocol: e.target.value as 'tcp' | 'udp' };
                setPorts(next);
              }}
            >
              {protocolOptions.map((p) => (
                <option key={p} value={p}>
                  {p}
                </option>
              ))}
            </select>
            <button
              type="button"
              onClick={helpers.remove}
              className="rounded-md p-1 text-muted-foreground hover:bg-destructive/10 hover:text-destructive"
              aria-label="Remove port mapping"
            >
              ×
            </button>
          </div>
        )}
      />
    </Box>
  );
};

export const WithEmptyState = () => {
  const [items, setItems] = useState<{ name: string }[]>([]);
  return (
    <Box className="max-w-md p-8">
      <RepeaterRows
        items={items}
        onChange={setItems}
        addLabel="Add network"
        onAdd={() => ({ name: '' })}
        emptyState={{
          icon: <IconNetwork className="size-5" />,
          title: 'No networks attached',
          description: 'Click "Add network" to attach the first one.',
        }}
        renderRow={(item, _index, helpers) => (
          <div className="mc-glass flex items-center gap-2 rounded-lg p-4">
            <Input
              value={item.name}
              onChange={(e) => {
                const idx = items.indexOf(item);
                const next = [...items];
                next[idx] = { name: e.target.value };
                setItems(next);
              }}
              placeholder="bridge"
            />
            <button
              type="button"
              onClick={helpers.remove}
              className="rounded-md p-1 text-muted-foreground hover:bg-destructive/10 hover:text-destructive"
              aria-label="Remove network"
            >
              ×
            </button>
          </div>
        )}
      />
    </Box>
  );
};
