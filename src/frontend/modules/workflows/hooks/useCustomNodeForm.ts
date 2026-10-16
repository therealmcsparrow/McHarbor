// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { CustomNodeWithCode } from './useCustomNodeSync';
import { useCreateCustomNode, useTestCustomNode, useUpdateCustomNode, type CustomNodeInput } from './useCustomNodes';

export const CUSTOM_NODE_CATEGORIES = ['trigger', 'action', 'logic', 'utility', 'integration'] as const;

const STARTER_CODE = `// msg is the incoming message, config the node's settings.
// Return the message to pass it on.
msg.payload = msg.payload ?? {};
return msg;
`;

/** Ports are edited as a comma-separated list; the API takes an array. */
const splitPorts = (value: string) =>
  value
    .split(',')
    .map((p) => p.trim())
    .filter(Boolean);

/**
 * Form state and the three mutations behind the custom-node editor.
 *
 * Extracted from `CustomNodeDialog` so the dialog stays a rendering concern:
 * eleven pieces of field state plus create/update/test wiring pushed it past
 * the 200-line component limit on its own.
 */
export function useCustomNodeForm(node: CustomNodeWithCode | undefined, onSaved: () => void) {
  const { t } = useTranslation('common');
  const createNode = useCreateCustomNode();
  const updateNode = useUpdateCustomNode();
  const testNode = useTestCustomNode();

  const editing = !!node;
  const [key, setKey] = useState(node?.key ?? '');
  const [label, setLabel] = useState(node?.label ?? '');
  const [description, setDescription] = useState(node?.description ?? '');
  const [category, setCategory] = useState<CustomNodeWithCode['category']>(node?.category ?? 'action');
  const [icon, setIcon] = useState(node?.icon ?? 'IconBolt');
  const [inputPorts, setInputPorts] = useState((node?.inputPorts ?? ['in']).join(', '));
  const [outputPorts, setOutputPorts] = useState((node?.outputPorts ?? ['out']).join(', '));
  const [code, setCode] = useState(node?.code ?? STARTER_CODE);
  const [testMsg, setTestMsg] = useState('{\n  "payload": {}\n}');
  const [testMsgError, setTestMsgError] = useState<string | null>(null);

  const buildInput = (): CustomNodeInput => ({
    definition: {
      key,
      label,
      category,
      description,
      icon,
      source: 'custom',
      configSchema: node?.configSchema ?? [],
      inputPorts: splitPorts(inputPorts),
      outputPorts: splitPorts(outputPorts),
    },
    code,
    translations: node?.translations ?? {},
  });

  // The test route takes the *unsaved* script, which is the point: a script can
  // be tried before it is stored. An unparseable test message is reported
  // inline rather than thrown, so a stray comma does not read as a sandbox
  // failure.
  function runTest() {
    let parsed: Record<string, unknown>;
    try {
      parsed = JSON.parse(testMsg) as Record<string, unknown>;
      setTestMsgError(null);
    } catch {
      setTestMsgError(t('customNodes.testInput'));
      return;
    }
    testNode.mutate({ code, config: {}, msg: parsed });
  }

  function save() {
    const input = buildInput();
    if (editing && node) {
      updateNode.mutate({ key: node.key, input }, { onSuccess: onSaved });
      return;
    }
    createNode.mutate(input, { onSuccess: onSaved });
  }

  return {
    editing,
    fields: {
      key,
      setKey,
      label,
      setLabel,
      description,
      setDescription,
      category,
      setCategory,
      icon,
      setIcon,
      inputPorts,
      setInputPorts,
      outputPorts,
      setOutputPorts,
      code,
      setCode,
      testMsg,
      setTestMsg,
    },
    testMsgError,
    testResult: testNode.data,
    testing: testNode.isPending,
    saving: createNode.isPending || updateNode.isPending,
    runTest,
    save,
  };
}
