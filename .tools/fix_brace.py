import re
import json

paths = [
    ('nl', 'src/frontend/core/i18n/locales/nl/common.json'),
    ('de', 'src/frontend/core/i18n/locales/de/common.json'),
    ('es', 'src/frontend/core/i18n/locales/es/common.json'),
    ('fr', 'src/frontend/core/i18n/locales/fr/common.json'),
    ('pt', 'src/frontend/core/i18n/locales/pt/common.json'),
]

for loc, path in paths:
    with open(path, 'rb') as f:
        text = f.read().decode('utf-8')
    # The lifecycle block was inserted with 2-space indent for
    # the closing brace (since it's a sibling of "logs" which
    # is at the top level of the file with 2-space indent). The
    # next sibling "title" is also at 2-space indent. JSON requires
    # a comma between them.
    new_text, count = re.subn(
        r'(^  \})\r?\n(    "title":)',
        r'\1,\r?\n\2',
        text,
        count=1,
        flags=re.MULTILINE,
    )
    if count > 0 and new_text != text:
        with open(path, 'wb') as f:
            f.write(new_text.encode('utf-8'))
        print(f'{loc}: fixed ({count})')
    try:
        json.loads(new_text)
        print(f'{loc}: ok')
    except json.JSONDecodeError as e:
        print(f'{loc}: STILL INVALID: {e}')
