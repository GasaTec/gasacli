const code = new URLSearchParams(location.hash.slice(1)).get('code');
if (location.hash) history.replaceState(null, '', location.pathname + location.search);
if (code) fetch('/api/session', {method:'POST',credentials:'same-origin',headers:{'Content-Type':'application/json'},body:JSON.stringify({code})}).then(response=>{if(!response.ok)throw new Error('session unavailable')}).catch(()=>{const note=document.createElement('p');note.textContent='Could not start the dashboard session. Open the dashboard again from GasaCLI.';document.body.append(note)});
