import json
import os
import re

base = 'Z:/projects/web/management/McHarbor/src/frontend/core/i18n/locales'
locales = ['en', 'nl', 'de', 'es', 'fr', 'pt']

pages = [
    'src/frontend/modules/containers/pages/BackupsPage.tsx',
    'src/frontend/modules/containers/pages/backups-page-tables.tsx',
    'src/frontend/modules/containers/pages/backups-page-helpers.tsx',
    'src/frontend/modules/containers/components/BackupLogsTab.tsx',
    'src/frontend/modules/containers/components/BackupSelectionFields.tsx',
    'src/frontend/modules/containers/components/BackupRunDestinations.tsx',
    'src/frontend/modules/containers/components/RestoreRunDialog.tsx',
    'src/frontend/modules/containers/components/EditPlanDialog.tsx',
    'src/frontend/modules/containers/components/tabs/BackupsTab.tsx',
    'src/frontend/modules/containers/components/tabs/BackupRunDestinations.tsx',
]


def collect_keys(path):
    with open(path, 'r', encoding='utf-8') as f:
        text = f.read()
    keys = set()
    for m in re.finditer(r"t\(['\"]([a-zA-Z0-9._]+)['\"]", text):
        keys.add(m.group(1))
    for m in re.finditer(r"t\(`([a-zA-Z0-9._]+)`\)", text):
        keys.add(m.group(1))
    return keys


def lookup(data, key):
    parts = key.split('.')
    obj = data
    for p in parts:
        if isinstance(obj, dict) and p in obj:
            obj = obj[p]
        else:
            return None
    return obj


# Determine the root prefix for each key.
def root_for(key):
    if key.startswith('common.') or key.startswith('nav.'):
        return 'common.json'
    return 'containers.json'


def lookup_anywhere(data, key):
    """The page's t('backups.X') key may live anywhere in the JSON
    tree. Find the first match by walking the tree."""
    parts = key.split('.')
    def walk(node, idx):
        if idx == len(parts):
            return node
        if isinstance(node, dict):
            if parts[idx] in node:
                return walk(node[parts[idx]], idx + 1)
            for v in node.values():
                r = walk(v, idx)
                if r is not None:
                    return r
        return None
    return walk(data, 0)


all_keys = set()
for p in pages:
    try:
        all_keys.update(collect_keys(p))
    except FileNotFoundError:
        pass

print(f'Found {len(all_keys)} unique t() keys')
print()

for loc in locales:
    common = json.load(open(f'{base}/{loc}/common.json', encoding='utf-8'))
    containers = json.load(open(f'{base}/{loc}/containers.json', encoding='utf-8'))
    missing = []
    for k in sorted(all_keys):
        if k == 'a':
            continue
        target = common if root_for(k) == 'common.json' else containers
        if lookup_anywhere(target, k) is None:
            missing.append(k)
    print(f'{loc}: {len(missing)} missing')
    for k in missing:
        print(f'  {root_for(k):18s} {k}')
