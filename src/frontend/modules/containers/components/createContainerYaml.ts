// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// parseSimpleYaml is a tiny, single-purpose YAML parser scoped to the
// subset of docker-compose we need: top-level mapping with nested
// mappings/sequences/scalars. Indentation-based blocks, no anchors,
// no flow-style, no tags. This keeps us off the `yaml` npm dependency
// and keeps the dialog bundle small.
//
// Exported for unit testing — not part of the public dialog API.
export function parseSimpleYaml(input: string): unknown {
  const lines = input.split(/\r?\n/);
  // Strip BOM
  if (lines.length > 0 && lines[0]?.charCodeAt(0) === 0xfeff) {
    lines[0] = lines[0].slice(1);
  }

  type Frame = { indent: number; container: unknown; isArray: boolean };
  // docker-compose YAML is always a top-level object map (with
  // `services:` as the first key), so we seed the root frame with an
  // empty object instead of null. Without this seed, the very first
  // top-level key (e.g. `services:`) hits `parent.container = null`
  // and throws "Cannot set properties of null".
  const rootObject: Record<string, unknown> = {};
  const stack: Frame[] = [{ indent: -1, container: rootObject, isArray: false }];
  let rootValue: unknown = rootObject;

  for (const rawLine of lines) {
    if (rawLine.trim() === '' || rawLine.trimStart().startsWith('#')) continue;

    const indentMatch = rawLine.match(/^(\s*)(.*)$/);
    if (!indentMatch) continue;
    const indent = indentMatch[1]?.length ?? 0;
    let content = indentMatch[2] ?? '';

    // Pop frames whose indent is greater than the current line.
    while (stack.length > 1) {
      const topFrame = stack[stack.length - 1];
      if (topFrame && topFrame.indent >= indent) {
        stack.pop();
      } else {
        break;
      }
    }

    const parentFrame = stack[stack.length - 1];
    if (!parentFrame) continue;

    // Helper: look ahead past blank lines / comments to the first
    // meaningful line. Returns its indent + content, or null if the
    // file ends here.
    function peekNextLine(): { indent: number; content: string } | null {
      let idx = lines.indexOf(rawLine) + 1;
      while (idx < lines.length) {
        const nextRaw = lines[idx] ?? '';
        if (nextRaw.trim() === '' || nextRaw.trimStart().startsWith('#')) {
          idx += 1;
          continue;
        }
        const m = nextRaw.match(/^(\s*)(.*)$/);
        return { indent: m?.[1]?.length ?? 0, content: m?.[2] ?? '' };
      }
      return null;
    }

    // Array item
    if (content.startsWith('- ')) {
      content = content.slice(2);
      if (!parentFrame.isArray) {
        // This shouldn't trigger anymore — `peekNextLine` above decides
        // array vs object when we create the placeholder. If it does
        // fire (e.g. malformed YAML), fall back to a real array so we
        // don't crash with `parent.container.push is not a function`.
        parentFrame.container = [];
        parentFrame.isArray = true;
      }
      const arr = parentFrame.container as unknown[];
      const value = parseScalar(content);
      arr.push(value);
      // If the array item is itself a mapping (next lines start at
      // greater indent), push a frame so those keys attach here.
      const next = peekNextLine();
      if (next && next.indent > indent && !next.content.startsWith('- ')) {
        const nextValue: Record<string, unknown> = {};
        arr[arr.length - 1] = nextValue;
        stack.push({ indent: next.indent, container: nextValue, isArray: false });
        continue;
      }
      continue;
    }

    // key: value pair
    const colonIdx = content.indexOf(':');
    if (colonIdx < 0) continue;
    const key = content.slice(0, colonIdx).trim();
    const rest = content.slice(colonIdx + 1).trim();
    const obj = parentFrame.container as Record<string, unknown>;
    if (rest === '') {
      // Look ahead to decide whether this is an object block or a
      // sequence block. Sequences start with `- ` on the next
      // meaningful line; everything else is an object block.
      const next = peekNextLine();
      const isSequenceBlock = next?.content.startsWith('- ') ?? false;
      const placeholder: unknown = isSequenceBlock ? [] : {};
      obj[key] = placeholder;
      rootValue ??= placeholder;
      if (next && next.indent > indent) {
        // Push the frame at the KEY's own indent, not the children's
        // indent. The frame's indent represents the column where this
        // key was declared; lines at exactly this column are its
        // siblings, lines at greater columns are its children. With
        // the pop condition `top.indent >= I`, that lets us pop a
        // sibling's frame correctly while still keeping the parent
        // around for its own children.
        stack.push({
          indent,
          container: placeholder,
          isArray: isSequenceBlock,
        });
      }
      continue;
    }
    // Inline value
    obj[key] = parseScalar(rest);
  }

  return stack[0]?.container ?? rootValue;
}

function parseScalar(input: string): unknown {
  let value = input.trim();
  // Strip trailing inline comment "#…"
  const hashIdx = value.indexOf(' #');
  if (hashIdx >= 0) value = value.slice(0, hashIdx).trim();
  if (value === '' || value === '~' || value === 'null') return null;
  if (value === 'true') return true;
  if (value === 'false') return false;
  // Quoted strings
  if ((value.startsWith('"') && value.endsWith('"')) || (value.startsWith("'") && value.endsWith("'"))) {
    return value.slice(1, -1);
  }
  if (/^-?\d+(\.\d+)?$/.test(value)) return Number(value);
  return value;
}
