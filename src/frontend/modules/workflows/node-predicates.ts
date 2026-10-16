// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ConfigField } from './types';

export const PREDICATE_OPERATORS = [
  { value: '==', label: 'Equals' },
  { value: '!=', label: 'Not equal' },
  { value: '>', label: 'Greater than' },
  { value: '<', label: 'Less than' },
  { value: '>=', label: 'Greater than or equal' },
  { value: '<=', label: 'Less than or equal' },
  { value: 'contains', label: 'Contains' },
  { value: 'starts_with', label: 'Starts with' },
  { value: 'regex', label: 'Matches regex' },
  { value: 'is_empty', label: 'Is empty' },
  { value: 'is_not_empty', label: 'Is not empty' },
] as const;

export const PREDICATE_OPERATOR_VALUES: readonly string[] = PREDICATE_OPERATORS.map((operator) => operator.value);

export function predicateConfigFields(): ConfigField[] {
  return [
    {
      key: 'field',
      label: 'Field',
      type: 'expression',
      required: true,
      default: 'payload',
    },
    {
      key: 'operator',
      label: 'Operator',
      type: 'select',
      required: true,
      default: '==',
      options: PREDICATE_OPERATORS.map((operator) => ({ ...operator })),
    },
    {
      key: 'value',
      label: 'Value',
      type: 'text',
      required: false,
    },
  ];
}
