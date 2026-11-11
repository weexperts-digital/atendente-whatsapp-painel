'use strict';
// Aba Treinar: de um lado o simulador de conversa, do outro tudo o que a IA aprendeu.
// O simulador nunca manda nada para o WhatsApp. O que se ensina vale já na próxima mensagem.

(function () {
  let tela = null;
  let contador = 0;
  let cacheBanco = null;

  const SUGESTOES = ['Oi, tudo bem?', 'Quanto custa?', 'Quero marcar um horário', 'Quero falar com uma pessoa'];
  const FRASE_TEMP = { quente: 'Cliente quente', morno: 'Cliente morno', frio: 'Cliente frio', indefinido: 'Ainda sem classificação' };
  const MAX_SITUACAO = 500;
  const MAX_RESPOSTA = 1200;
  const LAMPADA = [
    'M15 14c.2-1 .7-1.7 1.5-2.5 1-.9 1.5-2.2 1.5-3.5A6 6 0 0 0 6 8c0 1 .2 2.2 1.5 3.5.7.7 1.3 1.5 1.5 2.5',
    'M9 18h6', 'M10 22h4',
  ];

  // ---------- Peças pequenas ----------

  // desenho cria um ícone que não está na folha de ícones do index.html.
  function desenho(tracos, classe) {
    const svg = document.createElementNS(SVG, 'svg');
    svg.setAttribute('class', 'ic' + (classe ? ' ' + classe : ''));
    svg.setAttribute('viewBox', '0 0 24 24');
    svg.setAttribute('aria-hidden', 'true');
    svg.setAttribute('focusable', 'false');
    for (const d of tracos) {
      const p = document.createElementNS(SVG, 'path');
      p.setAttribute('d', d);
      svg.appendChild(p);
    }
    return svg;
  }

  // entrar faz o elemento aparecer com suavidade.
  function entrar(el) {
    if (typeof animar === 'function') {
      // animar() já respeita quem pediu menos movimento no sistema.
      try {
        animar(el, { opacity: [0, 1], y: [6, 0] }, { duration: 0.22 });
        return el;
      } catch (e) { /* cai na animação do CSS */ }
    }
    el.classList.add('tl-entrando');
    return el;
  }

  function umaLinha(s) {
    return String(s == null ? '' : s).replace(/\s+/g, ' ').trim();
  }

  function memoria() {
    if (!S.aprender) S.aprender = { form: { aberto: false, situacao: '', resposta: '' }, editando: null, notas: null, notasAbertas: false };
    return S.aprender;
  }

  // "2026-11-12" vira "qui 12/11".
  function dataFalada(data) {
    const m = /^(\d{4})-(\d{2})-(\d{2})$/.exec(data || '');
    if (!m) return data || '';
    const d = new Date(Number(m[1]), Number(m[2]) - 1, Number(m[3]));
    return DIAS_CURTOS[d.getDay()] + ' ' + m[3] + '/' + m[2];
  }

  function fraseAgenda(ag) {
    if (!ag || (ag.acao !== 'marcar' && ag.acao !== 'cancelar')) return '';
    const quando = [dataFalada(ag.data), ag.hora ? 'às ' + ag.hora : ''].filter(Boolean).join(' ');
    const partes = [ag.servico, quando].filter(Boolean).join(', ');
    if (ag.acao === 'marcar') return partes ? 'Marcaria: ' + partes : 'Marcaria um horário';
    return partes ? 'Cancelaria: ' + partes : 'Cancelaria o horário marcado';
  }

  // ---------- Aprendizados: ler e montar o texto ----------
  // Cada linha que começa com "- " é um aprendizado. O resto são anotações livres.

  const RE_QUANDO = /^Quando o cliente disser algo como "([\s\S]*?)", responda nesta linha: "([\s\S]*)"$/;
  const RE_GERAL = /^Responda nesta linha: "([\s\S]*)"$/;

  function lerItem(linhas) {
    const bruto = linhas.join('\n');
    const corpo = umaLinha(linhas.join(' ').slice(2));
    let m = RE_QUANDO.exec(corpo);
    if (m) return { bruto, corpo, situacao: m[1], resposta: m[2] };
    m = RE_GERAL.exec(corpo);
    if (m) return { bruto, corpo, situacao: '', resposta: m[1] };
    return { bruto, corpo, livre: true };
  }

  function arrumarNotas(t) {
    return String(t || '').replace(/\r\n?/g, '\n').replace(/[ \t]+$/gm, '').replace(/\n{3,}/g, '\n\n').trim();
  }

  function separar(texto) {
    const grupos = [];
    const soltas = [];
    let atual = null;
    for (const linha of String(texto || '').replace(/\r\n?/g, '\n').split('\n')) {
      if (linha.startsWith('- ')) {
        atual = [linha];
        grupos.push(atual);
      } else if (atual && /^[ \t]+\S/.test(linha)) {
        atual.push(linha); // continuação recuada do mesmo aprendizado
      } else {
        atual = null;
        soltas.push(linha);
      }
    }
    return { itens: grupos.map(lerItem).filter((i) => i.corpo), notas: arrumarNotas(soltas.join('\n')) };
  }

  function montarTexto(notas, itens) {
    const partes = [];
    const n = arrumarNotas(notas);
    if (n) partes.push(n);
    if (itens.length) partes.push(itens.map((i) => i.bruto).join('\n'));
    return partes.length ? partes.join('\n\n') + '\n' : '';
  }

  function linhaDe(situacao, resposta) {
    const s = umaLinha(situacao);
    const r = umaLinha(resposta);
    return s
      ? '- Quando o cliente disser algo como "' + s + '", responda nesta linha: "' + r + '"'
      : '- Responda nesta linha: "' + r + '"';
  }

  function editor() {
    if (!S.editores.aprendizados) S.editores.aprendizados = { salvo: '', atual: '', padrao: false, antesDaIA: null };
    return S.editores.aprendizados;
  }

  function banco() {
    const texto = editor().salvo || '';
    if (!cacheBanco || cacheBanco.texto !== texto) cacheBanco = Object.assign({ texto }, separar(texto));
    return cacheBanco;
  }

  function notasSujas() {
    const m = memoria();
    return m.notas != null && arrumarNotas(m.notas) !== banco().notas;
  }

  // O rascunho das anotações conta como alteração não salva para o aviso de sair da página.
  function sincronizar() {
    const ed = editor();
    ed.atual = notasSujas() ? montarTexto(memoria().notas, banco().itens) : ed.salvo;
  }

  function definirSalvo(texto) {
    editor().salvo = String(texto || '');
    sincronizar();
  }

  async function gravar(itens, notas) {
    const texto = montarTexto(notas, itens);
    await api.post('/api/textos', { arquivo: 'aprendizados', texto });
    definirSalvo(texto);
  }

  // ensinar acrescenta um aprendizado e devolve a linha nova, para destacar na lista.
  async function ensinar(situacao, resposta) {
    const r = await api.post('/api/aprendizado', { situacao: umaLinha(situacao), resposta: umaLinha(resposta) });
    if (typeof r.texto === 'string') {
      definirSalvo(r.texto);
    } else {
      await carregarTextos();
      sincronizar();
    }
    const itens = banco().itens;
    return itens.length ? itens[itens.length - 1].bruto : '';
  }

  // ---------- Tela ----------

  async function montar(main) {
    const m = memoria();
    const mensagens = h('div', { class: 'tr-mensagens', role: 'log', 'aria-live': 'polite', 'aria-label': 'Conversa de treino', tabindex: '0' });
    const estreita = window.matchMedia && window.matchMedia('(max-width: 600px)').matches;
    const texto = h('textarea', {
      rows: 1, placeholder: estreita ? 'Escreva como cliente' : 'Escreva como se fosse um cliente',
      'aria-label': 'Mensagem do cliente', maxLength: 2000,
    });
    const enviar = h('button', { type: 'button', class: 'btn btn-primario', 'aria-label': 'Enviar mensagem' }, icone('enviar'), h('span', { text: 'Enviar' }));
    const conta = h('span', { class: 'tr-conta', text: '' });
    const ensinarEl = h('div', { class: 'tr-ensinar', hidden: true });
    const lista = h('div', { class: 'tr-lista' });
    const notas = h('div');
    const rolagem = h('div', { class: 'tr-banco-rolagem' });
    const botaoEnsinar = h('button', {
      type: 'button', class: 'btn btn-primario', 'aria-expanded': 'false',
      onclick: () => abrirEnsinar(!m.form.aberto),
    }, icone('mais'), h('span', { text: 'Ensinar algo novo' }));
    const grade = h('div', { class: 'tr-grade' });
    const minha = tela = { main, grade, mensagens, texto, enviar, conta, ensinar: ensinarEl, lista, notas, rolagem, botaoEnsinar, digitando: null };

    enviar.addEventListener('click', () => mandar());
    texto.addEventListener('keydown', (ev) => {
      if (ev.key === 'Enter' && !ev.shiftKey && !ev.isComposing) { ev.preventDefault(); mandar(); }
    });
    texto.addEventListener('input', ajustarTexto);

    por(grade, [
      h('section', { class: 'tr-painel tr-simulador', 'aria-label': 'Simulador de conversa' },
        h('div', { class: 'tr-painel-topo' },
          h('div', { class: 'tr-painel-titulo' },
            h('h2', { text: 'Simulador' }),
            h('p', { text: 'Nada vai para o WhatsApp e nada é marcado na agenda.' })),
          botao('Nova conversa', {
            classe: 'btn-suave', icone: 'desfazer',
            aoClicar: () => { novaConversa(); pintarChat(); if (tela) tela.texto.focus(); },
          })),
        mensagens,
        h('div', { class: 'tr-compor' }, texto, enviar)),
      h('section', { class: 'tr-painel tr-banco', 'aria-label': 'O que a IA aprendeu' },
        h('div', { class: 'tr-painel-topo' },
          h('div', { class: 'tr-painel-titulo' },
            h('h2', null, 'O que a IA aprendeu ', conta)),
          botaoEnsinar),
        por(rolagem, [
          h('p', { class: 'ajuda tr-banco-nota', text: 'O assistente lê esta lista antes de cada resposta. O que você ensina vale já na próxima mensagem.' }),
          ensinarEl, lista, notas,
        ])),
    ]);

    trocar(main, h('div', { class: 'tela-treinar' },
      h('div', { class: 'topo-tela' }, h('div', { class: 'topo-textos' },
        h('h1', { text: 'Treinar' }),
        h('p', { class: 'descricao', text: 'Converse como se fosse um cliente. Se uma resposta não ficar boa, corrija. O assistente aprende na hora.' }))),
      grade));

    window.addEventListener('resize', ajustarAltura);
    ajustarAltura();
    pintarChat();
    pintarEnsinar();
    await carregarBanco(minha);
  }

  function sair() {
    window.removeEventListener('resize', ajustarAltura);
    tela = null;
  }

  // Os dois painéis ocupam a altura que sobra na janela (em telas largas).
  function ajustarAltura() {
    if (!tela) return;
    // offsetTop não muda com a animação de entrada da tela (getBoundingClientRect mudaria).
    let topo = 0;
    for (let el = tela.grade; el; el = el.offsetParent) topo += el.offsetTop;
    const respiro = parseFloat(getComputedStyle(tela.main).paddingBottom) || 24;
    tela.grade.style.setProperty('--tr-altura', Math.max(520, Math.round(window.innerHeight - topo - respiro)) + 'px');
  }

  function ajustarTexto() {
    if (!tela) return;
    tela.texto.style.height = 'auto';
    tela.texto.style.height = Math.min(tela.texto.scrollHeight + 2, 140) + 'px';
  }

  // ---------- Simulador ----------

  function novaConversa() {
    S.treino = { mensagens: [], lead: {}, ultimo: null, esperando: false, rodada: (S.treino.rodada || 0) + 1 };
  }

  function situacaoDe(indice) {
    for (let i = indice - 1; i >= 0; i--) {
      if (S.treino.mensagens[i].de === 'cliente') return S.treino.mensagens[i].texto;
    }
    return '';
  }

  function bolhaCliente(m) {
    return h('div', { class: 'tr-msg tr-msg-cliente' },
      h('span', { class: 'so-leitor', text: 'Você, como cliente:' }),
      h('p', { class: 'tr-bolha tr-bolha-cliente', text: m.texto }));
  }

  function comeco() {
    return h('div', { class: 'tr-comeco' },
      icone('treinar', 'ic-grande'),
      h('h3', { text: 'Teste o seu assistente' }),
      h('p', { text: 'Escreva abaixo o que um cliente escreveria. Ou comece por uma destas mensagens.' }),
      h('div', { class: 'tr-sugestoes' }, SUGESTOES.map((s) => h('button', {
        type: 'button', class: 'tr-sugestao', text: s, onclick: () => mandar(s),
      }))));
  }

  function digitandoEl() {
    return h('div', { class: 'tr-digitando', role: 'status' },
      h('span', { class: 'pontinhos', 'aria-hidden': 'true' }, h('span'), h('span'), h('span')),
      h('span', { text: 'O assistente está escrevendo. Pode levar até um minuto.' }));
  }

  // detalhesDe monta o que aparece em "Ver detalhes": lead, materiais e tempo.
  function detalhesDe(info) {
    const l = info.lead || {};
    const pontos = Math.max(0, Math.min(100, Math.round(Number(l.pontuacao) || 0)));
    const trechos = Array.isArray(info.trechos) ? info.trechos : [];
    const segundos = ((Number(info.ms) || 0) / 1000).toFixed(1).replace('.', ',');
    return [
      h('dl', { class: 'tr-dados' },
        definicao('Nome', l.nome),
        definicao('O que precisa', l.necessidade),
        definicao('Prazo', l.prazo),
        definicao('Quem decide', l.decisor),
        definicao('Orçamento', l.orcamento),
        definicao('Etapa da conversa', l.etapa, 'Começando'),
        definicao('Resumo', l.resumo, 'Ainda sem resumo'),
        h('dt', { text: 'Pontos' }), h('dd', { text: pontos + ' de 100' }),
        info.transbordo ? [h('dt', { text: 'Por que chamaria uma pessoa' }), h('dd', { text: info.motivo || 'Não informou o motivo' })] : null),
      h('div', { class: 'tr-materiais' },
        h('h3', { text: 'Materiais consultados' }),
        trechos.length
          ? trechos.map((t) => h('details', { class: 'tr-trecho' },
            h('summary', { class: 'resumo-linha', text: t.titulo || 'Material' }),
            h('p', { text: t.texto || '' })))
          : h('p', { class: 'ajuda', text: 'Nenhum material da base de conhecimento entrou nesta resposta.' })),
      h('p', { class: 'ajuda', text: 'Respondeu em ' + segundos + ' segundos.' + (info.modo === 'teste' ? ' Em modo teste, sem o Claude.' : '') }),
    ];
  }

  function respostaIA(m, indice) {
    const info = m.info || {};
    const id = ++contador;
    const caixa = h('div', { class: 'tr-msg tr-msg-ia' });
    const acoes = h('div', { class: 'tr-acoes' });
    const detalhes = h('div', { class: 'tr-detalhes', id: 'tr-detalhes-' + id, hidden: true });
    const correcao = h('div', { class: 'tr-correcao', id: 'tr-correcao-' + id, hidden: true });

    // Uma linha curta com o que a IA entendeu.
    const temp = temperatura((info.lead || {}).classificacao);
    const agenda = fraseAgenda(info.agendamento);
    const entendeu = h('div', { class: 'tr-entendeu' },
      h('span', { class: 'so-leitor', text: 'O que a IA entendeu:' }),
      h('span', { class: 'etiqueta temp-' + temp }, icone('termometro'), FRASE_TEMP[temp]),
      info.transbordo ? h('span', { class: 'etiqueta etiqueta-aguardando' }, icone('pessoa'), 'Chamaria uma pessoa') : null,
      agenda ? h('span', { class: 'etiqueta tr-etiqueta-agenda' }, icone('agenda'), agenda) : null);

    function alargar() {
      caixa.classList.toggle('tr-msg-aberta', !!(m.aberto || m.corrigindo));
    }

    function pintarAcoes() {
      trocar(acoes,
        h('button', {
          type: 'button', class: 'tr-link', 'aria-expanded': m.aberto ? 'true' : 'false', 'aria-controls': detalhes.id,
          onclick: () => { m.aberto = !m.aberto; pintarAcoes(); pintarDetalhes(true); },
        }, icone('info'), m.aberto ? 'Esconder detalhes' : 'Ver detalhes'),
        m.corrigido
          ? h('span', { class: 'etiqueta etiqueta-ok' }, icone('check'), 'Correção salva')
          : h('button', {
            type: 'button', class: 'tr-link', 'aria-expanded': m.corrigindo ? 'true' : 'false', 'aria-controls': correcao.id,
            onclick: () => { m.corrigindo = !m.corrigindo; pintarAcoes(); pintarCorrecao(true); },
          }, icone('lapis'), 'Corrigir'));
    }

    function pintarDetalhes(mostrar) {
      detalhes.hidden = !m.aberto;
      alargar();
      if (!m.aberto) return;
      if (!detalhes.firstChild) por(detalhes, detalhesDe(info));
      if (mostrar) detalhes.scrollIntoView({ block: 'nearest' });
    }

    function pintarCorrecao(focar) {
      correcao.hidden = !m.corrigindo;
      alargar();
      if (!m.corrigindo) { correcao.replaceChildren(); return; }
      if (m.correcao == null) m.correcao = '';
      const area = areaTexto(m, 'correcao', { rows: 3, maxLength: MAX_RESPOSTA, placeholder: 'Escreva a resposta ideal' });
      trocar(correcao,
        campo('Como deveria ter respondido?', area, 'O assistente passa a seguir este exemplo quando perguntarem algo parecido.'),
        h('div', { class: 'acoes' },
          botao('Salvar correção', {
            classe: 'btn-primario', icone: 'check',
            aoClicar: async () => {
              if (!m.correcao.trim()) { area.focus(); throw new Error('Escreva como o assistente deveria ter respondido.'); }
              const nova = await ensinar(situacaoDe(indice), m.correcao);
              m.corrigindo = false;
              m.corrigido = true;
              m.correcao = '';
              pintarAcoes();
              pintarCorrecao();
              pintarBanco(nova);
              toast('Correção salva. Ela já aparece em O que a IA aprendeu.');
            },
          }),
          botao('Cancelar', { classe: 'btn-fantasma', aoClicar: () => { m.corrigindo = false; pintarAcoes(); pintarCorrecao(); } })));
      if (focar) { area.focus(); correcao.scrollIntoView({ block: 'nearest' }); }
    }

    por(caixa, [
      h('span', { class: 'tr-autor', text: 'Assistente' }),
      m.texto
        ? h('p', { class: 'tr-bolha tr-bolha-ia', text: m.texto })
        : h('p', { class: 'tr-bolha tr-bolha-ia tr-bolha-muda', text: 'O assistente ficaria em silêncio aqui.' }),
      entendeu, acoes, detalhes, correcao,
    ]);
    pintarAcoes();
    pintarDetalhes();
    pintarCorrecao();
    return caixa;
  }

  function rolarFim() {
    if (tela) tela.mensagens.scrollTop = tela.mensagens.scrollHeight;
  }

  function pintarChat() {
    if (!tela) return;
    const t = S.treino;
    const nos = t.mensagens.map((m, i) => (m.de === 'cliente' ? bolhaCliente(m) : respostaIA(m, i)));
    if (!nos.length) nos.push(comeco());
    tela.digitando = null;
    if (t.esperando) { tela.digitando = digitandoEl(); nos.push(tela.digitando); }
    trocar(tela.mensagens, nos);
    tela.enviar.disabled = t.esperando;
    rolarFim();
  }

  async function mandar(pronto) {
    if (!tela) return;
    const t = S.treino;
    const digitado = typeof pronto !== 'string';
    const texto = (digitado ? tela.texto.value : pronto).trim();
    if (!texto || t.esperando) return;
    const rodada = t.rodada;
    const primeira = t.mensagens.length === 0;
    t.mensagens.push({ de: 'cliente', texto });
    t.esperando = true;
    if (digitado) { tela.texto.value = ''; ajustarTexto(); }
    if (primeira) {
      pintarChat();
    } else {
      tela.mensagens.appendChild(entrar(bolhaCliente({ texto })));
      tela.digitando = digitandoEl();
      tela.mensagens.appendChild(tela.digitando);
      tela.enviar.disabled = true;
      rolarFim();
    }

    let r = null;
    let erro = null;
    try {
      r = await api.post('/api/treinar', {
        mensagens: t.mensagens.map((m) => ({ de: m.de, texto: m.texto })),
        lead: t.lead || {},
      });
    } catch (e) {
      erro = e;
    }
    if (S.treino !== t || t.rodada !== rodada) return; // a pessoa começou outra conversa
    t.esperando = false;

    if (erro) {
      t.mensagens.pop();
      toast(erro.message, 'erro');
      if (tela) {
        pintarChat();
        if (digitado && !tela.texto.value) { tela.texto.value = texto; ajustarTexto(); }
        tela.texto.focus();
      }
      return;
    }
    t.lead = r.lead || {};
    t.ultimo = r;
    const msg = {
      de: 'atendente', texto: r.resposta || '',
      info: {
        transbordo: !!r.transbordo, motivo: r.motivo || '', lead: r.lead || {},
        agendamento: r.agendamento || {}, trechos: r.trechos || [], modo: r.modo || '', ms: r.ms || 0,
      },
    };
    t.mensagens.push(msg);
    if (!tela) return;
    if (tela.digitando && tela.digitando.isConnected) {
      tela.digitando.replaceWith(entrar(respostaIA(msg, t.mensagens.length - 1)));
      tela.digitando = null;
      tela.enviar.disabled = false;
      rolarFim();
    } else {
      pintarChat();
    }
  }

  // ---------- O que a IA aprendeu ----------

  async function carregarBanco(minha) {
    trocar(minha.lista, carregandoEl('Carregando os aprendizados...'));
    try {
      await carregarTextos();
    } catch (e) {
      if (tela !== minha) return;
      trocar(minha.lista, h('div', { class: 'tr-vazio' },
        icone('alerta', 'ic-grande'),
        h('h3', { text: 'Não consegui carregar os aprendizados' }),
        h('p', { text: e.message || 'Tente de novo em alguns segundos.' }),
        botao('Tentar de novo', { classe: 'btn-primario', aoClicar: () => carregarBanco(minha) })));
      return;
    }
    if (tela !== minha) return;
    sincronizar();
    pintarBanco();
  }

  function pintarBanco(destaque) {
    if (!tela) return;
    const n = banco().itens.length;
    tela.conta.textContent = n === 0 ? 'Nada ainda' : n === 1 ? '1 aprendizado' : n + ' aprendizados';
    pintarLista(destaque);
    pintarNotas();
  }

  function abrirEnsinar(abrir) {
    memoria().form.aberto = abrir;
    pintarEnsinar(abrir);
    if (abrir && tela) tela.rolagem.scrollTop = 0;
  }

  function pintarEnsinar(focar) {
    if (!tela) return;
    const f = memoria().form;
    tela.botaoEnsinar.setAttribute('aria-expanded', f.aberto ? 'true' : 'false');
    tela.ensinar.hidden = !f.aberto;
    if (!f.aberto) { tela.ensinar.replaceChildren(); return; }
    const situacao = areaTexto(f, 'situacao', { rows: 2, maxLength: MAX_SITUACAO, placeholder: 'Ex.: Vocês aceitam cartão?' });
    const resposta = areaTexto(f, 'resposta', { rows: 3, maxLength: MAX_RESPOSTA, placeholder: 'Ex.: Sim. Aceitamos Pix, débito e crédito em até 3 vezes.' });
    trocar(tela.ensinar,
      h('h3', { text: 'Ensinar algo novo' }),
      campo('Quando o cliente perguntar', situacao, 'Opcional. Deixe vazio se a orientação vale para qualquer conversa.'),
      campo('O assistente deve responder', resposta),
      h('div', { class: 'acoes' },
        botao('Salvar aprendizado', {
          classe: 'btn-primario', icone: 'check',
          aoClicar: async () => {
            if (!f.resposta.trim()) { resposta.focus(); throw new Error('Escreva como o assistente deve responder.'); }
            const nova = await ensinar(f.situacao, f.resposta);
            f.situacao = '';
            f.resposta = '';
            f.aberto = false;
            pintarEnsinar();
            pintarBanco(nova);
            if (tela) tela.botaoEnsinar.focus();
            toast('Aprendizado salvo. Vale já na próxima mensagem.');
          },
        }),
        botao('Cancelar', { classe: 'btn-fantasma', aoClicar: () => { abrirEnsinar(false); if (tela) tela.botaoEnsinar.focus(); } })));
    if (focar) situacao.focus();
  }

  function botaoIcone(nome, rotulo, acao) {
    const b = h('button', { type: 'button', class: 'btn-icone', 'aria-label': rotulo, title: rotulo }, icone(nome));
    b.addEventListener('click', () => ocupar(b, acao));
    return b;
  }

  function cartao(item, indice, novo) {
    const e = memoria().editando;
    if (e && e.indice === indice && e.bruto === item.bruto) return cartaoEdicao(item, indice);
    const corpo = item.livre
      ? h('p', { class: 'tr-fala', text: item.corpo })
      : [
        item.situacao
          ? h('div', { class: 'tr-par tr-par-pergunta' }, h('span', { class: 'tr-rotulo', text: 'Quando perguntarem' }), h('p', { class: 'tr-fala', text: item.situacao }))
          : null,
        h('div', { class: 'tr-par tr-par-resposta' },
          h('span', { class: 'tr-rotulo', text: item.situacao ? 'Responder' : 'Orientação geral' }),
          h('p', { class: 'tr-fala', text: item.resposta })),
      ];
    const li = h('li', { class: 'tr-cartao' + (novo ? ' tr-novo' : '') },
      h('div', { class: 'tr-cartao-corpo' }, corpo),
      h('div', { class: 'tr-cartao-acoes' },
        botaoIcone('lapis', 'Editar este aprendizado', () => {
          memoria().editando = {
            indice, bruto: item.bruto,
            situacao: item.situacao || '', resposta: item.resposta || '', texto: item.livre ? item.corpo : '',
          };
          pintarLista();
          const area = tela && tela.lista.querySelector('.tr-cartao-edicao textarea');
          if (area) area.focus();
        }),
        botaoIcone('lixo', 'Apagar este aprendizado', async () => {
          const sim = await confirmar({
            titulo: 'Apagar este aprendizado?',
            texto: 'O assistente deixa de seguir esta orientação. Não dá para desfazer.',
            botao: 'Apagar', perigo: true,
          });
          if (!sim) return;
          const b = banco();
          if (!b.itens[indice] || b.itens[indice].bruto !== item.bruto) {
            pintarBanco();
            throw new Error('A lista mudou. Tente apagar de novo.');
          }
          await gravar(b.itens.filter((x, n) => n !== indice), b.notas);
          memoria().editando = null;
          pintarBanco();
          if (tela) tela.botaoEnsinar.focus();
          toast('Aprendizado apagado.');
        })));
    return novo ? entrar(li) : li;
  }

  function cartaoEdicao(item, indice) {
    const e = memoria().editando;
    const campos = item.livre
      ? [campo('Texto do aprendizado', areaTexto(e, 'texto', { rows: 3, maxLength: MAX_SITUACAO + MAX_RESPOSTA }))]
      : [
        campo('Quando perguntarem', areaTexto(e, 'situacao', { rows: 2, maxLength: MAX_SITUACAO }), 'Opcional. Deixe vazio se a orientação vale para qualquer conversa.'),
        campo('Responder', areaTexto(e, 'resposta', { rows: 3, maxLength: MAX_RESPOSTA })),
      ];
    const fechar = () => {
      memoria().editando = null;
      pintarLista();
    };
    return h('li', { class: 'tr-cartao tr-cartao-edicao' },
      campos,
      h('div', { class: 'acoes' },
        botao('Salvar alteração', {
          classe: 'btn-primario', icone: 'check',
          aoClicar: async () => {
            const b = banco();
            if (!b.itens[indice] || b.itens[indice].bruto !== item.bruto) {
              fechar();
              throw new Error('A lista mudou. Abra o aprendizado de novo para editar.');
            }
            let linha;
            if (item.livre) {
              const t = umaLinha(e.texto).replace(/^-\s*/, '');
              if (!t) throw new Error('Escreva o texto do aprendizado.');
              linha = '- ' + t;
            } else {
              if (!umaLinha(e.resposta)) throw new Error('Escreva como o assistente deve responder.');
              linha = linhaDe(e.situacao, e.resposta);
            }
            await gravar(b.itens.map((x, n) => (n === indice ? { bruto: linha } : x)), b.notas);
            memoria().editando = null;
            pintarBanco(linha);
            toast('Aprendizado atualizado. Vale já na próxima mensagem.');
          },
        }),
        botao('Cancelar', { classe: 'btn-fantasma', aoClicar: fechar })));
  }

  function pintarLista(destaque) {
    if (!tela) return;
    const itens = banco().itens;
    if (!itens.length) {
      trocar(tela.lista, h('div', { class: 'tr-vazio' },
        desenho(LAMPADA, 'ic-grande'),
        h('h3', { text: 'A IA ainda não aprendeu nada aqui' }),
        h('p', { text: 'Converse no simulador. Quando uma resposta não ficar boa, clique em Corrigir e escreva como deveria ser. A correção aparece nesta lista.' }),
        h('p', { text: 'Você também pode ensinar direto, sem conversar.' }),
        botao('Ensinar algo novo', { classe: 'btn-suave', icone: 'mais', aoClicar: () => abrirEnsinar(true) })));
      return;
    }
    // O mais novo fica em cima.
    const nos = [];
    let marcou = false;
    for (let i = itens.length - 1; i >= 0; i--) {
      const novo = !marcou && !!destaque && itens[i].bruto === destaque;
      if (novo) marcou = true;
      nos.push(cartao(itens[i], i, novo));
    }
    trocar(tela.lista, h('ul', { class: 'tr-cartoes' }, nos));
    if (marcou) tela.rolagem.scrollTop = 0;
  }

  function pintarNotas() {
    if (!tela) return;
    const m = memoria();
    const b = banco();
    const area = h('textarea', { rows: 5, spellcheck: 'true', placeholder: 'Ex.: Nunca prometa desconto sem falar com a equipe.' });
    area.value = m.notas != null ? m.notas : b.notas;
    const situacao = h('p', { class: 'situacao', 'aria-live': 'polite' });
    const salvar = botao('Salvar anotações', {
      classe: 'btn-suave',
      aoClicar: async () => {
        await gravar(banco().itens, m.notas != null ? m.notas : banco().notas);
        m.notas = null;
        m.notasAbertas = true;
        pintarBanco();
        toast('Anotações salvas. Valem já na próxima mensagem.');
      },
    });
    const refletir = () => {
      const sujo = notasSujas();
      situacao.textContent = sujo ? 'Alterações não salvas' : '';
      situacao.classList.toggle('sujo', sujo);
      salvar.disabled = !sujo;
      sincronizar();
    };
    area.addEventListener('input', () => { m.notas = area.value; refletir(); });
    const linhas = b.notas ? b.notas.split('\n').filter((l) => l.trim()).length : 0;
    const dobra = h('details', { class: 'tr-notas', open: m.notasAbertas || notasSujas() },
      h('summary', { class: 'resumo-linha' },
        h('span', { text: 'Anotações livres' }),
        linhas ? h('span', { class: 'tr-conta', text: linhas === 1 ? '1 linha' : linhas + ' linhas' }) : null),
      h('div', { class: 'tr-notas-corpo' },
        campo('Texto solto que o assistente também lê', area,
          'Aqui fica o que está no texto de aprendizados e não começa com um traço. Bom para regras gerais.'),
        h('div', { class: 'acoes' }, salvar, situacao)));
    dobra.addEventListener('toggle', () => {
      const abriu = dobra.open && !m.notasAbertas;
      m.notasAbertas = dobra.open;
      if (abriu) dobra.scrollIntoView({ block: 'nearest' }); // o texto abre no fim da lista
    });
    trocar(tela.notas, dobra);
    refletir();
  }

  TELAS.treinar = { montar, sair };
})();
