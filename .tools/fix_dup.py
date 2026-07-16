import json
import sys

path = sys.argv[1]
with open(path, 'r', encoding='utf-8') as f:
    raw = f.read()
needle = '    "logging": "Lifecycle Log",\n'
positions = []
i = 0
while True:
    j = raw.find(needle, i)
    if j < 0:
        break
    positions.append(j)
    i = j + len(needle)
print('found', len(positions), 'occurrences of the duplicate nav line')
if len(positions) > 1:
    raw = raw[:positions[1]] + raw[positions[1] + len(needle):]
    with open(path, 'w', encoding='utf-8') as f:
        f.write(raw)
    print('removed second occurrence')
# Verify
data = json.loads(raw)
nav = data.get('nav', {})
print('nav.logging =', nav.get('logging'))
print('lifecycle present =', 'lifecycle' in data)
