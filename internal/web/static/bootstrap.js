import { dashboardText } from './catalog.js';

const code = new URLSearchParams(location.hash.slice(1)).get('code');
if (location.hash) history.replaceState(null, '', location.pathname + location.search);

async function hasValidSession() {
  try {
    const response = await fetch('/api/projects', { credentials: 'same-origin' });
    return response.ok;
  } catch {
    return false;
  }
}

async function bootstrap() {
  if (!code) return;
  try {
    const response = await fetch('/api/session', {
      method: 'POST',
      credentials: 'same-origin',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ code })
    });
    if (!response.ok) throw new Error('session unavailable');
  } catch (error) {
    if (await hasValidSession()) return;
    const note = document.createElement('p');
    note.className = 'bootstrap-error';
    note.setAttribute('role', 'alert');
    note.textContent = dashboardText('bootstrap.sessionError');
    document.body.append(note);
    throw error;
  }
}

window.gasaSessionReady = bootstrap();
