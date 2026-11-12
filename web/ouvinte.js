'use strict';

// Ouvinte: página sem tela que o próprio programa abre, escondida, para transcrever os
// áudios que os clientes mandam no WhatsApp. Usa o mesmo Whisper da voz do painel
// (voz-worker.js). Só conversa com este computador: pega o som em /api/ouvido/proximo e
// devolve o texto em /api/ouvido/resultado. Ver ouvido.go.
(async () => {
  const esperar = (ms) => new Promise((r) => setTimeout(r, ms));
  const pendentes = new Map();
  let worker = null;
  let seq = 0;

  function ligar(arquivos) {
    const novo = new Worker('/voz-worker.js');
    novo.onmessage = (ev) => {
      const m = ev.data || {};
      if (m.id == null) return; // avisos de carga: o resultado de cada trecho já diz se deu certo
      const p = pendentes.get(m.id);
      if (!p) return;
      pendentes.delete(m.id);
      if (m.tipo === 'texto') p.ok(String(m.texto || ''));
      else p.falha(new Error(m.erro || 'falha'));
    };
    novo.onerror = () => {
      for (const p of pendentes.values()) p.falha(new Error('o Whisper parou'));
      pendentes.clear();
      worker = null;
    };
    novo.postMessage({ tipo: 'carregar', arquivos });
    worker = novo;
  }

  function transcrever(audio) {
    return new Promise((ok, falha) => {
      const id = ++seq;
      pendentes.set(id, { ok, falha });
      worker.postMessage({ tipo: 'transcrever', id, audio }, [audio.buffer]);
    });
  }

  let falhas = 0;
  for (;;) {
    try {
      if (!worker) {
        const r = await fetch('/api/ouvido/inicio', { cache: 'no-store' });
        if (!r.ok) throw new Error('inicio ' + r.status);
        ligar((await r.json()).arquivos);
      }
      const r = await fetch('/api/ouvido/proximo', { cache: 'no-store' });
      if (r.status === 410) { window.close(); return; } // o programa mandou fechar
      if (r.status === 204) { falhas = 0; continue; }
      if (!r.ok) throw new Error('proximo ' + r.status);
      const id = r.headers.get('X-Trecho') || '';
      const audio = new Float32Array(await r.arrayBuffer());
      let corpo;
      try {
        corpo = { texto: await transcrever(audio) };
      } catch (e) {
        corpo = { erro: String((e && e.message) || e) };
      }
      await fetch('/api/ouvido/resultado?id=' + encodeURIComponent(id), {
        method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(corpo),
      });
      falhas = 0;
    } catch (e) {
      // O programa saiu do ar: esta página não fica aberta à toa.
      if (++falhas > 20) { window.close(); return; }
      await esperar(1500);
    }
  }
})();
