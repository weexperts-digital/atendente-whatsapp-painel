'use strict';
// Aba Conhecimento: materiais do negócio que o assistente consulta para responder.

(function () {
  let tela = null;
  const itens = new Map(); // id -> { el, marca }
  let fila = [];           // arquivos em envio: { nome, estado, erro }
  let enviando = false;

  async function montar(main) {
    const listaEl = h('ul', { class: 'materiais' });
    const filaEl = h('ul', { class: 'fila', 'aria-live': 'polite' });
    const escolher = h('input', { type: 'file', multiple: true, 'aria-label': 'Escolher arquivos para enviar' });
    const zona = h('label', { class: 'soltar' },
      escolher,
      icone('subir', 'ic-grande'),
      h('strong', { text: 'Solte os arquivos aqui ou clique para escolher' }),
      h('span', { class: 'soltar-formatos' }));
    const organizar = botao('Organizar o cérebro com IA', { classe: 'btn-primario', icone: 'ia', aoClicar: organizarCerebro });
    const minha = tela = { main, listaEl, filaEl, zona, escolher, organizar };
    itens.clear();

    escolher.addEventListener('change', () => {
      enviarArquivos(Array.from(escolher.files || []));
      escolher.value = '';
    });
    for (const ev of ['dragenter', 'dragover']) {
      zona.addEventListener(ev, (e) => { e.preventDefault(); zona.classList.add('sobre'); });
    }
    for (const ev of ['dragleave', 'drop']) {
      zona.addEventListener(ev, (e) => { e.preventDefault(); zona.classList.remove('sobre'); });
    }
    zona.addEventListener('drop', (e) => {
      const arquivos = Array.from((e.dataTransfer && e.dataTransfer.files) || []);
      if (arquivos.length) enviarArquivos(arquivos);
    });

    const colar = { titulo: '', texto: '' };
    const campoTitulo = entrada(colar, 'titulo', { autocomplete: 'off', placeholder: 'Ex.: Perguntas que os clientes mais fazem' });
    const campoTexto = areaTexto(colar, 'texto', { rows: 6 });

    por(main, [
      h('div', { class: 'topo-tela' }, h('div', { class: 'topo-textos' },
        h('h1', { text: 'Base de conhecimento' }),
        h('p', { class: 'descricao', text: 'Envie tabela de preços, catálogo, regulamento, o que o seu negócio tiver. A IA lê tudo e o assistente consulta os trechos certos na hora de responder.' }))),
      h('div', { class: 'pilha' },
        zona,
        filaEl,
        h('details', { class: 'cartao' },
          h('summary', { class: 'resumo-linha' }, icone('lapis'), h('span', { text: 'Colar um texto' })),
          h('div', { class: 'pilha' },
            campo('Título', campoTitulo),
            campo('Texto', campoTexto, 'Cole aqui qualquer informação que não está em arquivo.'),
            h('div', { class: 'acoes' }, botao('Adicionar texto', {
              classe: 'btn-primario', icone: 'mais',
              aoClicar: async () => {
                if (!colar.titulo.trim() || !colar.texto.trim()) throw new Error('Preencha o título e o texto.');
                await api.post('/api/conhecimento/texto', { titulo: colar.titulo.trim(), texto: colar.texto.trim() });
                colar.titulo = ''; colar.texto = '';
                campoTitulo.value = ''; campoTexto.value = '';
                toast('Texto adicionado. A IA está lendo.');
                await recarregar();
              },
            })))),
        h('section', { class: 'cartao' },
          h('div', { class: 'editor-topo' },
            h('h2', { text: 'Materiais' }),
            organizar),
          h('p', { class: 'descricao', text: 'O botão junta os materiais e os dados do negócio em um cérebro novo, para você revisar antes de salvar. Pode levar até um minuto.' }),
          listaEl)),
    ]);

    pintarFila();
    trocar(listaEl, h('li', null, carregandoEl('Carregando os materiais...')));
    try {
      await carregarMateriais();
    } catch (e) {
      if (tela === minha) trocar(listaEl, h('li', null, falhaCarga(e, () => { main.replaceChildren(); montar(main); })));
      return;
    }
    if (tela !== minha) return;
    pintarLista();
  }

  function sair() { tela = null; }

  function atualizar() {
    if (!tela || !S.materiais) return;
    if (S.materiais.some((m) => m.estado === 'organizando')) recarregar();
  }

  async function recarregar() {
    try { await carregarMateriais(); } catch (e) { return; }
    pintarLista();
  }

  // ---------- Envio ----------

  function pintarFila() {
    if (!tela) return;
    trocar(tela.filaEl, fila.map((f) => h('li', { class: f.erro ? 'erro' : null },
      f.erro ? icone('alerta') : (f.estado === 'Enviado' ? icone('check') : h('span', { class: 'rodando' })),
      h('span', { class: 'fila-nome', text: f.nome }),
      h('span', { class: 'fila-estado', text: f.erro || f.estado }),
      f.erro ? h('button', {
        type: 'button', class: 'btn-icone', 'aria-label': 'Tirar ' + f.nome + ' da lista',
        onclick: () => { fila = fila.filter((x) => x !== f); pintarFila(); },
      }, icone('fechar')) : null)));
  }

  async function enviarArquivos(arquivos) {
    for (const arq of arquivos) {
      const item = { nome: arq.name, estado: 'Na fila', erro: '', arq };
      const ext = (arq.name.split('.').pop() || '').toLowerCase();
      if (!S.formatos.includes(ext)) {
        item.erro = ext === 'doc'
          ? 'Arquivo .doc antigo não é aceito. Salve como .docx ou PDF e envie de novo.'
          : 'Formato não aceito. Use ' + S.formatos.join(', ') + '.';
      } else if (arq.size > S.limiteMb * 1024 * 1024) {
        item.erro = 'Arquivo maior que ' + S.limiteMb + ' MB.';
      } else if (arq.size === 0) {
        item.erro = 'Arquivo vazio.';
      }
      fila.push(item);
    }
    pintarFila();
    if (enviando) return;
    enviando = true;
    try {
      let proximo;
      while ((proximo = fila.find((f) => f.estado === 'Na fila' && !f.erro))) {
        proximo.estado = 'Enviando...';
        pintarFila();
        try {
          await api.arquivo(proximo.arq);
          proximo.estado = 'Enviado';
          const feito = proximo;
          setTimeout(() => { fila = fila.filter((x) => x !== feito); pintarFila(); }, 3000);
          await recarregar();
        } catch (e) {
          proximo.erro = e.message || 'Não consegui enviar.';
        }
        proximo.arq = null;
        pintarFila();
      }
    } finally {
      enviando = false;
    }
  }

  // ---------- Lista ----------

  function itemMaterial(m) {
    const organizando = m.estado === 'organizando';
    const titulo = m.titulo || m.nome || 'Material';
    return h('li', { class: 'material' + (m.ativo ? '' : ' desligado') },
      icone('arquivo', 'ic-material'),
      h('div', { class: 'material-corpo' },
        h('div', { class: 'material-titulo', text: titulo }),
        h('div', { class: 'material-meta' },
          m.nome && m.nome !== titulo ? h('span', { text: m.nome }) : null,
          m.tipo ? h('span', { text: String(m.tipo).toUpperCase() }) : null,
          m.tamanho ? h('span', { text: tamanhoBonito(m.tamanho) }) : null,
          h('span', { text: (m.trechos || 0) + (m.trechos === 1 ? ' trecho' : ' trechos') }),
          m.perguntas ? h('span', { text: m.perguntas + ' perguntas e respostas' }) : null),
        organizando ? h('p', { class: 'material-estado' }, h('span', { class: 'rodando' }), 'A IA está lendo este material...') : null,
        m.estado === 'erro' || m.erro
          ? h('p', { class: 'material-erro' }, icone('alerta'), h('span', { text: m.erro || 'Não consegui ler este arquivo.' }))
          : null,
        m.resumo ? h('details', { class: 'material-resumo' }, h('summary', { class: 'resumo-linha', text: 'Ver resumo' }), h('p', { text: m.resumo })) : null),
      h('div', { class: 'material-acoes' },
        chave('Usar nas respostas', m.ativo, async (novo, b) => {
          b.disabled = true;
          try {
            await api.post('/api/conhecimento/alterar', { id: m.id, ativo: novo });
            toast(novo ? 'O assistente voltou a usar este material.' : 'O assistente não usa mais este material.');
          } catch (e) {
            toast(e.message, 'erro');
          }
          b.disabled = false;
          await recarregar();
        }, { manual: true }),
        botao('', {
          icone: 'lixo', classe: 'btn-icone', rotulo: 'Excluir ' + titulo,
          aoClicar: async () => {
            const sim = await confirmar({
              titulo: 'Excluir este material?',
              texto: '"' + titulo + '" sai da base de conhecimento e o assistente para de consultar. Não dá para desfazer.',
              botao: 'Excluir material', perigo: true,
            });
            if (!sim) return;
            await api.post('/api/conhecimento/excluir', { id: m.id });
            toast('Material excluído.');
            await recarregar();
          },
        })));
  }

  function pintarLista() {
    if (!tela || !S.materiais) return;
    const lista = S.materiais;
    tela.organizar.disabled = !lista.some((m) => m.estado === 'pronto' && m.ativo) || tela.organizar.classList.contains('ocupado');
    tela.zona.querySelector('.soltar-formatos').textContent =
      S.formatos.map((f) => f.toUpperCase()).join(', ') + '. Até ' + S.limiteMb + ' MB cada. Pode enviar vários de uma vez.';
    tela.escolher.accept = S.formatos.map((f) => '.' + f).join(',');

    if (!lista.length) {
      itens.clear();
      trocar(tela.listaEl, h('li', null, vazio('conhecimento', 'Nenhum material ainda',
        'Envie o primeiro arquivo acima. Sem materiais, o assistente responde só com o que está escrito no cérebro.')));
      return;
    }
    const ordem = [];
    const vivos = new Set();
    for (const m of lista) {
      vivos.add(m.id);
      const marca = JSON.stringify(m);
      let it = itens.get(m.id);
      if (!it || it.marca !== marca) {
        const aberto = it ? !!(it.el.querySelector('details') || {}).open : false;
        const el = itemMaterial(m);
        const dobra = el.querySelector('details');
        if (dobra && aberto) dobra.open = true;
        it = { el, marca };
        itens.set(m.id, it);
      }
      ordem.push(it.el);
    }
    for (const id of Array.from(itens.keys())) if (!vivos.has(id)) itens.delete(id);
    const atuais = Array.from(tela.listaEl.children);
    if (atuais.length !== ordem.length || atuais.some((el, i) => el !== ordem[i])) tela.listaEl.replaceChildren(...ordem);
    // Só no primeiro desenho da tela, os materiais entram em sequência.
    if (!tela.listaPintada) {
      tela.listaPintada = true;
      if (typeof entrarEmSequencia === 'function') entrarEmSequencia(ordem);
    }
  }

  async function organizarCerebro() {
    const r = await api.post('/api/ia/cerebro', {});
    if (!r.texto || !r.texto.trim()) throw new Error('A IA devolveu um cérebro vazio. Tente de novo.');
    await carregarTextos();
    const ed = S.editores.cerebro;
    if (ed.antesDaIA == null) ed.antesDaIA = ed.atual;
    ed.atual = r.texto;
    toast('Rascunho do cérebro pronto. Revise e salve.');
    location.hash = '#assistente/cerebro';
  }

  TELAS.conhecimento = { montar, atualizar, sair };
})();
