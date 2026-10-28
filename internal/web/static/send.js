const byId = id => document.getElementById(id);
let activeSend;

function supported(session) {
  const capabilities = window.gasaCapabilities || { send: false, agents: [] };
  return Boolean(capabilities.send && capabilities.agents?.includes(session?.agent));
}

function localizeSendError(message) {
  const value = String(message || '').toLowerCase();
  if (value.includes('cli is not installed')) return 'A CLI deste agente não está instalada.';
  if (value.includes('denied') || value.includes('permission')) return 'Envio negado pela política de permissões.';
  if (value.includes('timed out') || value.includes('timeout')) return 'O envio excedeu o tempo limite.';
  if (value.includes('signed in') || value.includes('login')) return 'A CLI deste agente não está autenticada.';
  if (value.includes('unknown or expired') || value.includes('session not found')) return 'Esta sessão não existe mais ou expirou.';
  if (value.includes('cancelled')) return 'Envio cancelado.';
  return message || 'Não foi possível enviar a mensagem.';
}

function updateComposer() {
  const session = window.gasaSendSession;
  const enabled = supported(session);
  const button = byId('send-message');
  button.disabled = !enabled || Boolean(activeSend);
  button.title = enabled ? 'Enviar mensagem para esta conversa' : 'Este agente não aceita envio';
  byId('send-capability').textContent = enabled ? 'Envio com aprovação' : 'Somente leitura';
  byId('send-text').placeholder = enabled ? 'Escreva uma mensagem para esta conversa' : 'Envio indisponível para este agente';
}

function approvalDialog(session, project, message) {
  return new Promise(resolve => {
    const dialog = document.createElement('dialog');
    dialog.className = 'send-approval';
    const title = document.createElement('h2');
    title.textContent = 'Aprovar envio';
    const details = document.createElement('dl');
    if (session.status === 'working') {
      const warning = document.createElement('p'); warning.className = 'send-error';
      warning.textContent = 'Esta sessão teve atividade recente e pode estar aberta em outro terminal. Continuar pode misturar mensagens.';
      dialog.append(title, warning);
    } else dialog.append(title);
    const access = document.createElement('p'); access.className = 'setting-note';
    access.textContent = 'Este envio inicia a CLI do agente e permite que ela acesse o provedor configurado.';
    dialog.append(access);
    for (const [label, value] of [['Agente', session.agent], ['Sessão', session.title || session.id], ['Projeto', project?.name || 'Projeto confiável'], ['Mensagem exata', message]]) {
      const term = document.createElement('dt'); term.textContent = label;
      const description = document.createElement('dd'); description.textContent = value;
      details.append(term, description);
    }
    const actions = document.createElement('div'); actions.className = 'send-approval-actions';
    const deny = document.createElement('button'); deny.type = 'button'; deny.className = 'secondary-button'; deny.textContent = 'Cancelar';
    const allow = document.createElement('button'); allow.type = 'button'; allow.className = 'primary-button'; allow.textContent = 'Aprovar e enviar';
    const finish = accepted => { dialog.close(); dialog.remove(); resolve(accepted); };
    deny.addEventListener('click', () => finish(false));
    allow.addEventListener('click', () => finish(true));
    dialog.addEventListener('cancel', event => { event.preventDefault(); finish(false); }, { once: true });
    actions.append(deny, allow); dialog.append(details, actions); document.body.append(dialog); dialog.showModal();
  });
}

async function sendMessage() {
  const session = window.gasaSendSession;
  const text = byId('send-text').value;
  if (!session || !supported(session) || !text.trim() || activeSend) return;
  const project = (window.gasaProjects || []).find(item => item.id === session.projectId);
  if (!await approvalDialog(session, project, text)) return;
  const chat = byId('chat');
  const userBubble = document.createElement('article'); userBubble.className = 'message user sending';
  userBubble.textContent = text;
  const state = document.createElement('span'); state.className = 'send-state'; state.textContent = 'enviando';
  userBubble.append(state); chat.append(userBubble); chat.scrollTop = chat.scrollHeight;
  const assistant = document.createElement('article'); assistant.className = 'message assistant';
  const role = document.createElement('div'); role.className = 'message-role'; role.textContent = session.agent;
  const content = document.createElement('p'); content.className = 'markdown-line';
  assistant.append(role, content);
  const controller = new AbortController(); activeSend = controller; updateComposer();
  byId('send-error').textContent = '';
  byId('send-text').disabled = true; byId('cancel-send').classList.remove('hidden');
  try {
    const response = await fetch(`/api/sessions/${encodeURIComponent(session.id)}/send`, {
      method: 'POST', credentials: 'same-origin', signal: controller.signal,
      headers: { 'Content-Type': 'application/json', Accept: 'text/event-stream, application/json' },
      body: JSON.stringify({ text, approved: true })
    });
    if (!response.ok) {
      const detail = (await response.text()).trim();
      throw new Error(localizeSendError(detail || (response.status === 403 ? 'Envio negado pela política de permissões.' : 'Não foi possível enviar a mensagem.')));
    }
    const reader = response.body.getReader(); const decoder = new TextDecoder(); let buffer = '';
    while (true) {
      const { value, done } = await reader.read(); if (done) break;
      buffer += decoder.decode(value, { stream: true });
      const chunks = buffer.split('\n\n'); buffer = chunks.pop() || '';
      for (const chunk of chunks) {
        const line = chunk.split('\n').find(row => row.startsWith('data: ')); if (!line) continue;
        const event = JSON.parse(line.slice(6));
        if (event.type === 'text') { if (!assistant.isConnected) chat.append(assistant); content.textContent += event.text || ''; chat.scrollTop = chat.scrollHeight; }
        if (event.type === 'error') throw new Error(localizeSendError(event.error));
        if (event.type === 'done') state.textContent = 'enviado';
      }
    }
    if (!content.textContent) assistant.remove();
    else chat.append(assistant);
    state.textContent = 'enviado'; userBubble.classList.remove('sending'); byId('send-text').value = '';
  } catch (error) {
    assistant.remove();
    state.textContent = controller.signal.aborted ? 'cancelado' : 'falhou';
    byId('send-error').textContent = controller.signal.aborted ? 'Envio cancelado.' : localizeSendError(error.message);
  } finally {
    activeSend = null; byId('send-text').disabled = false; byId('cancel-send').classList.add('hidden'); updateComposer();
  }
}

byId('send-message').addEventListener('click', sendMessage);
byId('send-text').addEventListener('keydown', event => {
  if (event.key === 'Enter' && !event.shiftKey) { event.preventDefault(); sendMessage(); }
});
byId('cancel-send').addEventListener('click', () => activeSend?.abort());
window.addEventListener('gasacapabilities', updateComposer);
window.addEventListener('gasasession', updateComposer);
window.addEventListener('gasaprojects', updateComposer);
setInterval(updateComposer, 500);
