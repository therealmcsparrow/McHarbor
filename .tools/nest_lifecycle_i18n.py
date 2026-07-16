import json
import os

base = 'Z:/projects/web/management/McHarbor/src/frontend/core/i18n/locales'
locales = ['en', 'nl', 'de', 'es', 'fr', 'pt']

# i18next splits the lookup key on '.'. 'backups.logs.severity.all'
# becomes ['backups','logs','severity','all'], so the JSON must have:
#   backups.logs.severity = { all: "..." }
# not the flat "severity.all": "..."
# The fix is to take any key in 'backups.logs.*' that contains a '.'
# and promote it to a nested object.

PROMOTE = {
    'severity.all', 'severity.info', 'severity.success', 'severity.warning', 'severity.error',
    'action.start', 'action.stop', 'action.delete', 'action.upload', 'action.fail',
    'action.complete', 'action.create', 'action.destroy', 'action.phase', 'action.audit',
    'table.time', 'table.environment', 'table.plan', 'table.container',
    'table.action', 'table.phase', 'table.message', 'table.severity',
    'table.duration', 'table.details',
}

def promote(obj):
    if not isinstance(obj, dict):
        return
    for k, v in list(obj.items()):
        if not isinstance(v, str) or '.' not in k:
            continue
        if k not in PROMOTE:
            continue
        top, sub = k.split('.', 1)
        subobj = obj.setdefault(top, {})
        if not isinstance(subobj, dict):
            # Already a non-dict value at this top; skip.
            continue
        subobj[sub] = v
        del obj[k]
        promote(subobj)


for loc in locales:
    p = f'{base}/{loc}/containers.json'
    data = json.load(open(p, encoding='utf-8'))
    bk = data.get('backups')
    if isinstance(bk, dict):
        promote(bk)
        for extra in ('logs',):
            target = bk.get(extra)
            if isinstance(target, dict):
                promote(target)
    out = json.dumps(data, ensure_ascii=False, indent=2) + '\n'
    with open(p, 'wb') as f:
        f.write(out.encode('utf-8'))
    print(f'{loc} updated')
