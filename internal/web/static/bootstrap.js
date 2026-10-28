const code = new URLSearchParams(location.hash.slice(1)).get('code');
if (location.hash) history.replaceState(null, '', location.pathname + location.search);
window.gasaSessionReady = code
  ? fetch('/api/session', {method:'POST',credentials:'same-origin',headers:{'Content-Type':'application/json'},body:JSON.stringify({code})}).then(response=>{if(!response.ok)throw new Error('session unavailable')}).catch(error=>{const note=document.createElement('p');note.className='bootstrap-error';note.setAttribute('role','alert');note.textContent='Could not start the dashboard session. Open the dashboard again from GasaCLI.';document.body.append(note);throw error})
  : Promise.resolve();
