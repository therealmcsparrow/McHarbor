// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { create } from 'zustand';
import type { NodeDefinition } from '../types';

type CustomNodeRegistryState = {
  customNodes: NodeDefinition[];
  setCustomNodes: (nodes: NodeDefinition[]) => void;
};

export const useCustomNodeRegistry = create<CustomNodeRegistryState>((set) => ({
  customNodes: [],
  setCustomNodes: (nodes) => set({ customNodes: nodes }),
}));
