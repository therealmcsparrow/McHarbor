// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { IconCheck, IconCopy } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Flex, Stack } from '@resources/components/ui/Layout';
import { Code } from '@resources/components/ui/Typography';

type CopyHandler = (value: string, key: string, label: string) => void;

type CommandListProps = {
  commands: string[];
  copiedKey: string | null;
  copyCommandLabel: string;
  onCopy: CopyHandler;
};

type CopySnippetProps = {
  value: string;
  copyKey: string;
  copiedKey: string | null;
  label: string;
  ariaLabel: string;
  onCopy: CopyHandler;
};

export function HAMigrationCommandList({ commands, copiedKey, copyCommandLabel, onCopy }: CommandListProps) {
  return (
    <Stack gap={2}>
      {commands.map((command) => (
        <HAMigrationCopySnippet
          key={command}
          value={command}
          copyKey={`command:${command}`}
          copiedKey={copiedKey}
          label={command}
          ariaLabel={copyCommandLabel}
          onCopy={onCopy}
        />
      ))}
    </Stack>
  );
}

export function HAMigrationCopySnippet({ value, copyKey, copiedKey, label, ariaLabel, onCopy }: CopySnippetProps) {
  return (
    <Flex align="center" gap={2} className="rounded-md bg-muted p-2">
      <Code className="min-w-0 flex-1 overflow-x-auto whitespace-pre">{value}</Code>
      <Button variant="ghost" size="icon" aria-label={ariaLabel} onClick={() => onCopy(value, copyKey, label)}>
        {copiedKey === copyKey ? <IconCheck className="size-3.5" /> : <IconCopy className="size-3.5" />}
      </Button>
    </Flex>
  );
}
