const { useState, useEffect } = React;

function App(){
  const [keys, setKeys] = useState([]);
  const [loading, setLoading] = useState(false);
  const [selected, setSelected] = useState(null);
  const [error, setError] = useState(null);
  const [prefix, setPrefix] = useState("");
  const [apiKey, setApiKey] = useState( localStorage.getItem('progressdb_admin_key') || '' );
  const [tmpKey, setTmpKey] = useState('');

  useEffect(()=>{ fetchKeys();
    // restore last selected key if present
    const last = localStorage.getItem('progressdb_selected_key');
    if(last){
      // attempt to open after a small delay to allow keys to load
      setTimeout(()=> openKey(last), 200);
    }
  }, []);

  function authHeaders(){
    const h = { 'X-Role-Name': 'admin' };
    if(apiKey && apiKey !== ''){
      h['Authorization'] = `Bearer ${apiKey}`;
    }
    return h;
  }

  async function fetchKeys(pfx){
    setLoading(true); setError(null);
    try{
      const qs = pfx ? `?prefix=${encodeURIComponent(pfx)}` : "";
      const res = await fetch(`/admin/keys${qs}`, { headers: authHeaders() });
      if(!res.ok) throw new Error(await res.text());
      const body = await res.json();
      setKeys(body.keys || []);
    }catch(e){ setError(String(e)); }
    setLoading(false);
  }

  async function openKey(key){
    setSelected({ key, loading: true });
    try{
      const res = await fetch(`/admin/keys/${encodeURIComponent(key)}`, { headers: authHeaders() });
      if(!res.ok) throw new Error(await res.text());
      const text = await res.text();
      setSelected({ key, value: text });
      // persist last selected
      try{ localStorage.setItem('progressdb_selected_key', key) }catch(e){}
    }catch(e){ setSelected({ key, error: String(e) }); }
  }

  // helper: pretty-print JSON with simple syntax highlighting
  function syntaxHighlightJson(text){
    try{
      const obj = JSON.parse(text);
      const pretty = JSON.stringify(obj, null, 2);
      return pretty
        .replace(/(&|<|>)/g, function(m){return {'&':'&amp;','<':'&lt;','>':'&gt;'}[m];})
        .replace(/\"(.*?)\":/g, '"<span class="text-gray-800 font-semibold">$1</span>":')
        .replace(/: \"(.*?)\"/g, ': "<span class="text-green-700">$1</span>"')
        .replace(/: (\\d+(?:\\.\\d+)?)/g, ': <span class="text-indigo-700">$1</span>')
        .replace(/: (true|false)/g, ': <span class="text-purple-700">$1</span>')
        .replace(/: null/g, ': <span class="text-gray-500">null</span>');
    }catch(e){
      return ''; // not JSON
    }
  }

  // Use Tailwind classes for static height and scrollable sidebar/content
  // h-[700px] for static height, flex-1, min-h-0, overflow-auto for scrollable areas

  return (
    <div className="max-w-screen-xl mx-auto p-5 min-h-screen max-h-screen h-screen flex flex-col">
      <div className="bg-white rounded-lg shadow p-4 flex items-center gap-4 mb-4">
        <div className="text-blue-500 font-semibold text-lg">ProgressDB Viewer</div>

        <div className="flex items-center gap-2">
          <input className="border border-gray-300 rounded px-3 py-2 w-56" type="search" placeholder="Filter by prefix..." value={prefix} onChange={e=>setPrefix(e.target.value)} />
          <button className="bg-blue-500 text-white px-3 py-2 rounded shadow-sm hover:bg-blue-400 transition border border-gray-300" onClick={()=>fetchKeys(prefix)}>Filter</button>
          <button className="border border-gray-300 px-3 py-2 rounded text-gray-600 hover:bg-gray-100 transition" onClick={()=>{ setPrefix(''); fetchKeys(''); }}>Clear</button>
        </div>

        <div className="ml-auto bg-white p-2 rounded flex items-center gap-2">
          <input
            className="border border-gray-300 rounded px-3 py-2 w-64"
            type="text"
            placeholder={apiKey ? `Currently set: ${apiKey.slice(0, 4)}...${apiKey.slice(-4)}` : "Enter admin key..."}
            value={tmpKey}
            onChange={e => setTmpKey(e.target.value)}
          />
          <button className="bg-blue-500 text-white px-3 py-2 rounded shadow-sm hover:bg-blue-400 transition border border-gray-300" onClick={()=>{ setApiKey(tmpKey); localStorage.setItem('progressdb_admin_key', tmpKey); setTmpKey(''); }}>Save</button>
          <button className="border border-gray-300 px-3 py-2 rounded text-gray-600 hover:bg-gray-100 transition" onClick={()=>{ setApiKey(''); localStorage.removeItem('progressdb_admin_key'); setTmpKey(''); }}>Clear</button>
        </div>
      </div>

      <div className="flex gap-4 items-stretch flex-1 min-h-0">
        <div className="w-80 flex flex-col gap-3 min-h-0 h-full">
          <div className="bg-white p-3 rounded shadow flex justify-between items-center flex-shrink-0">
            <div>
              <div className="font-medium">Keys</div>
              <div className="text-sm text-gray-400">{keys.length} items</div>
            </div>
            <div>
              <button className="border border-gray-300 px-3 py-1 rounded text-gray-600 hover:bg-gray-100 transition" onClick={()=>fetchKeys(prefix)}>Refresh</button>
            </div>
          </div>

          <div className="bg-white p-3 rounded shadow flex-1 min-h-0 overflow-auto">
            {loading && <div className="text-sm text-gray-400">Loading keys…</div>}
            {error && <div className="text-sm text-red-500">Error: {error}</div>}
            <ul className="mt-2" style={{listStyle:'none', padding:0}}>
              {keys.map(k=> (
                <li key={k} className={(selected && selected.key===k ? 'flex items-center gap-2 p-2 rounded selected cursor-pointer' : 'flex items-center gap-2 p-2 rounded hover:bg-gray-50 transition')}>
                  <button className="text-left w-full text-gray-700 hover:text-blue-600 transition text-xs" onClick={()=>openKey(k)}>{k}</button>
                </li>
              ))}
            </ul>
          </div>
        </div>

        <div className="bg-white p-4 rounded shadow flex-1 min-h-0 flex flex-col h-full">
          <div className="flex items-center gap-2 mb-3 flex-shrink-0">
            <div className="text-sm text-gray-400">{selected ? selected.key : 'No key selected'}</div>
            <div className="ml-auto">
              <button
                className="border border-gray-300 px-3 py-1 rounded text-gray-600 hover:bg-gray-100 transition"
                onClick={()=>{
                  if(selected && selected.value){
                    navigator.clipboard && navigator.clipboard.writeText(selected.value)
                  }
                }}
                disabled={!selected || !selected.value}
                style={!selected || !selected.value ? {opacity:0.5, cursor:'not-allowed'} : {}}
              >
                Copy
              </button>
            </div>
          </div>

          <div className="flex-1 min-h-0 overflow-auto">
            {!selected && <div className="text-sm text-gray-400">Select a key to view its content</div>}
            {selected && selected.loading && <div className="text-sm text-gray-400">Loading <span className="font-medium">{selected.key}</span>…</div>}
            {selected && selected.error && <div className="text-sm text-red-500">Error: {selected.error}</div>}
            {selected && selected.value && (
              <div>
                <h3 className="text-base font-medium mb-2 text-gray-700">{selected.key}</h3>
                <div className="bg-gray-50 p-4 rounded overflow-auto border border-gray-300 max-h-[400px]">
                  {(() => {
                    const highlighted = syntaxHighlightJson(selected.value);
                    if(highlighted && highlighted.length>0){
                      return <pre className="text-sm font-mono text-gray-900" style={{background:'none', border:'none', margin:0}} dangerouslySetInnerHTML={{__html: highlighted}} />
                    }
                    return <pre className="text-sm font-mono text-gray-900" style={{background:'none', border:'none', margin:0}}>{selected.value}</pre>
                  })()}
                </div>
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}

ReactDOM.render(<App/>, document.getElementById('root'));
