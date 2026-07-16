import json
import sys

translations = {
    'en': {
        'title': 'Lifecycle Log',
        'description': 'Docker, image, volume, network, and stack lifecycle events across all environments.',
        'searchPlaceholder': 'Search by container, image, or volume name',
        'subjectTypeLabel': 'Subject type', 'severityLabel': 'Severity',
        'subjectType.all': 'All subjects', 'subjectType.container': 'Containers',
        'subjectType.image': 'Images', 'subjectType.volume': 'Volumes',
        'subjectType.network': 'Networks', 'subjectType.stack': 'Stacks',
        'severity.all': 'Any severity', 'severity.success': 'Success',
        'severity.info': 'Info', 'severity.warning': 'Warning', 'severity.error': 'Error',
        'state.running': 'running', 'state.stopped': 'stopped', 'state.paused': 'paused',
        'state.created': 'created', 'state.restarting': 'restarting',
        'state.removed': 'removed', 'state.exited': 'exited',
        'state.available': 'available', 'state.in_use': 'in use',
        'state.dangling': 'dangling', 'state.tagged': 'tagged',
        'state.active': 'active', 'state.up': 'up',
        'state.partial': 'partially up', 'state.errored': 'errored',
        'action.start': 'started', 'action.stop': 'stopped', 'action.die': 'died',
        'action.kill': 'killed', 'action.pause': 'paused', 'action.unpause': 'resumed',
        'action.restart': 'restarted', 'action.create': 'created',
        'action.destroy': 'removed', 'action.pull': 'pulled',
        'action.load': 'loaded', 'action.import': 'imported',
        'action.tag': 'tagged', 'action.untag': 'untagged', 'action.delete': 'deleted',
        'action.mount': 'mounted', 'action.unmount': 'unmounted',
        'action.connect': 'connected', 'action.disconnect': 'disconnected',
        'action.oom': 'killed (OOM)', 'action.failed': 'failed', 'action.error': 'errored',
        'headerSubject': 'Subject', 'headerDetail': 'Detail', 'headerSeverity': 'Severity',
        'headerTime': 'Time', 'expand': 'Show details', 'collapse': 'Hide details',
        'empty': 'No lifecycle events match these filters.',
        'total': '{{total}} events', 'pageOf': 'Page {{page}} of {{totalPages}}',
        'source': 'Source: {{source}}', 'detailId': 'Event id', 'detailSubject': 'Subject',
        'detailEvent': 'Event / action', 'detailState': 'State',
        'detailAttributes': 'Attributes',
    },
    'nl': {
        'title': 'Lifecycle Logboek', 'description': 'Docker-, image-, volume-, netwerk- en stack-levenscyclusgebeurtenissen voor alle omgevingen.',
        'searchPlaceholder': 'Zoek op container-, image- of volumenaam', 'subjectTypeLabel': 'Type onderwerp', 'severityLabel': 'Ernst',
        'subjectType.all': 'Alle onderwerpen', 'subjectType.container': 'Containers', 'subjectType.image': 'Images',
        'subjectType.volume': 'Volumes', 'subjectType.network': 'Netwerken', 'subjectType.stack': 'Stacks',
        'severity.all': 'Elke ernst', 'severity.success': 'Succes', 'severity.info': 'Info',
        'severity.warning': 'Waarschuwing', 'severity.error': 'Fout',
        'state.running': 'draaiend', 'state.stopped': 'gestopt', 'state.paused': 'gepauzeerd',
        'state.created': 'aangemaakt', 'state.restarting': 'herstartend', 'state.removed': 'verwijderd',
        'state.exited': 'afgesloten', 'state.available': 'beschikbaar', 'state.in_use': 'in gebruik',
        'state.dangling': 'zwevend', 'state.tagged': 'getagd', 'state.active': 'actief',
        'state.up': 'actief', 'state.partial': 'gedeeltelijk actief', 'state.errored': 'mislukt',
        'action.start': 'gestart', 'action.stop': 'gestopt', 'action.die': 'gestorven',
        'action.kill': 'afgesloten', 'action.pause': 'gepauzeerd', 'action.unpause': 'hervat',
        'action.restart': 'herstart', 'action.create': 'aangemaakt', 'action.destroy': 'verwijderd',
        'action.pull': 'opgehaald', 'action.load': 'geladen', 'action.import': 'geimporteerd',
        'action.tag': 'getagd', 'action.untag': 'onttagd', 'action.delete': 'verwijderd',
        'action.mount': 'gekoppeld', 'action.unmount': 'ontkoppeld', 'action.connect': 'verbonden',
        'action.disconnect': 'verbroken', 'action.oom': 'afgesloten (OOM)', 'action.failed': 'mislukt',
        'action.error': 'fout opgetreden',
        'headerSubject': 'Onderwerp', 'headerDetail': 'Detail', 'headerSeverity': 'Ernst',
        'headerTime': 'Tijd', 'expand': 'Details tonen', 'collapse': 'Details verbergen',
        'empty': 'Geen levenscyclusgebeurtenissen voldoen aan deze filters.',
        'total': '{{total}} gebeurtenissen', 'pageOf': 'Pagina {{page}} van {{totalPages}}',
        'source': 'Bron: {{source}}', 'detailId': 'Gebeurtenis-ID', 'detailSubject': 'Onderwerp',
        'detailEvent': 'Gebeurtenis / actie', 'detailState': 'Status', 'detailAttributes': 'Kenmerken',
    },
    'de': {
        'title': 'Lifecycle-Log', 'description': 'Docker-, Image-, Volume-, Netzwerk- und Stack-Lifecycle-Ereignisse ueber alle Umgebungen hinweg.',
        'searchPlaceholder': 'Nach Container-, Image- oder Volume-Name suchen',
        'subjectTypeLabel': 'Subjekttyp', 'severityLabel': 'Schweregrad',
        'subjectType.all': 'Alle Subjekte', 'subjectType.container': 'Container', 'subjectType.image': 'Images',
        'subjectType.volume': 'Volumes', 'subjectType.network': 'Netzwerke', 'subjectType.stack': 'Stacks',
        'severity.all': 'Beliebiger Schweregrad', 'severity.success': 'Erfolg', 'severity.info': 'Info',
        'severity.warning': 'Warnung', 'severity.error': 'Fehler',
        'state.running': 'laeuft', 'state.stopped': 'gestoppt', 'state.paused': 'pausiert',
        'state.created': 'erstellt', 'state.restarting': 'startet neu', 'state.removed': 'entfernt',
        'state.exited': 'beendet', 'state.available': 'verfuegbar', 'state.in_use': 'in Verwendung',
        'state.dangling': 'verwaist', 'state.tagged': 'getaggt', 'state.active': 'aktiv',
        'state.up': 'aktiv', 'state.partial': 'teilweise aktiv', 'state.errored': 'fehlerhaft',
        'action.start': 'gestartet', 'action.stop': 'gestoppt', 'action.die': 'gestorben',
        'action.kill': 'abgebrochen', 'action.pause': 'pausiert', 'action.unpause': 'fortgesetzt',
        'action.restart': 'neu gestartet', 'action.create': 'erstellt', 'action.destroy': 'entfernt',
        'action.pull': 'gepullt', 'action.load': 'geladen', 'action.import': 'importiert',
        'action.tag': 'getaggt', 'action.untag': 'entfernt', 'action.delete': 'geloescht',
        'action.mount': 'eingebunden', 'action.unmount': 'ausgehaengt', 'action.connect': 'verbunden',
        'action.disconnect': 'getrennt', 'action.oom': 'abgebrochen (OOM)', 'action.failed': 'fehlgeschlagen',
        'action.error': 'fehler',
        'headerSubject': 'Subjekt', 'headerDetail': 'Detail', 'headerSeverity': 'Schweregrad',
        'headerTime': 'Zeit', 'expand': 'Details anzeigen', 'collapse': 'Details verbergen',
        'empty': 'Keine Lifecycle-Ereignisse entsprechen diesen Filtern.',
        'total': '{{total}} Ereignisse', 'pageOf': 'Seite {{page}} von {{totalPages}}',
        'source': 'Quelle: {{source}}', 'detailId': 'Ereignis-ID', 'detailSubject': 'Subjekt',
        'detailEvent': 'Ereignis / Aktion', 'detailState': 'Status', 'detailAttributes': 'Attribute',
    },
    'es': {
        'title': 'Registro de ciclo de vida',
        'description': 'Eventos del ciclo de vida de Docker, imagen, volumen, red y stack en todos los entornos.',
        'searchPlaceholder': 'Buscar por nombre de contenedor, imagen o volumen',
        'subjectTypeLabel': 'Tipo de sujeto', 'severityLabel': 'Severidad',
        'subjectType.all': 'Todos los sujetos', 'subjectType.container': 'Contenedores',
        'subjectType.image': 'Imagenes', 'subjectType.volume': 'Volumenes',
        'subjectType.network': 'Redes', 'subjectType.stack': 'Stacks',
        'severity.all': 'Cualquier severidad', 'severity.success': 'Exito', 'severity.info': 'Info',
        'severity.warning': 'Advertencia', 'severity.error': 'Error',
        'state.running': 'ejecutandose', 'state.stopped': 'detenido', 'state.paused': 'pausado',
        'state.created': 'creado', 'state.restarting': 'reiniciando', 'state.removed': 'eliminado',
        'state.exited': 'finalizado', 'state.available': 'disponible', 'state.in_use': 'en uso',
        'state.dangling': 'huerefano', 'state.tagged': 'etiquetado', 'state.active': 'activo',
        'state.up': 'activo', 'state.partial': 'parcialmente activo', 'state.errored': 'con error',
        'action.start': 'iniciado', 'action.stop': 'detenido', 'action.die': 'finalizado',
        'action.kill': 'eliminado', 'action.pause': 'pausado', 'action.unpause': 'reanudado',
        'action.restart': 'reiniciado', 'action.create': 'creado', 'action.destroy': 'eliminado',
        'action.pull': 'descargado', 'action.load': 'cargado', 'action.import': 'importado',
        'action.tag': 'etiquetado', 'action.untag': 'etiquetado', 'action.delete': 'eliminado',
        'action.mount': 'montado', 'action.unmount': 'desmontado', 'action.connect': 'conectado',
        'action.disconnect': 'desconectado', 'action.oom': 'eliminado (OOM)', 'action.failed': 'fallido',
        'action.error': 'error',
        'headerSubject': 'Sujeto', 'headerDetail': 'Detalle', 'headerSeverity': 'Severidad',
        'headerTime': 'Hora', 'expand': 'Mostrar detalles', 'collapse': 'Ocultar detalles',
        'empty': 'No hay eventos que coincidan con los filtros.',
        'total': '{{total}} eventos', 'pageOf': 'Pagina {{page}} de {{totalPages}}',
        'source': 'Origen: {{source}}', 'detailId': 'Id del evento', 'detailSubject': 'Sujeto',
        'detailEvent': 'Evento / accion', 'detailState': 'Estado', 'detailAttributes': 'Atributos',
    },
    'fr': {
        'title': 'Journal du cycle de vie',
        'description': 'Evenements du cycle de vie Docker, images, volumes, reseaux et piles pour tous les environnements.',
        'searchPlaceholder': 'Rechercher par nom de conteneur, image ou volume',
        'subjectTypeLabel': 'Type de sujet', 'severityLabel': 'Severite',
        'subjectType.all': 'Tous les sujets', 'subjectType.container': 'Conteneurs',
        'subjectType.image': 'Images', 'subjectType.volume': 'Volumes',
        'subjectType.network': 'Reseaux', 'subjectType.stack': 'Piles',
        'severity.all': 'Toute severite', 'severity.success': 'Succes', 'severity.info': 'Info',
        'severity.warning': 'Avertissement', 'severity.error': 'Erreur',
        'state.running': 'en cours', 'state.stopped': 'arrete', 'state.paused': 'en pause',
        'state.created': 'cree', 'state.restarting': 'en redemarrage', 'state.removed': 'supprime',
        'state.exited': 'termine', 'state.available': 'disponible', 'state.in_use': 'en service',
        'state.dangling': 'orphelin', 'state.tagged': 'tague', 'state.active': 'actif',
        'state.up': 'actif', 'state.partial': 'partiellement actif', 'state.errored': 'en erreur',
        'action.start': 'demarre', 'action.stop': 'arrete', 'action.die': 'termine',
        'action.kill': 'elimine', 'action.pause': 'en pause', 'action.unpause': 'repris',
        'action.restart': 'redemarre', 'action.create': 'cree', 'action.destroy': 'supprime',
        'action.pull': 'tire', 'action.load': 'charge', 'action.import': 'importe',
        'action.tag': 'tague', 'action.untag': 'detague', 'action.delete': 'supprime',
        'action.mount': 'monte', 'action.unmount': 'demonte', 'action.connect': 'connecte',
        'action.disconnect': 'deconnecte', 'action.oom': 'elimine (OOM)', 'action.failed': 'echoue',
        'action.error': 'erreur',
        'headerSubject': 'Sujet', 'headerDetail': 'Detail', 'headerSeverity': 'Severite',
        'headerTime': 'Heure', 'expand': 'Afficher les details', 'collapse': 'Masquer les details',
        'empty': 'Aucun evenement ne correspond a ces filtres.',
        'total': '{{total}} evenements', 'pageOf': 'Page {{page}} sur {{totalPages}}',
        'source': 'Source : {{source}}', 'detailId': 'Id de l evenement', 'detailSubject': 'Sujet',
        'detailEvent': 'Evenement / action', 'detailState': 'Etat', 'detailAttributes': 'Attributs',
    },
    'pt': {
        'title': 'Registro de ciclo de vida',
        'description': 'Eventos do ciclo de vida do Docker, imagem, volume, rede e stack em todos os ambientes.',
        'searchPlaceholder': 'Pesquisar por nome do container, imagem ou volume',
        'subjectTypeLabel': 'Tipo de assunto', 'severityLabel': 'Severidade',
        'subjectType.all': 'Todos os assuntos', 'subjectType.container': 'Containers',
        'subjectType.image': 'Imagens', 'subjectType.volume': 'Volumes',
        'subjectType.network': 'Redes', 'subjectType.stack': 'Stacks',
        'severity.all': 'Qualquer severidade', 'severity.success': 'Sucesso', 'severity.info': 'Info',
        'severity.warning': 'Aviso', 'severity.error': 'Erro',
        'state.running': 'em execucao', 'state.stopped': 'parado', 'state.paused': 'pausado',
        'state.created': 'criado', 'state.restarting': 'reiniciando', 'state.removed': 'removido',
        'state.exited': 'encerrado', 'state.available': 'disponivel', 'state.in_use': 'em uso',
        'state.dangling': 'orfão', 'state.tagged': 'marcado', 'state.active': 'ativo',
        'state.up': 'ativo', 'state.partial': 'parcialmente ativo', 'state.errored': 'com erro',
        'action.start': 'iniciado', 'action.stop': 'parado', 'action.die': 'encerrado',
        'action.kill': 'encerrado', 'action.pause': 'pausado', 'action.unpause': 'retomado',
        'action.restart': 'reiniciado', 'action.create': 'criado', 'action.destroy': 'removido',
        'action.pull': 'puxado', 'action.load': 'carregado', 'action.import': 'importado',
        'action.tag': 'marcado', 'action.untag': 'desmarcado', 'action.delete': 'excluido',
        'action.mount': 'montado', 'action.unmount': 'desmontado', 'action.connect': 'conectado',
        'action.disconnect': 'desconectado', 'action.oom': 'encerrado (OOM)', 'action.failed': 'falhou',
        'action.error': 'erro',
        'headerSubject': 'Assunto', 'headerDetail': 'Detalhe', 'headerSeverity': 'Severidade',
        'headerTime': 'Hora', 'expand': 'Mostrar detalhes', 'collapse': 'Ocultar detalhes',
        'empty': 'Nenhum evento corresponde a esses filtros.',
        'total': '{{total}} eventos', 'pageOf': 'Pagina {{page}} de {{totalPages}}',
        'source': 'Origem: {{source}}', 'detailId': 'Id do evento', 'detailSubject': 'Assunto',
        'detailEvent': 'Evento / acao', 'detailState': 'Estado', 'detailAttributes': 'Atributos',
    },
}

