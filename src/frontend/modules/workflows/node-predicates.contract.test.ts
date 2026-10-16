// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { describe, expect, it } from 'vitest';
import { PREDICATE_OPERATORS, PREDICATE_OPERATOR_VALUES } from './node-predicates';

// evaluateCondition's default branch returns true, so an operator the UI offers
// and the engine does not implement produces a condition that silently always
// passes: no error, no warning, and the canvas looks correct. That is how
// ExtraConditions came to offer >, <, >=, <= and starts_with against an engine
// that evaluated none of them.
//
// This test reads the Go switch rather than restating it, so the two cannot drift.
const SERVICE_GO = resolve(__dirname, '../../../backend/modules/workflows/service.go');

function engineOperators(): string[] {
  const source = readFileSync(SERVICE_GO, 'utf8');

  const start = source.indexOf('func evaluateCondition(');
  expect(start, 'evaluateCondition not found in service.go').toBeGreaterThan(-1);
  const switchStart = source.indexOf('switch operator {', start);
  expect(switchStart, 'the operator switch not found').toBeGreaterThan(-1);
  const switchEnd = source.indexOf('\n\t}', switchStart);
  const body = source.slice(switchStart, switchEnd);

  const found = new Set<string>();
  for (const match of body.matchAll(/case ((?:"[^"]+"(?:, )?)+):/g)) {
    for (const quoted of match[1]!.matchAll(/"([^"]+)"/g)) found.add(quoted[1]!);
  }
  return [...found];
}

describe('predicate operators are a contract with the workflow engine', () => {
  it('offers only operators evaluateCondition implements', () => {
    const engine = engineOperators();
    expect(engine.length).toBeGreaterThan(5);

    const offeredButUnimplemented = PREDICATE_OPERATOR_VALUES.filter((op) => !engine.includes(op));
    expect(
      offeredButUnimplemented,
      'these operators would silently always pass, because evaluateCondition falls through to `default: return true`',
    ).toEqual([]);
  });

  it('offers every operator the engine implements, so none is unreachable', () => {
    const engine = engineOperators();

    const implementedButHidden = engine.filter((op) => !PREDICATE_OPERATOR_VALUES.includes(op));
    expect(
      implementedButHidden,
      'the engine evaluates these but no UI offers them; add them to PREDICATE_OPERATORS',
    ).toEqual([]);
  });

  it('gives every operator a distinct value and a non-empty label', () => {
    const values = PREDICATE_OPERATORS.map((operator) => operator.value);
    expect(new Set(values).size).toBe(values.length);

    for (const operator of PREDICATE_OPERATORS) {
      expect(operator.label.length, `${operator.value} needs a label`).toBeGreaterThan(0);
    }
  });
});
