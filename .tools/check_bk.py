import json
for loc in ['en', 'nl', 'de', 'es', 'fr', 'pt']:
    d = json.load(open(f'src/frontend/core/i18n/locales/{loc}/containers.json', encoding='utf-8'))
    bk = d.get('backups', {})
    relink = bk.get('relinkAll')
    save = bk.get('saveChanges')
    runs_table = bk.get('runs', {}).get('table', {})
    plans_table = bk.get('plans', {}).get('table', {})
    print(f'{loc}:')
    print(f'  relinkAll: {relink!r}')
    print(f'  saveChanges: {save!r}')
    print(f'  runs.table.container: {runs_table.get("container")!r}')
    print(f'  plans.table.container: {plans_table.get("container")!r}')