paths = {
    'en': 'src/frontend/core/i18n/locales/en/common.json',
    'nl': 'src/frontend/core/i18n/locales/nl/common.json',
    'de': 'src/frontend/core/i18n/locales/de/common.json',
    'es': 'src/frontend/core/i18n/locales/es/common.json',
    'fr': 'src/frontend/core/i18n/locales/fr/common.json',
    'pt': 'src/frontend/core/i18n/locales/pt/common.json',
}

for loc, path in paths.items():
    with open(path, 'r', encoding='utf-8') as f:
        data = json.load(f)
    t = translations[loc]
    # Build the lifecycle block
    lifecycle_block = {
        'title': t['title'], 'description': t['description'],
        'searchPlaceholder': t['searchPlaceholder'], 'subjectTypeLabel': t['subjectTypeLabel'],
        'severityLabel': t['severityLabel'],
        'subjectType': {k.split('.', 1)[1]: v for k, v in t.items() if k.startswith('subjectType.')},
        'severity': {k.split('.', 1)[1]: v for k, v in t.items() if k.startswith('severity.')},
        'state': {k.split('.', 1)[1]: v for k, v in t.items() if k.startswith('state.')},
        'action': {k.split('.', 1)[1]: v for k, v in t.items() if k.startswith('action.')},
        'headerSubject': t['headerSubject'], 'headerDetail': t['headerDetail'],
        'headerSeverity': t['headerSeverity'], 'headerTime': t['headerTime'],
        'expand': t['expand'], 'collapse': t['collapse'], 'empty': t['empty'],
        'total': t['total'], 'pageOf': t['pageOf'], 'source': t['source'],
        'detailId': t['detailId'], 'detailSubject': t['detailSubject'],
        'detailEvent': t['detailEvent'], 'detailState': t['detailState'],
        'detailAttributes': t['detailAttributes'],
    }
    # Insert lifecycle as a sibling of "logs" (insert before "logs" in top-level)
    if 'lifecycle' not in data:
        new_data = {}
        inserted = False
        for key, val in data.items():
            if key == 'logs' and not inserted:
                new_data['lifecycle'] = lifecycle_block
                inserted = True
            new_data[key] = val
        if not inserted:
            new_data['lifecycle'] = lifecycle_block
        data = new_data
    # Add nav.logging
    if 'logging' not in data.get('nav', {}):
        nav = data['nav']
        new_nav = {}
        inserted = False
        for key, val in nav.items():
            if key == 'logs' and not inserted:
                new_nav['logging'] = t['title']
                inserted = True
            new_nav[key] = val
        if not inserted:
            new_nav['logging'] = t['title']
        data['nav'] = new_nav
    with open(path, 'w', encoding='utf-8') as f:
        json.dump(data, f, ensure_ascii=False, indent=2)
        f.write('\n')
    print(f'{loc}: ok')
