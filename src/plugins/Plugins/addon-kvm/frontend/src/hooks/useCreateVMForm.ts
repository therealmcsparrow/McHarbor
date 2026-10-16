// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useCreateVMData } from './useCreateVMData';
import { useCreateVMFields } from './useCreateVMFields';
import { useCreateVMSubmit } from './useCreateVMSubmit';

export function useCreateVMForm(environments: { id: string; name: string }[], onCreated: (id: string) => void) {
  const fields = useCreateVMFields(environments);
  const data = useCreateVMData(fields);
  const actions = useCreateVMSubmit(fields, data, onCreated);
  return { ...fields, ...data, ...actions };
}

export type CreateVMForm = ReturnType<typeof useCreateVMForm>;
