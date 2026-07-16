import json
import sys
import re

locales = {
    'en': 'src/frontend/core/i18n/locales/en/common.json',
    'nl': 'src/frontend/core/i18n/locales/nl/common.json',
    'de': 'src/frontend/core/i18n/locales/de/common.json',
    'es': 'src/frontend/core/i18n/locales/es/common.json',
    'fr': 'src/frontend/core/i18n/locales/fr/common.json',
    'pt': 'src/frontend/core/i18n/locales/pt/common.json',
}

nav_labels = {
    'en': 'Lifecycle Log',
    'nl': 'Lifecycle Logboek',
    'de': 'Lifecycle-Log',
    'es': 'Registro de ciclo de vida',
    'fr': 'Journal du cycle de vie',
    'pt': 'Registro de ciclo de vida',
}

lifecycle_blocks = {
    'en': '''  "lifecycle": {
    "title": "Lifecycle Log",
    "description": "Docker, image, volume, network, and stack lifecycle events across all environments.",
    "searchPlaceholder": "Search by container, image, or volume name",
    "subjectTypeLabel": "Subject type",
    "severityLabel": "Severity",
    "subjectType": {
      "all": "All subjects",
      "container": "Containers",
      "image": "Images",
      "volume": "Volumes",
      "network": "Networks",
      "stack": "Stacks"
    },
    "severity": {
      "all": "Any severity",
      "success": "Success",
      "info": "Info",
      "warning": "Warning",
      "error": "Error"
    },
    "state": {
      "running": "running",
      "stopped": "stopped",
      "paused": "paused",
      "created": "created",
      "restarting": "restarting",
      "removed": "removed",
      "exited": "exited",
      "available": "available",
      "in_use": "in use",
      "dangling": "dangling",
      "tagged": "tagged",
      "active": "active",
      "up": "up",
      "partial": "partially up",
      "errored": "errored"
    },
    "action": {
      "start": "started",
      "stop": "stopped",
      "die": "died",
      "kill": "killed",
      "pause": "paused",
      "unpause": "resumed",
      "restart": "restarted",
      "create": "created",
      "destroy": "removed",
      "pull": "pulled",
      "load": "loaded",
      "import": "imported",
      "tag": "tagged",
      "untag": "untagged",
      "delete": "deleted",
      "mount": "mounted",
      "unmount": "unmounted",
      "connect": "connected",
      "disconnect": "disconnected",
      "oom": "killed (OOM)",
      "failed": "failed",
      "error": "errored"
    },
    "headerSubject": "Subject",
    "headerDetail": "Detail",
    "headerSeverity": "Severity",
    "headerTime": "Time",
    "expand": "Show details",
    "collapse": "Hide details",
    "empty": "No lifecycle events match these filters.",
    "total": "{{total}} events",
    "pageOf": "Page {{page}} of {{totalPages}}",
    "source": "Source: {{source}}",
    "detailId": "Event id",
    "detailSubject": "Subject",
    "detailEvent": "Event / action",
    "detailState": "State",
    "detailAttributes": "Attributes"
  }''',
}

for loc, path in locales.items():
    with open(path, 'r', encoding='utf-8') as f:
        raw = f.read()
    nav_line = f'    "logging": "{nav_labels[loc]}",\n'

    # 1. Insert nav line in `nav` block, just before "logs"
    if '"logging":' not in raw:
        raw = raw.replace('    "logs":\n', nav_line + '    "logs":\n', 1)

    # 2. Insert the lifecycle block as a sibling of "logs" (a new top-level key in common.json)
    if '"lifecycle":' not in raw:
        # Find the logs block's opening and close, then insert after
        # We do this by splitting on the "logs" key line and inserting just before
        # the line before it.
        # Simpler: insert right before the "  \"logs\": {" line if it exists.
        if '  "logs": {' in raw:
            # Find the start of the logs block and back up one level
            # We want to insert "lifecycle": {...} as a sibling at the same indentation.
            # The indentation of the block is 2 spaces.
            block = lifecycle_blocks['en'].rstrip()
            # Make sure the block ends with a comma so the next sibling "logs" is valid JSON
            if not block.endswith(','):
                block = block + ','
            raw = raw.replace('  "logs": {', block + '\n\n  "logs": {', 1)
        else:
            print(f'{loc}: could not find "logs" block to insert into')

    # Validate JSON
    try:
        json.loads(raw)
    except json.JSONDecodeError as e:
        print(f'{loc}: INVALID JSON: {e}')
        continue

    with open(path, 'w', encoding='utf-8') as f:
        f.write(raw)
    print(f'{loc}: ok')
