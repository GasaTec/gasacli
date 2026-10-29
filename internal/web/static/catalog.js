const catalogs = {
  'pt-BR': {
    'bootstrap.sessionError': 'Não foi possível iniciar a sessão do dashboard. Abra o dashboard novamente pelo GasaCLI.',
    'agents.label': 'Agentes prontos',
    'agents.unknown': 'Indisponível',
    'agents.unknownHint': 'Estado de disponibilidade não coletado',
    'agents.checking': 'Verificando...',
    'agents.checkingHint': 'Verificando disponibilidade local',
    'agents.of': 'de',
    'agents.tooltipHint': 'Passe o cursor para ver o estado de cada agente',
    'agents.ready': 'pronto',
    'agents.needsLogin': 'login necessário',
    'agents.installed': 'instalado',
    'agents.missing': 'ausente',
    'agents.unknownStatus': 'desconhecido'
  },
  en: {
    'bootstrap.sessionError': 'Could not start the dashboard session. Open the dashboard again from GasaCLI.',
    'agents.label': 'Ready agents',
    'agents.unknown': 'Unavailable',
    'agents.unknownHint': 'Availability state was not collected',
    'agents.checking': 'Checking...',
    'agents.checkingHint': 'Checking local availability',
    'agents.of': 'of',
    'agents.tooltipHint': 'Hover to see each agent status',
    'agents.ready': 'ready',
    'agents.needsLogin': 'login required',
    'agents.installed': 'installed',
    'agents.missing': 'missing',
    'agents.unknownStatus': 'unknown'
  }
};

export function dashboardText(key) {
  let language = 'pt-BR';
  try {
    language = localStorage.getItem('gasa-language') === 'en' ? 'en' : 'pt-BR';
  } catch {}
  return catalogs[language][key] || catalogs['pt-BR'][key] || key;
}
