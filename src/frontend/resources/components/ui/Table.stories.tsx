// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
// i18n keys added by this file: none.

import { Badge } from './Badge';
import { Button } from './Button';
import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from './Table';

const rows = [
  { id: '1', name: 'nginx-proxy', state: 'running', cpu: '2.1%', memory: '128 MB' },
  { id: '2', name: 'postgres-primary', state: 'running', cpu: '12%', memory: '1.4 GB' },
  { id: '3', name: 'redis-cache', state: 'restarting', cpu: '0.4%', memory: '64 MB' },
  { id: '4', name: 'worker-beat', state: 'exited', cpu: '—', memory: '—' },
];

const stateVariant: Record<string, 'success' | 'warning' | 'destructive'> = {
  running: 'success',
  restarting: 'warning',
  exited: 'destructive',
};

export default {
  title: 'Primitives / Table',
};

export const Compact = () => (
  <div className="max-w-2xl p-8">
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>Name</TableHead>
          <TableHead>State</TableHead>
          <TableHead className="text-right">CPU</TableHead>
          <TableHead className="text-right">Memory</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {rows.map((row) => (
          <TableRow key={row.id}>
            <TableCell className="font-medium">{row.name}</TableCell>
            <TableCell>
              <Badge variant={stateVariant[row.state]}>{row.state}</Badge>
            </TableCell>
            <TableCell className="text-right font-mono">{row.cpu}</TableCell>
            <TableCell className="text-right font-mono">{row.memory}</TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  </div>
);

export const WithActions = () => (
  <div className="max-w-3xl p-8">
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>Name</TableHead>
          <TableHead>State</TableHead>
          <TableHead className="text-right">CPU</TableHead>
          <TableHead className="text-right">Memory</TableHead>
          <TableHead className="text-right">Actions</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {rows.map((row) => (
          <TableRow key={row.id}>
            <TableCell className="font-medium">{row.name}</TableCell>
            <TableCell>
              <Badge variant={stateVariant[row.state]}>{row.state}</Badge>
            </TableCell>
            <TableCell className="text-right font-mono">{row.cpu}</TableCell>
            <TableCell className="text-right font-mono">{row.memory}</TableCell>
            <TableCell className="text-right">
              <Button variant="ghost" size="sm">
                Manage
              </Button>
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  </div>
);
