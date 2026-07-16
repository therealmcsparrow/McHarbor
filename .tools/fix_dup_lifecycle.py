import json
import sys
import re

path = sys.argv[1]
with open(path, 'r', encoding='utf-8') as f:
    raw = f.read()
# Find all "lifecycle": occurrences
positions = []
i = 0
while True:
    j = raw.find('"lifecycle":', i)
    if j < 0:
        break
    positions.append(j)
    i = j + 1
print(f'found {len(positions)} occurrences')
# Find first "logs": block that follows the first lifecycle block
# If malformed, remove the lifecycle block
if len(positions) >= 2:
    # Remove the SECOND occurrence
    second_start = positions[1]
    # Find the next "logs" or close brace
    # Remove from the lifecycle to the next ',\n  "logs":' (the second 'logs')
    # Simpler: scan for a balanced removal of the second lifecycle block
    # Find the start of the line containing second_start
    line_start = raw.rfind('\n', 0, second_start) + 1
    # Find the next ",  "logs": {" or "  }" at the same indent
    pattern = re.compile(r'^\s*"logs":', re.MULTILINE)
    end = -1
    for m in pattern.finditer(raw, second_start):
        line = m.start()
        if raw[max(0, line-1):line] in ('\n', ' '):
            # same indent
            end = line
            break
    if end < 0:
        # fallback: next closing }
        end = raw.find('  }', second_start)
    if end > 0:
        raw = raw[:line_start] + raw[end:]
        print('removed second lifecycle block')
with open(path, 'w', encoding='utf-8') as f:
    f.write(raw)
# Validate
try:
    json.loads(raw)
    print('valid JSON')
except Exception as e:
    print(f'invalid: {e}')
