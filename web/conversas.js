'use strict';
// Aba Conversas: lista com busca e um filtro só, conversa com cara de WhatsApp e ficha do lead.
//
// A casca (app.js) busca /api/estado a cada 3 segundos e chama atualizar(). Aqui nada é
// recriado sem precisar: só entra na página o que mudou. Assim não se perde o texto
// digitado, o anexo escolhido, a rolagem nem um áudio que está tocando.
// Regra de ouro: nada que vem da API vira HTML. Tudo entra como texto.

(function () {
  const NS_SVG = 'http://www.w3.org/2000/svg';
  const LIMITE_ARQUIVO = 25 * 1024 * 1024;
  const MAX_ANEXOS = 10;
  const MAX_LEGENDA = 1000;
  const LARGURA_GAVETA = 1040; // com menos espaço que isso, a ficha vira painel deslizante
  const CHAVE_FICHA = 'conversas.ficha';
  // O WhatsApp toca bem ogg/opus (vira mensagem de voz) e mp4 com AAC. O mp4 com opus,
  // que o Chrome escolhe sozinho, não toca em todo celular: por isso o AAC vem antes.
  const TIPOS_VOZ = ['audio/ogg;codecs=opus', 'audio/mp4;codecs=mp4a.40.2', 'audio/mp4', 'audio/webm;codecs=opus'];
  // Fundos para as iniciais. Todos passam no contraste AA com letra branca.
  const CORES_AVATAR = ['#6B49F5', '#1F63C4', '#047857', '#B4235A', '#A85507', '#0E7490', '#9333EA', '#B45309'];

  let tela = null;              // peças da tela montada
  let painel = null;            // peças da conversa aberta
  const itens = new Map();      // id do lead -> peças do item da lista
  const rascunhos = new Map();  // id do lead -> { texto, anexos } (não se perde ao trocar de conversa)
  let busca = '';
  // O filtro tem três grupos que combinam entre si. A situação mora em S.filtro
  // ('todas', 'fila', 'atendimento' ou 'assistente'); a temperatura e a etiqueta ficam aqui.
  let tempFiltro = '';
  let etiquetaFiltro = '';
  let gravacao = null;          // gravação de voz em andamento
  let visor = null;             // janela da imagem ampliada
  let buscaEmCurso = null;
  let buscaEsperando = null;

  // ---------- Ícones que não estão no index.html ----------

  const DESENHOS = {
    busca: [['circle', { cx: 11, cy: 11, r: 8 }], ['path', { d: 'm21 21-4.3-4.3' }]],
    clipe: [['path', { d: 'm21.44 11.05-9.19 9.19a6 6 0 0 1-8.49-8.49l8.57-8.57A4 4 0 1 1 18 8.84l-8.59 8.57a2 2 0 0 1-2.83-2.83l8.49-8.48' }]],
    microfone: [['path', { d: 'M12 2a3 3 0 0 0-3 3v7a3 3 0 0 0 6 0V5a3 3 0 0 0-3-3Z' }], ['path', { d: 'M19 10v2a7 7 0 0 1-14 0v-2M12 19v3' }]],
    imagem: [['rect', { x: 3, y: 3, width: 18, height: 18, rx: 2 }], ['circle', { cx: 9, cy: 9, r: 2 }], ['path', { d: 'm21 15-3.1-3.1a2 2 0 0 0-2.8 0L6 21' }]],
    video: [['path', { d: 'm16 13 5.2 3.5a.5.5 0 0 0 .8-.4V7.9a.5.5 0 0 0-.8-.4L16 10.5' }], ['rect', { x: 2, y: 6, width: 14, height: 12, rx: 2 }]],
    copiar: [['rect', { x: 8, y: 8, width: 14, height: 14, rx: 2 }], ['path', { d: 'M4 16c-1.1 0-2-.9-2-2V4c0-1.1.9-2 2-2h10c1.1 0 2 .9 2 2' }]],
    etiqueta: [['path', { d: 'M12.6 2.6A2 2 0 0 0 11.2 2H4a2 2 0 0 0-2 2v7.2a2 2 0 0 0 .6 1.4l8.7 8.7a2.4 2.4 0 0 0 3.4 0l6.6-6.6a2.4 2.4 0 0 0 0-3.4Z' }], ['path', { d: 'M7.5 7.5h.01' }]],
    filtro: [['path', { d: 'M22 3H2l8 9.46V19l4 2v-8.54L22 3z' }]],
    painel: [['rect', { x: 3, y: 3, width: 18, height: 18, rx: 2 }], ['path', { d: 'M15 3v18' }]],
    descer: [['path', { d: 'M12 5v14M19 12l-7 7-7-7' }]],
    fone: [['path', { d: 'M3 14h3a2 2 0 0 1 2 2v3a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-7a9 9 0 0 1 18 0v7a2 2 0 0 1-2 2h-1a2 2 0 0 1-2-2v-3a2 2 0 0 1 2-2h3' }]],
    cadeado: [['rect', { x: 3, y: 11, width: 18, height: 11, rx: 2 }], ['path', { d: 'M7 11V7a5 5 0 0 1 10 0v4' }]],
    historico: [['path', { d: 'M3 12a9 9 0 1 0 9-9 9.75 9.75 0 0 0-6.74 2.74L3 8' }], ['path', { d: 'M3 3v5h5M12 7v5l4 2' }]],
    texto: [['path', { d: 'M15 12H3M17 18H3M21 6H3' }]],
    sabemos: [['rect', { x: 8, y: 2, width: 8, height: 4, rx: 1 }], ['path', { d: 'M16 4h2a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2h2M12 11h4M12 16h4M8 11h.01M8 16h.01' }]],
  };

  // ic desenha um ícone daqui; se o nome não for daqui, usa os do index.html.
  function ic(nome, classe) {
    const partes = DESENHOS[nome];
    if (!partes) return icone(nome, classe);
    const svg = document.createElementNS(NS_SVG, 'svg');
    svg.setAttribute('class', 'ic' + (classe ? ' ' + classe : ''));
    svg.setAttribute('viewBox', '0 0 24 24');
    svg.setAttribute('aria-hidden', 'true');
    svg.setAttribute('focusable', 'false');
    for (const [forma, attrs] of partes) {
      const el = document.createElementNS(NS_SVG, forma);
      for (const [k, v] of Object.entries(attrs)) el.setAttribute(k, String(v));
      svg.appendChild(el);
    }
    return svg;
  }

  // ---------- Ajudantes ----------

  const naFila = (l) => l.status === 'aguardando humano';
  const emAtendimento = (l) => l.status === 'em atendimento';
  const situacaoDe = (l) => (naFila(l) ? 'fila' : (emAtendimento(l) ? 'atendimento' : 'ia'));

  // Os três grupos do filtro. Em cada um vale uma escolha só; id vazio quer dizer "todas".
  const SITUACOES = [
    { id: '', rotulo: 'Todas' },
    { id: 'fila', rotulo: 'Na fila' },
    { id: 'atendimento', rotulo: 'Em atendimento' },
    { id: 'assistente', rotulo: 'Com o assistente' },
  ];
  const TEMPERATURAS_FILTRO = [
    { id: '', rotulo: 'Todas' },
    { id: 'quente', rotulo: 'Quentes', cor: 'var(--quente)' },
    { id: 'morno', rotulo: 'Mornos', cor: 'var(--morno)' },
    { id: 'frio', rotulo: 'Frios', cor: 'var(--frio)' },
  ];

  const MIDIAS = {
    imagem: { icone: 'imagem', palavra: 'Foto', falta: 'Imagem não disponível' },
    audio: { icone: 'microfone', palavra: 'Áudio', falta: 'Áudio não disponível' },
    video: { icone: 'video', palavra: 'Vídeo', falta: 'Vídeo não disponível' },
    documento: { icone: 'arquivo', palavra: 'Arquivo', falta: 'Arquivo não disponível' },
  };
  const MARCAS_PREVIA = { imagem: 'imagem', 'áudio': 'audio', audio: 'audio', 'vídeo': 'video', video: 'video', arquivo: 'documento', documento: 'documento' };

  function leads() { return (S.estado && S.estado.leads) || []; }
  function resumoDe(id) { return leads().find((l) => l.id === id) || null; }
  function nomeDe(l) { return (l && (l.exibir || l.nome || l.nome_whatsapp || l.telefone_bonito)) || 'Sem nome'; }
  function conectado() { return !!(S.estado && S.estado.status === 'conectado'); }

  function semAcento(s) {
    return String(s == null ? '' : s).toLowerCase().normalize('NFD').replace(/[̀-ͯ]/g, '');
  }

  function ler(chave) { try { return localStorage.getItem(chave); } catch (e) { return null; } }
  function gravar(chave, valor) { try { localStorage.setItem(chave, valor); } catch (e) { /* sem armazenamento */ } }

  // euMesmo diz o nome de quem está usando o painel (vem do login). Vazio se ninguém entrou.
  function euMesmo() { return SESSAO.usuario ? String(SESSAO.usuario.nome || '').trim() : ''; }

  // mudou só roda "fazer" quando o valor é diferente do que já está na tela.
  function mudou(dono, chave, valor, fazer) {
    if (dono.marcas[chave] === valor) return false;
    dono.marcas[chave] = valor;
    fazer();
    return true;
  }

  function rascunho(id) {
    let r = rascunhos.get(id);
    if (!r) { r = { texto: '', anexos: [] }; rascunhos.set(id, r); }
    return r;
  }

  function menosMovimento() {
    if (typeof semMovimento === 'function') return semMovimento();
    return !!(window.matchMedia && window.matchMedia('(prefers-reduced-motion: reduce)').matches);
  }

  // mover usa animar() do painel (biblioteca Motion). Sem ela, devolve null e a tela
  // cai na transição de CSS.
  function mover(el, quadros, opcoes) {
    try {
      if (typeof animar === 'function') return animar(el, quadros, opcoes) || null;
      if (window.Motion && typeof window.Motion.animate === 'function') return window.Motion.animate(el, quadros, opcoes) || null;
    } catch (e) { /* cai no CSS */ }
    return null;
  }

  function aoTerminar(controle, fazer) {
    let feito = false;
    const fim = () => { if (!feito) { feito = true; fazer(); } };
    try { Promise.resolve(controle && controle.finished ? controle.finished : controle).then(fim, fim); } catch (e) { fim(); }
    setTimeout(fim, 700);
  }

  // rotuloDia: Hoje, Ontem ou "12 de novembro".
  function rotuloDia(d) {
    const hoje = new Date();
    if (mesmoDia(d, hoje)) return 'Hoje';
    const ontem = new Date(hoje);
    ontem.setDate(hoje.getDate() - 1);
    if (mesmoDia(d, ontem)) return 'Ontem';
    const opc = { day: 'numeric', month: 'long' };
    if (d.getFullYear() !== hoje.getFullYear()) opc.year = 'numeric';
    try { return d.toLocaleDateString('pt-BR', opc); } catch (e) { return diaRotulo(d.toISOString()); }
  }

  function quando(iso) {
    const dia = diaRotulo(iso);
    return dia ? dia + ' às ' + horaCurta(iso) : '';
  }

  async function copiarTexto(texto) {
    try {
      if (navigator.clipboard && navigator.clipboard.writeText) {
        await navigator.clipboard.writeText(texto);
        return true;
      }
    } catch (e) { /* tenta do jeito antigo */ }
    const campoTemp = h('textarea', { class: 'cv-so-leitor', readOnly: true });
    campoTemp.value = texto;
    document.body.appendChild(campoTemp);
    campoTemp.select();
    let ok = false;
    try { ok = document.execCommand('copy'); } catch (e) { ok = false; }
    campoTemp.remove();
    return ok;
  }

  // ---------- Foto do contato ----------

  function chaveFoto(l) { return [nomeDe(l), l.tem_foto ? 1 : 0, l.foto_versao || ''].join('|'); }

  function iniciais(l) {
    const nome = String((l && (l.exibir || l.nome || l.nome_whatsapp)) || '').trim();
    const palavras = nome.split(/\s+/).filter((p) => /^\p{L}/u.test(p));
    if (!palavras.length) return '';
    const primeira = Array.from(palavras[0])[0];
    const ultima = palavras.length > 1 ? Array.from(palavras[palavras.length - 1])[0] : '';
    return (primeira + ultima).toUpperCase();
  }

  function corEstavel(texto) {
    let soma = 0;
    for (const letra of String(texto)) soma = (soma * 31 + letra.codePointAt(0)) >>> 0;
    return CORES_AVATAR[soma % CORES_AVATAR.length];
  }

  // avatar mostra a foto do contato. Sem foto, ou se ela não carregar, ficam as iniciais.
  function avatar(l, classe) {
    const el = h('span', { class: 'cv-avatar ' + classe, 'aria-hidden': 'true' });
    el.style.backgroundColor = corEstavel(nomeDe(l));
    const letras = iniciais(l);
    el.appendChild(letras ? h('span', { text: letras }) : icone('pessoa'));
    if (l.tem_foto) {
      const img = h('img', { alt: '', src: '/api/foto?id=' + encodeURIComponent(l.id) + '&v=' + encodeURIComponent(l.foto_versao || '') });
      img.addEventListener('error', () => img.remove());
      el.appendChild(img);
    }
    return el;
  }

  // ---------- Ações da fila ----------

  async function atualizarTudo() {
    await pulsar();
    await buscarLead();
  }

  // O programa sabe quem está atendendo pelo login. A página não manda nome nenhum.
  async function iniciarAtendimento(id) {
    await api.post('/api/assumir', { id });
    toast('Atendimento iniciado. Agora é você quem responde esta conversa.');
    await atualizarTudo();
    if (painel && painel.id === id && painel.pode) painel.texto.focus();
  }

  async function encerrarAtendimento(id) {
    await api.post('/api/retomar', { id });
    toast('Atendimento encerrado. O assistente voltou a responder esta conversa.');
    await atualizarTudo();
  }

  // ---------- Montagem da tela ----------

  function prefFicha() { return ler(CHAVE_FICHA) !== '0'; }

  function montar(main, sub) {
    // S.filtro guarda a situação escolhida. Valores antigos (de quando havia uma fileira
    // de botões) continuam valendo: a temperatura passa para o grupo dela.
    if (S.filtro === 'aguardando') S.filtro = 'fila';
    if (TEMPERATURAS_FILTRO.some((t) => t.id && t.id === S.filtro)) { tempFiltro = S.filtro; S.filtro = 'todas'; }
    if (!SITUACOES.some((x) => x.id && x.id === S.filtro)) S.filtro = 'todas';
    // "#conversas/<id>" abre direto a conversa da pessoa (a aba Contatos chega assim).
    if (sub) {
      try { S.aberto = decodeURIComponent(sub); } catch (e) { S.aberto = sub; }
      S.lead = null;
      try { history.replaceState(null, '', '#conversas'); } catch (e) { /* o endereço fica como está */ }
    }

    const campoBusca = h('input', {
      type: 'search', class: 'cv-busca-campo', placeholder: 'Buscar por nome ou telefone',
      'aria-label': 'Buscar conversa por nome ou telefone', autocomplete: 'off', spellcheck: 'false', value: busca,
    });
    const limparBusca = h('button', {
      type: 'button', class: 'cv-busca-limpar', 'aria-label': 'Limpar busca', hidden: !busca,
      onclick: () => { definirBusca(''); campoBusca.focus(); },
    }, icone('fechar'));
    campoBusca.addEventListener('input', () => definirBusca(campoBusca.value));

    // Um filtro só: o botão diz o que está valendo e abre o painel com os três grupos.
    const resumoFiltro = h('span', { class: 'cv-filtrar-texto' });
    const botaoFiltro = h('button', { type: 'button', class: 'cv-filtrar' },
      ic('filtro'), resumoFiltro, icone('seta-baixo', 'cv-filtrar-seta'));
    const limparFiltro = h('button', {
      type: 'button', class: 'cv-filtrar-limpar', 'aria-label': 'Limpar filtros', title: 'Limpar filtros', hidden: true,
      onclick: () => { limparFiltros(); botaoFiltro.focus(); },
    }, icone('fechar'));
    const caixaFiltro = h('div', { class: 'cv-filtro-caixa' }, botaoFiltro, limparFiltro);
    const filtro = suspenso(botaoFiltro, montarPainelFiltro, {
      papel: 'dialog', classe: 'cv-sus', largura: 'gatilho', altura: 720, ancora: caixaFiltro,
      aoFechar: () => { if (tela) tela.painelFiltro = null; },
    });
    // A fila é o que mais importa: quando tem gente esperando, o aviso fica sempre à vista.
    const textoFila = h('strong');
    const avisoFila = h('button', { type: 'button', class: 'cv-aviso-fila', hidden: true, onclick: verFila },
      icone('relogio'), textoFila, h('span', { class: 'cv-aviso-fila-ver' }, 'Ver', icone('seta-dir')));

    const lista = h('div', { class: 'cv-itens', role: 'group', 'aria-label': 'Lista de conversas' });
    const colunaLista = h('section', { class: 'cv-lista cv-wa', 'aria-label': 'Conversas' },
      h('div', { class: 'cv-lista-topo' },
        h('h1', { text: 'Conversas' }),
        h('a', { class: 'cv-baixar', href: '/leads.csv', download: 'leads.csv' }, icone('baixar'), h('span', { text: 'Baixar planilha de leads' }))),
      h('div', { class: 'cv-busca' }, ic('busca'), campoBusca, limparBusca),
      h('div', { class: 'cv-filtro-linha' }, caixaFiltro),
      avisoFila, lista);

    const chat = h('section', { class: 'cv-chat cv-wa', 'aria-label': 'Conversa aberta' });
    const ficha = h('aside', { class: 'cv-ficha', id: 'cv-ficha', 'aria-label': 'Detalhes do cliente' });
    const veu = h('button', { type: 'button', class: 'cv-veu', 'aria-label': 'Fechar detalhes', tabIndex: -1, onclick: () => definirFicha(false) });
    const raiz = h('div', { class: 'cv' }, colunaLista, chat, veu, ficha);
    main.appendChild(raiz);

    tela = {
      raiz, lista, chat, ficha, veu, campoBusca, limparBusca,
      filtro, botaoFiltro, resumoFiltro, limparFiltro, avisoFila, textoFila, painelFiltro: null,
      marcas: {}, marcaNota: null, marcaVazio: null, gaveta: false, fichaAberta: false, vezFicha: 0, topo: -1,
      arrastes: 0, observador: null,
    };
    painel = null;
    itens.clear();

    raiz.addEventListener('keydown', (ev) => {
      if (ev.key === 'Escape' && tela && tela.gaveta && tela.fichaAberta) { ev.stopPropagation(); definirFicha(false); }
    });
    ligarArrastar(chat);
    window.addEventListener('resize', medir);
    if (window.ResizeObserver) {
      tela.observador = new ResizeObserver(medir);
      for (const el of [raiz, $('.estado'), $('#faixa'), $('.lateral')]) if (el) tela.observador.observe(el);
    }
    medir();
    definirFicha(tela.gaveta ? false : prefFicha(), { direto: true });

    if (!S.estado) trocar(lista, carregandoEl('Carregando conversas...'));
    pintarFiltros();
    pintarSemConversa();
    atualizar();
  }

  function sair() {
    cancelarGravacao();
    if (tela) {
      window.removeEventListener('resize', medir);
      if (tela.observador) tela.observador.disconnect();
    }
    soltarPainel();
    if (visor) { if (visor.dlg.open) visor.dlg.close(); visor.dlg.remove(); visor = null; }
    if (tela && tela.filtro) tela.filtro.fechar();
    tela = null;
    itens.clear();
  }

  function soltarPainel() {
    if (painel && painel.observador) painel.observador.disconnect();
    painel = null;
  }

  // medir acerta a altura da tela (ela ocupa o que sobra abaixo da barra de situação)
  // e decide se a ficha cabe ao lado ou vira painel deslizante.
  function medir() {
    if (!tela) return;
    const r = tela.raiz.getBoundingClientRect();
    const topo = Math.max(0, Math.round(r.top + window.scrollY));
    if (topo !== tela.topo) {
      tela.topo = topo;
      tela.raiz.style.setProperty('--cv-topo', topo + 'px');
    }
    const gaveta = r.width > 0 && r.width < LARGURA_GAVETA;
    if (gaveta !== tela.gaveta) {
      tela.gaveta = gaveta;
      tela.raiz.classList.toggle('cv-gaveta', gaveta);
      definirFicha(gaveta ? false : prefFicha(), { direto: true });
    }
  }

  // ---------- Ficha: mostrar e recolher ----------

  // definirFicha mostra ou recolhe a ficha. Com espaço, ela é uma coluna. Sem espaço, é um
  // painel que desliza por cima da conversa (com a biblioteca Motion, ou transição de CSS).
  // Enquanto desliza, a tela leva a classe cv-movendo. Parada, quem manda na posição é o CSS.
  function definirFicha(aberta, opc) {
    if (!tela) return;
    opc = opc || {};
    const { raiz, ficha } = tela;
    const era = tela.fichaAberta;
    tela.fichaAberta = aberta;
    const vez = ++tela.vezFicha;
    const desliza = tela.gaveta && !opc.direto && era !== aberta && !menosMovimento();
    const largura = ficha.offsetWidth || 360;
    const focoDentro = ficha.contains(document.activeElement);
    const parar = () => {
      if (!tela || tela.vezFicha !== vez) return;
      raiz.classList.remove('cv-movendo');
      raiz.classList.toggle('cv-ficha-aberta', aberta);
      if (aberta) ficha.style.removeProperty('transform');
    };
    ficha.inert = !aberta;
    if (aberta) raiz.classList.add('cv-ficha-aberta');
    const c = desliza
      ? mover(ficha, { x: aberta ? [largura, 0] : [0, largura] }, aberta ? { duration: 0.26 } : { duration: 0.2, ease: 'easeIn', manter: true })
      : null;
    if (c) {
      raiz.classList.add('cv-movendo');
      aoTerminar(c, parar);
    } else {
      parar();
    }
    if (aberta && tela.gaveta && !opc.direto) {
      const botaoFechar = ficha.querySelector('.cv-ficha-fechar');
      if (botaoFechar) botaoFechar.focus({ preventScroll: true });
    }
    if (!aberta && focoDentro && painel) painel.botaoFicha.focus({ preventScroll: true });
    pintarBotaoFicha();
  }

  function alternarFicha() {
    if (!tela) return;
    const nova = !tela.fichaAberta;
    if (!tela.gaveta) gravar(CHAVE_FICHA, nova ? '1' : '0');
    definirFicha(nova);
  }

  function pintarBotaoFicha() {
    if (!tela || !painel) return;
    const aberta = tela.fichaAberta;
    painel.botaoFicha.setAttribute('aria-expanded', aberta ? 'true' : 'false');
    painel.botaoFicha.setAttribute('aria-label', aberta ? 'Esconder detalhes do cliente' : 'Mostrar detalhes do cliente');
  }

  // ---------- Filtros, busca e lista ----------

  function filtroAtual() {
    return { situacao: S.filtro === 'todas' ? '' : S.filtro, temp: tempFiltro, etiqueta: etiquetaFiltro };
  }

  function temFiltro() { return S.filtro !== 'todas' || !!tempFiltro || !!etiquetaFiltro; }

  // definirFiltro troca a escolha de um grupo: 'situacao', 'temp' ou 'etiqueta'. Vazio é "todas".
  function definirFiltro(grupo, id) {
    if (grupo === 'situacao') S.filtro = id || 'todas';
    else if (grupo === 'temp') tempFiltro = id || '';
    else etiquetaFiltro = id || '';
    pintarFiltros();
    pintarLista();
  }

  function limparFiltros() {
    S.filtro = 'todas';
    tempFiltro = '';
    etiquetaFiltro = '';
    pintarFiltros();
    pintarLista();
  }

  // verFila mostra só quem está esperando uma pessoa, sem mais nada no caminho.
  function verFila() {
    S.filtro = 'fila';
    tempFiltro = '';
    etiquetaFiltro = '';
    definirBusca('');
  }

  function definirBusca(valor) {
    busca = valor;
    if (tela) {
      if (tela.campoBusca.value !== valor) tela.campoBusca.value = valor;
      tela.limparBusca.hidden = !valor;
    }
    pintarFiltros();
    pintarLista();
  }

  function etiquetasEmUso() {
    if (S.estado && Array.isArray(S.estado.etiquetas)) return S.estado.etiquetas.map(String);
    const todas = new Set();
    for (const l of leads()) for (const e of l.etiquetas || []) todas.add(String(e));
    return Array.from(todas).sort((a, b) => a.localeCompare(b, 'pt-BR'));
  }

  function passaFiltro(l, f) {
    if (f.situacao && (situacaoDe(l) === 'ia' ? 'assistente' : situacaoDe(l)) !== f.situacao) return false;
    if (f.temp && temperatura(l.classificacao) !== f.temp) return false;
    if (f.etiqueta && !(l.etiquetas || []).map(String).includes(f.etiqueta)) return false;
    return true;
  }

  // filtroDaBusca devolve a função que diz se o lead bate com o texto digitado.
  function filtroDaBusca() {
    const termo = semAcento(busca.trim());
    const digitos = busca.replace(/\D/g, '');
    if (!termo) return () => true;
    return (l) => {
      const nomes = semAcento([l.exibir, l.nome, l.nome_whatsapp, l.telefone_bonito].filter(Boolean).join(' '));
      if (nomes.includes(termo)) return true;
      return digitos.length >= 2 && String(l.telefone || '').replace(/\D/g, '').includes(digitos);
    };
  }

  // resumoDoFiltro diz em poucas palavras o que está valendo: "Na fila, Quentes".
  function resumoDoFiltro() {
    const f = filtroAtual();
    const partes = [];
    const situacao = SITUACOES.find((x) => x.id === f.situacao);
    if (f.situacao && situacao) partes.push(situacao.rotulo);
    const temp = TEMPERATURAS_FILTRO.find((x) => x.id === f.temp);
    if (f.temp && temp) partes.push(temp.rotulo);
    if (f.etiqueta) partes.push(f.etiqueta);
    return partes.join(', ');
  }

  function pintarFiltros() {
    if (!tela) return;
    if (etiquetaFiltro && S.estado && !etiquetasEmUso().includes(etiquetaFiltro)) etiquetaFiltro = '';
    const resumo = resumoDoFiltro();
    const ativo = !!resumo;
    mudou(tela, 'resumo', resumo, () => {
      tela.resumoFiltro.textContent = resumo || 'Todas as conversas';
      tela.botaoFiltro.classList.toggle('ativo', ativo);
      tela.botaoFiltro.setAttribute('aria-label', 'Filtrar conversas. ' + (resumo ? 'Mostrando: ' + resumo : 'Mostrando todas as conversas'));
      tela.botaoFiltro.title = resumo ? 'Filtro: ' + resumo : 'Filtrar conversas';
      tela.limparFiltro.hidden = !ativo;
    });
    const esperando = leads().filter(naFila).length;
    mudou(tela, 'fila', String(esperando), () => {
      const estava = tela.avisoFila.hidden;
      tela.avisoFila.hidden = !esperando;
      tela.textoFila.textContent = esperando + ' na fila';
      tela.avisoFila.setAttribute('aria-label', (esperando === 1 ? '1 conversa na fila' : esperando + ' conversas na fila') + '. Ver só a fila');
      if (esperando && estava && tela.marcas.filaPintada) mover(tela.avisoFila, { opacity: [0, 1], y: [-6, 0] }, { duration: 0.2 });
      tela.marcas.filaPintada = true;
    });
    if (tela.painelFiltro) tela.painelFiltro.pintar();
  }

  // montarPainelFiltro preenche o painel do filtro: três grupos, uma escolha em cada um.
  // Cada opção mostra quantas conversas tem, no total. O botão do rodapé diz quantas
  // sobram com tudo junto (os três grupos e a busca).
  function montarPainelFiltro(painelEl, fecharPainel) {
    painelEl.setAttribute('role', 'dialog');
    painelEl.setAttribute('aria-label', 'Filtrar conversas');
    const caixa = h('div', { class: 'sus-rolagem', role: 'listbox', 'aria-label': 'Filtros', 'aria-multiselectable': 'true' });
    const limpar = h('button', {
      type: 'button', class: 'btn cv-btn-linha', tabIndex: -1, 'aria-label': 'Limpar filtros',
      onclick: () => { limparFiltros(); },
    }, 'Limpar');
    const textoVer = h('span');
    const ver = h('button', { type: 'button', class: 'btn cv-btn-verde', tabIndex: -1, onclick: () => fecharPainel(true) }, textoVer);
    let pecas = [];
    let marcaEtiquetas = null;

    function grupo(chave, titulo, opcoes) {
      const el = h('div', { class: 'sus-grupo', role: 'group', 'aria-label': titulo },
        h('div', { class: 'sus-titulo', 'aria-hidden': 'true', text: titulo }));
      const comCor = opcoes.some((o) => o.cor);
      for (const o of opcoes) {
        const item = susItem({ texto: o.rotulo, cor: o.cor }, { role: 'option' }, comCor);
        item.el.addEventListener('click', () => definirFiltro(chave, o.id));
        pecas.push({ chave, id: o.id, item });
        el.appendChild(item.el);
      }
      return el;
    }

    function construir() {
      const etiquetas = etiquetasEmUso();
      marcaEtiquetas = etiquetas.join('\n');
      pecas = [];
      trocar(caixa,
        grupo('situacao', 'Situação', SITUACOES),
        grupo('temp', 'Temperatura', TEMPERATURAS_FILTRO),
        grupo('etiqueta', 'Etiquetas', [{ id: '', rotulo: 'Todas' }].concat(etiquetas.map((e) => ({ id: e, rotulo: e })))),
        typeof ETIQUETAS !== 'undefined' && ACESSO.pode('admin')
          ? h('button', {
            type: 'button', class: 'btn btn-fantasma cv-gerenciar-etq', tabIndex: -1,
            onclick: () => { fecharPainel(false); ETIQUETAS.abrir(); },
          }, icone('lapis'), h('span', { text: 'Gerenciar etiquetas' }))
          : null);
    }

    function pintar() {
      if (marcaEtiquetas !== etiquetasEmUso().join('\n')) {
        // Apareceu ou sumiu uma etiqueta: a lista é refeita e o item ativo se mantém.
        if (marcaEtiquetas === null) construir();
        else { tela.filtro.refazer(); return; }
      }
      const f = filtroAtual();
      const todos = leads();
      for (const p of pecas) {
        const so = { situacao: '', temp: '', etiqueta: '' };
        so[p.chave] = p.id;
        p.item.marcar(f[p.chave] === p.id);
        p.item.contar(todos.filter((l) => passaFiltro(l, so)).length);
      }
      const n = leadsVisiveis().length;
      textoVer.textContent = n === 0 ? 'Nenhuma conversa' : (n === 1 ? 'Ver 1 conversa' : 'Ver ' + n + ' conversas');
      limpar.hidden = !temFiltro();
    }

    painelEl.append(caixa, h('div', { class: 'cv-sus-pe' }, limpar, ver));
    tela.painelFiltro = { pintar };
    pintar();
  }

  function leadsVisiveis() {
    const f = filtroAtual();
    const lista = leads().filter(filtroDaBusca()).filter((l) => passaFiltro(l, f));
    // Quem está na fila vem primeiro. O resto segue a ordem da API (mais recente antes).
    return lista.filter(naFila).concat(lista.filter((l) => !naFila(l)));
  }

  // previaDe prepara a última mensagem para a lista: ícone e palavra quando é mídia.
  function previaDe(l) {
    let texto = String(l.previa || '');
    let tipo = MIDIAS[l.previa_tipo] ? l.previa_tipo : '';
    const marca = /^\[(imagem|áudio|audio|vídeo|video|arquivo|documento)\]\s*/i.exec(texto);
    if (marca) {
      if (!tipo) tipo = MARCAS_PREVIA[marca[1].toLowerCase()] || '';
      texto = texto.slice(marca[0].length);
    }
    if (tipo && !texto) texto = MIDIAS[tipo].palavra;
    const quem = l.previa_de === 'humano' ? 'Equipe: ' : (l.previa_de === 'atendente' ? 'Assistente: ' : '');
    return { icone: tipo ? MIDIAS[tipo].icone : '', quem, texto };
  }

  function criarItem(id) {
    const p = {
      marcas: {},
      foto: h('span', { class: 'cv-item-foto' }),
      nome: h('span', { class: 'cv-item-nome' }),
      hora: h('span', { class: 'cv-item-hora' }),
      previa: h('span', { class: 'cv-item-previa' }),
      bolinha: h('span', { class: 'cv-bolinha', hidden: true }),
      meta: h('span', { class: 'cv-item-meta', hidden: true }),
      motivo: h('span', { class: 'cv-item-motivo', hidden: true }),
      leitor: h('span', { class: 'cv-so-leitor' }),
    };
    p.el = h('button', { type: 'button', class: 'cv-item', onclick: () => abrir(id) },
      p.foto,
      h('span', { class: 'cv-item-corpo' },
        h('span', { class: 'cv-item-linha' }, p.nome, p.hora),
        h('span', { class: 'cv-item-linha' }, p.previa, p.bolinha),
        p.meta, p.motivo, p.leitor));
    return p;
  }

  function atualizarItem(p, l) {
    const aberto = S.aberto === l.id;
    const naoLidas = Math.max(0, Number(l.nao_lidas) || 0);
    const temp = temperatura(l.classificacao);
    const fila = naFila(l);
    const classe = 'cv-item' + (fila ? ' na-fila' : '') + (aberto ? ' aberto' : '') + (naoLidas ? ' nao-lida' : '');
    if (p.el.className !== classe) p.el.className = classe;
    if (p.el.dataset.temp !== temp) p.el.dataset.temp = temp;
    if (aberto) p.el.setAttribute('aria-current', 'true');
    else p.el.removeAttribute('aria-current');

    const nome = nomeDe(l);
    mudou(p, 'foto', chaveFoto(l), () => trocar(p.foto, avatar(l, 'cv-avatar-m')));
    mudou(p, 'nome', nome, () => { p.nome.textContent = nome; });
    const hora = tempoRelativo(l.ultima_msg);
    mudou(p, 'hora', hora, () => { p.hora.textContent = hora; });
    const pv = previaDe(l);
    mudou(p, 'previa', [pv.icone, pv.quem, pv.texto].join('|'), () => {
      trocar(p.previa, pv.icone ? ic(pv.icone) : null, pv.quem + pv.texto);
    });
    mudou(p, 'lidas', String(naoLidas), () => {
      p.bolinha.hidden = !naoLidas;
      trocar(p.bolinha, naoLidas > 99 ? '99+' : String(naoLidas),
        h('span', { class: 'cv-so-leitor', text: naoLidas === 1 ? ' mensagem não lida' : ' mensagens não lidas' }));
    });

    const etiquetas = (l.etiquetas || []).map(String);
    const atendendo = emAtendimento(l);
    mudou(p, 'meta', [l.status, l.atendente || '', etiquetas.join('\n')].join('|'), () => {
      const partes = [];
      if (fila) partes.push(h('span', { class: 'cv-selo cv-selo-fila' }, icone('relogio'), 'Na fila'));
      else if (atendendo) partes.push(h('span', { class: 'cv-selo cv-selo-atend' }, ic('fone'), l.atendente ? 'Com ' + l.atendente : 'Em atendimento'));
      for (const e of etiquetas.slice(0, 2)) partes.push(h('span', { class: 'cv-tag', text: e }));
      if (etiquetas.length > 2) {
        partes.push(h('span', { class: 'cv-tag cv-tag-mais', title: etiquetas.slice(2).join(', ') },
          '+' + (etiquetas.length - 2), h('span', { class: 'cv-so-leitor', text: ' etiquetas' })));
      }
      p.meta.hidden = !partes.length;
      trocar(p.meta, partes);
    });
    const motivo = fila && l.motivo ? 'Motivo: ' + l.motivo : '';
    mudou(p, 'motivo', motivo, () => { p.motivo.hidden = !motivo; p.motivo.textContent = motivo; });
    const leitor = ' Lead ' + TEMPERATURAS[temp].toLowerCase() + '.';
    mudou(p, 'leitor', leitor, () => { p.leitor.textContent = leitor; });
  }

  // encaixar deixa os filhos da caixa na ordem pedida, mexendo só no que saiu do lugar.
  function encaixar(caixa, ordem) {
    let ref = caixa.firstChild;
    for (const el of ordem) {
      if (el === ref) ref = ref.nextSibling;
      else caixa.insertBefore(el, ref);
    }
    while (ref) {
      const proximo = ref.nextSibling;
      ref.remove();
      ref = proximo;
    }
  }

  function pintarLista() {
    if (!tela || !S.estado) return;
    const visiveis = leadsVisiveis();
    const vivos = new Set();
    const ordem = [];
    for (const l of visiveis) {
      vivos.add(l.id);
      let p = itens.get(l.id);
      if (!p) { p = criarItem(l.id); itens.set(l.id, p); }
      atualizarItem(p, l);
      ordem.push(p.el);
    }
    for (const [id, p] of Array.from(itens)) {
      if (!vivos.has(id)) { p.el.remove(); itens.delete(id); }
    }
    if (!ordem.length) {
      const filtrando = temFiltro() || !!busca.trim();
      const marca = [S.filtro, tempFiltro, etiquetaFiltro, busca.trim()].join('|');
      if (tela.marcaNota !== marca) {
        tela.marcaNota = marca;
        const texto = busca.trim()
          ? 'Nada encontrado para "' + cortarTexto(busca.trim(), 40) + '"' + (temFiltro() ? ' com esse filtro.' : '.')
          : (temFiltro() ? 'Nenhuma conversa com esse filtro.' : 'Nenhuma conversa aqui agora.');
        trocar(tela.lista, h('div', { class: 'cv-nota' },
          h('p', { text: texto }),
          filtrando ? h('button', {
            type: 'button', class: 'btn cv-btn-linha',
            onclick: () => { S.filtro = 'todas'; tempFiltro = ''; etiquetaFiltro = ''; definirBusca(''); if (tela) tela.botaoFiltro.focus(); },
          }, 'Ver todas as conversas') : null));
      }
      return;
    }
    tela.marcaNota = null;
    const rolagem = tela.lista.scrollTop;
    encaixar(tela.lista, ordem);
    if (tela.lista.scrollTop !== rolagem) tela.lista.scrollTop = rolagem;
  }

  // atualizar é chamado pela casca a cada busca de /api/estado.
  function atualizar() {
    if (!tela) return;
    medir();
    if (!S.estado) return;
    pintarFiltros();

    const semLeads = leads().length === 0;
    tela.raiz.classList.toggle('cv-sem-leads', semLeads);
    if (S.aberto && !resumoDe(S.aberto)) {
      // A conversa aberta sumiu da lista (foi apagada).
      S.aberto = null;
      S.lead = null;
      soltarPainel();
    }
    if (S.aberto && !painel) abrir(S.aberto);
    if (!S.aberto && marcaDoVazio() !== tela.marcaVazio) pintarSemConversa();

    pintarLista();
    if (S.aberto && painel) {
      const resumo = resumoDe(S.aberto);
      if (resumo) pintarTopo(resumo);
      buscarLead();
    }
  }

  // ---------- Sem conversa aberta ----------

  function marcaDoVazio() {
    const e = S.estado;
    if (!e) return 'carregando';
    return leads().length ? 'escolha' : ['vazio', e.status, e.ligado ? 1 : 0].join('|');
  }

  function pintarSemConversa() {
    if (!tela) return;
    cancelarGravacao();
    soltarPainel();
    tela.raiz.classList.remove('cv-tem-aberta');
    trocar(tela.ficha);
    tela.marcaVazio = marcaDoVazio();
    const e = S.estado;
    if (e && !leads().length) {
      const ligadoAoWhats = e.status === 'conectado';
      let texto = 'Com o atendente ligado, mande um "Oi" de outro celular para o número do negócio. A conversa aparece aqui em poucos segundos.';
      if (!ligadoAoWhats) texto = 'O WhatsApp ainda não está conectado. Conecte nos Ajustes e depois mande um "Oi" de outro celular para o número do negócio.';
      else if (!e.ligado) texto = 'O atendente está pausado. Ligue a chave lá em cima e mande um "Oi" de outro celular para o número do negócio.';
      tela.chat.classList.add('cv-chat-painel');
      trocar(tela.chat, vazio('conversas', 'Nenhuma conversa ainda', texto,
        ligadoAoWhats ? null : h('a', { class: 'btn btn-primario', href: '#ajustes' }, icone('celular'), h('span', { text: 'Conectar o WhatsApp' })),
        h('a', { class: 'btn btn-suave', href: '#treinar' }, icone('treinar'), h('span', { text: 'Testar sem WhatsApp' }))));
      return;
    }
    tela.chat.classList.remove('cv-chat-painel');
    trocar(tela.chat, h('div', { class: 'cv-intro' },
      h('span', { class: 'cv-intro-icone' }, icone('conversas', 'ic-grande')),
      h('h2', { text: 'Escolha uma conversa' }),
      h('p', { text: 'Quem está na fila aparece primeiro na lista. A faixa colorida ao lado de cada nome mostra a temperatura do lead.' })));
  }

  // ---------- Abrir e fechar uma conversa ----------

  function abrir(id) {
    if (!tela) return;
    if (painel && painel.id === id) {
      tela.raiz.classList.add('cv-tem-aberta');
      return;
    }
    cancelarGravacao();
    soltarPainel();
    S.aberto = id;
    S.lead = null;
    S.agendamentosLead = [];
    montarConversa(id);
    tela.raiz.classList.add('cv-tem-aberta');
    if (tela.gaveta) definirFicha(false, { direto: true });
    pintarLista();
    buscarLead();
  }

  function fechar() {
    const id = S.aberto;
    S.aberto = null;
    S.lead = null;
    if (!tela) return;
    if (tela.gaveta) definirFicha(false, { direto: true });
    pintarSemConversa();
    pintarLista();
    const item = id && itens.get(id);
    if (item) item.el.focus({ preventScroll: true });
  }

  function montarConversa(id) {
    const r = rascunho(id);
    const p = { id, marcas: {}, nos: new Map(), vistas: new Map(), temAnteriores: null, pintou: false, noFim: true, irAoFim: false, novas: 0, enviando: false, pode: false, situacao: 'ia' };

    // Cabeçalho
    p.foto = h('span', { class: 'cv-cab-foto' });
    p.nome = h('h2', { class: 'cv-cab-nome' });
    p.sub = h('div', { class: 'cv-cab-sub' });
    p.botaoFicha = h('button', { type: 'button', class: 'cv-detalhes', 'aria-controls': 'cv-ficha', onclick: alternarFicha },
      ic('painel'), h('span', { text: 'Detalhes' }));
    const cabecalho = h('header', { class: 'cv-cab' },
      h('button', { type: 'button', class: 'cv-ib cv-voltar', 'aria-label': 'Voltar para a lista', onclick: fechar }, icone('voltar')),
      p.foto, h('div', { class: 'cv-cab-quem' }, p.nome, p.sub), p.botaoFicha);
    p.faixa = h('div', { class: 'cv-situacao' });

    // Mensagens
    p.msgs = h('div', { class: 'cv-msgs', role: 'group', tabindex: '0', 'aria-label': 'Mensagens da conversa' },
      carregandoEl('Abrindo a conversa...'));
    p.textoNovas = h('span');
    p.botaoNovas = h('button', { type: 'button', class: 'cv-novas', hidden: true, onclick: () => irAoFim(true) }, ic('descer'), p.textoNovas);
    p.textoSoltar = h('span', { text: 'Solte aqui para anexar' });
    p.soltar = h('div', { class: 'cv-soltar', hidden: true }, icone('subir', 'ic-grande'), p.textoSoltar);
    p.msgs.addEventListener('scroll', () => {
      p.noFim = distanciaDoFim(p.msgs) < 48;
      if (p.noFim && p.novas) { p.novas = 0; p.botaoNovas.hidden = true; }
    }, { passive: true });
    if (window.ResizeObserver) {
      // Quando o campo de escrever cresce, a conversa continua colada no fim.
      p.observador = new ResizeObserver(() => { if (p.noFim) p.msgs.scrollTop = p.msgs.scrollHeight; });
      p.observador.observe(p.msgs);
    }

    // Campo de escrever
    p.aviso = h('div', { class: 'cv-aviso', role: 'alert', hidden: true });
    p.anexos = h('div', { class: 'cv-anexos', hidden: true, role: 'group', 'aria-label': 'Arquivos para enviar' });
    p.arquivo = h('input', { type: 'file', multiple: true, hidden: true, tabIndex: -1, 'aria-hidden': 'true' });
    p.arquivo.addEventListener('change', () => { adicionarArquivos(p.arquivo.files); p.arquivo.value = ''; });
    p.botaoAnexo = h('button', { type: 'button', class: 'cv-ib', 'aria-label': 'Anexar arquivo', title: 'Anexar arquivo', onclick: () => p.arquivo.click() }, ic('clipe'));
    p.texto = h('textarea', { class: 'cv-campo', rows: 1, placeholder: 'Escreva uma mensagem', 'aria-label': 'Mensagem para o cliente', maxLength: 4000 });
    p.texto.value = r.texto;
    p.texto.addEventListener('keydown', (ev) => {
      if (ev.key === 'Enter' && !ev.shiftKey && !ev.isComposing) { ev.preventDefault(); enviar(); }
    });
    p.texto.addEventListener('input', () => { r.texto = p.texto.value; ajustarCampo(p); });
    p.texto.addEventListener('paste', (ev) => {
      const colados = Array.from((ev.clipboardData && ev.clipboardData.files) || []);
      if (!colados.length) return;
      ev.preventDefault();
      adicionarArquivos(colados.map(nomearColado));
    });
    p.botaoVoz = h('button', { type: 'button', class: 'cv-ib', 'aria-label': 'Gravar mensagem de voz', title: 'Gravar mensagem de voz', onclick: comecarGravacao }, ic('microfone'));
    p.botaoEnviar = h('button', { type: 'button', class: 'cv-enviar', 'aria-label': 'Enviar mensagem', title: 'Enviar', onclick: enviar }, icone('enviar'));
    p.compor = h('div', { class: 'cv-compor', hidden: true }, p.botaoAnexo, p.arquivo, p.texto, p.botaoVoz, p.botaoEnviar);

    p.cronometro = h('span', { class: 'cv-cronometro', role: 'timer', text: '0:00' });
    p.gravando = h('div', { class: 'cv-gravando', hidden: true },
      h('button', { type: 'button', class: 'btn cv-btn-linha', onclick: () => terminarGravacao(false) }, icone('lixo'), h('span', { text: 'Cancelar gravação' })),
      h('span', { class: 'cv-gravando-meio' }, h('span', { class: 'cv-ponto-gravando', 'aria-hidden': 'true' }), p.cronometro, h('span', { class: 'cv-gravando-texto', text: 'Gravando' })),
      h('button', { type: 'button', class: 'btn cv-btn-verde', onclick: () => terminarGravacao(true) }, icone('enviar'), h('span', { text: 'Enviar gravação' })));
    p.travado = h('div', { class: 'cv-travado' });
    p.vivo = h('div', { class: 'cv-so-leitor', 'aria-live': 'polite' });

    tela.chat.classList.remove('cv-chat-painel');
    trocar(tela.chat, cabecalho, p.faixa,
      h('div', { class: 'cv-corpo' }, p.msgs, p.botaoNovas, p.soltar),
      h('div', { class: 'cv-pe' }, p.aviso, p.anexos, p.compor, p.gravando, p.travado),
      p.vivo);
    painel = p;
    montarFicha(p);
    pintarBotaoFicha();

    const resumo = resumoDe(id);
    if (resumo) {
      pintarTopo(resumo);
      pintarFicha(resumo, null);
    } else {
      pintarCompor();
    }
    pintarAnexos();
    ajustarCampo(p);
  }

  // buscarLead traz a conversa aberta. Nunca roda duas buscas ao mesmo tempo.
  function buscarLead() {
    if (!buscaEmCurso) {
      buscaEmCurso = buscarLeadAgora().finally(() => { buscaEmCurso = null; });
      return buscaEmCurso;
    }
    if (!buscaEsperando) {
      buscaEsperando = buscaEmCurso.then(() => { buscaEsperando = null; return buscarLead(); });
    }
    return buscaEsperando;
  }

  async function buscarLeadAgora() {
    const id = S.aberto;
    const p = painel;
    if (!tela || !id || !p || p.id !== id) return;
    let r;
    try {
      r = await api.get('/api/lead?id=' + encodeURIComponent(id));
    } catch (e) {
      if (!tela || painel !== p) return;
      if (S.estado && !S.semConexao && !resumoDe(id)) fechar();
      else if (!p.pintou) trocar(p.msgs, h('p', { class: 'cv-nota-msgs', text: 'Não consegui abrir a conversa. Vou tentar de novo em instantes.' }));
      return;
    }
    if (!tela || painel !== p || S.aberto !== id || !r || !r.lead) return;
    S.lead = r.lead;
    S.agendamentosLead = r.agendamentos || [];
    // Só a primeira resposta diz se existe conversa mais antiga; depois vale o que já foi buscado.
    if (p.temAnteriores === null) p.temAnteriores = !!r.tem_anteriores;
    pintarTopo(r.lead);
    pintarMensagens(r.lead.historico || []);
    pintarFicha(r.lead, S.agendamentosLead);
  }

  // ---------- Cabeçalho, situação e trava do campo ----------

  function pintarTopo(l) {
    const p = painel;
    if (!p) return;
    const nome = nomeDe(l);
    mudou(p, 'foto', chaveFoto(l), () => trocar(p.foto, avatar(l, 'cv-avatar-p')));
    mudou(p, 'nome', nome, () => { p.nome.textContent = nome; });
    const sub = l.telefone_bonito && l.telefone_bonito !== nome ? l.telefone_bonito : '';
    mudou(p, 'sub', sub, () => { p.sub.textContent = sub; p.sub.hidden = !sub; });

    const situacao = situacaoDe(l);
    p.situacao = situacao;
    mudou(p, 'faixa', [situacao, l.atendente || '', l.motivo || '', euMesmo()].join('|'), () => {
      p.faixa.dataset.situacao = situacao;
      // O texto fica em até duas linhas; inteiro, ele aparece ao passar o mouse e na ficha.
      const texto = (desenho, forte, resto) => h('div', { class: 'cv-situacao-texto' }, desenho,
        h('span', { title: resto ? forte + ' ' + resto : null }, h('strong', { text: forte }),
          resto ? h('span', { class: 'cv-situacao-resto', text: ' ' + resto }) : null));
      if (situacao === 'fila') {
        trocar(p.faixa,
          texto(icone('relogio'), 'Na fila.', l.motivo ? 'Motivo: ' + l.motivo : 'Esperando uma pessoa da equipe.'),
          botao('Iniciar atendimento', { classe: 'btn-primario cv-acao', icone: 'assumir', aoClicar: () => iniciarAtendimento(l.id) }));
      } else if (situacao === 'atendimento') {
        trocar(p.faixa,
          texto(ic('fone'), l.atendente ? (l.atendente === euMesmo() ? 'Você está atendendo' : 'Atendendo: ' + l.atendente) : 'Em atendimento'),
          botao('Encerrar e devolver ao assistente', { classe: 'cv-btn-linha cv-acao', aoClicar: () => encerrarAtendimento(l.id) }));
      } else {
        trocar(p.faixa, texto(icone('assistente'), 'Assistente respondendo.', 'Ele cuida desta conversa sozinho.'));
      }
    });
    pintarCompor();
  }

  function pintarCompor() {
    const p = painel;
    if (!p) return;
    const ligado = conectado();
    const pode = p.situacao === 'atendimento' && ligado;
    p.pode = pode;
    if (!pode && gravacao && gravacao.id === p.id) cancelarGravacao();
    const gravandoAqui = !!gravacao && gravacao.id === p.id;
    p.compor.hidden = !pode || gravandoAqui;
    p.gravando.hidden = !gravandoAqui;
    p.anexos.hidden = !pode || gravandoAqui || !rascunho(p.id).anexos.length;
    p.travado.hidden = pode;
    if (pode) p.aviso.hidden = !p.aviso.firstChild;
    else p.aviso.hidden = true;
    mudou(p, 'trava', [p.situacao, ligado, pode].join('|'), () => {
      if (pode) { trocar(p.travado); ajustarCampo(p); return; }
      if (p.situacao === 'ia') {
        trocar(p.travado, ic('cadeado'),
          h('p', { text: 'O assistente está respondendo. Para escrever, assuma a conversa.' }),
          botao('Assumir conversa', { classe: 'cv-btn-linha', icone: 'assumir', aoClicar: () => iniciarAtendimento(p.id) }));
      } else if (p.situacao === 'fila') {
        trocar(p.travado, ic('cadeado'), h('p', { text: 'Clique em Iniciar atendimento para responder.' }));
      } else {
        trocar(p.travado, icone('alerta'),
          h('p', { text: 'O WhatsApp não está conectado. Conecte nos Ajustes para responder por aqui.' }),
          h('a', { class: 'btn cv-btn-linha', href: '#ajustes' }, h('span', { text: 'Abrir os Ajustes' })));
      }
    });
  }

  function avisarCompor(texto) {
    const p = painel;
    if (!p) { toast(texto, 'erro'); return; }
    trocar(p.aviso, icone('alerta'), h('p', { text: texto }),
      h('button', { type: 'button', class: 'cv-ib cv-ib-p', 'aria-label': 'Fechar aviso', onclick: limparAviso }, icone('fechar')));
    p.aviso.hidden = false;
  }

  function limparAviso() {
    if (!painel) return;
    trocar(painel.aviso);
    painel.aviso.hidden = true;
  }

  // ---------- Mensagens ----------

  function distanciaDoFim(cx) { return cx.scrollHeight - cx.scrollTop - cx.clientHeight; }

  function irAoFim(suave) {
    const p = painel;
    if (!p) return;
    p.novas = 0;
    p.botaoNovas.hidden = true;
    p.noFim = true;
    if (suave && !menosMovimento() && p.msgs.scrollTo) p.msgs.scrollTo({ top: p.msgs.scrollHeight, behavior: 'smooth' });
    else p.msgs.scrollTop = p.msgs.scrollHeight;
  }

  function urlMidia(m) { return '/api/midia?id=' + encodeURIComponent(m.midia); }

  function textoDoEvento(m) {
    if (m.texto && String(m.texto).trim()) return String(m.texto);
    const quem = m.autor || 'A equipe';
    if (m.evento === 'transbordo') return 'O assistente transferiu a conversa para a equipe';
    if (m.evento === 'assumiu') return quem + ' iniciou o atendimento';
    if (m.evento === 'devolveu') return quem + ' devolveu a conversa ao assistente';
    if (m.evento === 'agendou') return 'Horário marcado';
    if (m.evento === 'cancelou') return 'Horário cancelado';
    return 'Registro interno';
  }

  function ehEvento(m) { return m.de === 'sistema' || m.tipo === 'evento'; }

  function elMidia(m, tipo, quem) {
    const url = urlMidia(m);
    if (tipo === 'imagem') {
      const descricao = 'Imagem enviada por ' + quem;
      const img = h('img', { src: url, alt: descricao, loading: 'lazy' });
      const b = h('button', { type: 'button', class: 'cv-foto', 'aria-label': 'Ampliar imagem enviada por ' + quem, onclick: () => abrirImagem(url, descricao) }, img);
      img.addEventListener('error', () => b.replaceWith(h('p', { class: 'cv-texto cv-faltou', text: 'Não consegui carregar a imagem.' })));
      return b;
    }
    if (tipo === 'audio') return h('audio', { class: 'cv-audio', controls: true, preload: 'none', src: url, 'aria-label': 'Áudio enviado por ' + quem });
    if (tipo === 'video') return h('video', { class: 'cv-video', controls: true, preload: 'metadata', playsinline: true, src: url, 'aria-label': 'Vídeo enviado por ' + quem });
    const nome = m.nome || 'Arquivo';
    const extensao = /\.([a-z0-9]{1,5})$/i.exec(nome);
    const detalhes = [extensao ? extensao[1].toUpperCase() : '', m.tamanho ? tamanhoBonito(m.tamanho) : ''].filter(Boolean).join(', ');
    return h('div', { class: 'cv-doc' },
      h('span', { class: 'cv-doc-icone' }, icone('arquivo')),
      h('span', { class: 'cv-doc-texto' },
        h('span', { class: 'cv-doc-nome', text: nome }),
        detalhes ? h('span', { class: 'cv-doc-detalhes', text: detalhes }) : null),
      h('a', { class: 'cv-doc-abrir', href: url, target: '_blank', rel: 'noopener', 'aria-label': 'Abrir ' + nome + ' em outra aba' }, 'Abrir'));
  }

  function elMensagem(m, nomeCliente) {
    if (ehEvento(m)) {
      return h('div', { class: 'cv-evento' },
        h('span', { text: textoDoEvento(m) }),
        h('time', { datetime: m.hora, text: horaCurta(m.hora) }));
    }
    const doNegocio = m.de !== 'cliente';
    // Tipo que a tela não conhece, mas que trouxe arquivo, aparece como documento.
    const tipo = MIDIAS[m.tipo] ? m.tipo : (m.tipo && m.tipo !== 'texto' && m.midia ? 'documento' : 'texto');
    const temMidia = tipo !== 'texto' && !!m.midia;
    const quem = doNegocio ? (m.de === 'atendente' ? 'Assistente' : (m.autor || 'Equipe')) : nomeCliente;
    const bolha = h('div', { class: 'cv-bolha ' + (doNegocio ? 'cv-enviada' : 'cv-recebida') });
    if (doNegocio) {
      bolha.appendChild(h('span', { class: 'cv-autor ' + (m.de === 'atendente' ? 'cv-autor-ia' : 'cv-autor-equipe') },
        icone(m.de === 'atendente' ? 'assistente' : 'pessoa'), quem));
    }
    if (temMidia) bolha.appendChild(elMidia(m, tipo, quem));
    const texto = m.texto == null ? '' : String(m.texto);
    if (texto.trim()) {
      bolha.appendChild(h('p', { class: 'cv-texto', text: texto }));
    } else if (!temMidia) {
      // Mídia que não pôde ser baixada, sem legenda: a bolha diz o que era.
      const falta = tipo !== 'texto' ? MIDIAS[tipo].falta + (m.nome ? ': ' + m.nome : '') : 'Mensagem sem texto';
      bolha.appendChild(h('p', { class: 'cv-texto cv-faltou', text: falta }));
    }
    bolha.appendChild(h('time', { class: 'cv-hora', datetime: m.hora, text: horaCurta(m.hora) }));
    return h('div', { class: 'cv-linha ' + (doNegocio ? 'cv-linha-enviada' : 'cv-linha-recebida') }, bolha);
  }

  // entradasDe transforma o histórico em uma lista de peças com chave: separadores de dia e mensagens.
  function entradasDe(historico, nomeCliente) {
    const lista = [];
    const usadas = new Map();
    let diaAnterior = '';
    for (const m of historico) {
      if (m.evento === 'etiqueta') continue; // mudança de etiqueta: só no histórico interno
      const d = new Date(m.hora);
      const dia = isNaN(d) ? '' : d.getFullYear() + '-' + (d.getMonth() + 1) + '-' + d.getDate();
      if (dia && dia !== diaAnterior) {
        diaAnterior = dia;
        const rotulo = rotuloDia(d);
        lista.push({ chave: 'dia|' + dia, marca: rotulo, criar: () => h('div', { class: 'cv-dia' }, h('span', { text: rotulo })) });
      }
      let chave = m.id ? 'id|' + m.id : ['m', m.de, m.hora, m.tipo || ''].join('|');
      const vezes = (usadas.get(chave) || 0) + 1;
      usadas.set(chave, vezes);
      if (vezes > 1) chave += '#' + vezes;
      lista.push({
        chave, msg: m,
        marca: JSON.stringify([m.de, m.texto, m.hora, m.tipo, m.midia, m.nome, m.mime, m.tamanho, m.autor, m.evento, nomeCliente]),
        criar: () => elMensagem(m, nomeCliente),
      });
    }
    if (!lista.length) {
      lista.push({ chave: 'vazio', marca: '', criar: () => h('p', { class: 'cv-nota-msgs', text: 'Ainda sem mensagens nesta conversa.' }) });
    }
    return lista;
  }

  function entrarMensagem(el) {
    if (menosMovimento()) return;
    if (mover(el, { opacity: [0, 1], y: [10, 0] }, { duration: 0.22 })) return;
    el.classList.add('cv-entrando');
    el.addEventListener('animationend', () => el.classList.remove('cv-entrando'), { once: true });
  }

  function resumoParaLeitor(m) {
    const texto = m.texto && String(m.texto).trim();
    if (texto) return cortarTexto(texto, 140);
    return MIDIAS[m.tipo] ? MIDIAS[m.tipo].palavra : 'mensagem';
  }

  // juntarHistorico soma ao que veio agora as mensagens mais antigas que já passaram
  // pela tela. O programa manda só as últimas; o resto fica no banco de dados.
  function juntarHistorico(p, atual) {
    const primeiroN = atual.length && atual[0].n ? atual[0].n : 0;
    if (!primeiroN) return atual;
    for (const m of atual) if (m.n) p.vistas.set(m.n, m);
    const antes = [];
    for (const [n, m] of p.vistas) if (n < primeiroN) antes.push(m);
    antes.sort((a, b) => a.n - b.n);
    return antes.length ? antes.concat(atual) : atual;
  }

  // buscarAnteriores traz do banco o trecho anterior da conversa e mantém a leitura no lugar.
  async function buscarAnteriores(p) {
    let menor = 0;
    for (const n of p.vistas.keys()) if (!menor || n < menor) menor = n;
    if (!menor) return;
    const r = await api.get('/api/lead/historico?id=' + encodeURIComponent(p.id) + '&antes=' + menor);
    if (!tela || painel !== p) return;
    for (const m of r.mensagens || []) if (m.n) p.vistas.set(m.n, m);
    p.temAnteriores = !!r.tem_mais;
    const cx = p.msgs;
    const ateOFim = cx.scrollHeight - cx.scrollTop;
    pintarMensagens((S.lead && S.lead.historico) || []);
    cx.scrollTop = cx.scrollHeight - ateOFim;
  }

  function pintarMensagens(historico) {
    const p = painel;
    const cx = p.msgs;
    const primeira = !p.pintou;
    // Mede na hora: só acompanha o fim se a pessoa está mesmo no fim da conversa.
    const noFim = primeira || p.irAoFim || distanciaDoFim(cx) < 48;
    const entradas = entradasDe(juntarHistorico(p, historico), nomeDe(S.lead || resumoDe(p.id)));
    if (p.temAnteriores) {
      entradas.unshift({
        chave: 'anteriores', marca: '',
        criar: () => {
          const b = h('button', { type: 'button', class: 'btn btn-suave' }, ic('historico'), h('span', { text: 'Ver mensagens anteriores' }));
          b.addEventListener('click', () => ocupar(b, () => buscarAnteriores(p)));
          return h('div', { class: 'cv-anteriores' }, b);
        },
      });
    }

    // Sai o que não existe mais (o programa guarda só as mensagens mais recentes).
    const vivas = new Set(entradas.map((e) => e.chave));
    for (const [chave, no] of Array.from(p.nos)) {
      if (!vivas.has(chave)) { no.el.remove(); p.nos.delete(chave); }
    }
    const ordem = [];
    const novas = [];
    for (const e of entradas) {
      let no = p.nos.get(e.chave);
      if (!no) {
        no = { el: e.criar(), marca: e.marca };
        p.nos.set(e.chave, no);
        if (!primeira && e.msg) novas.push({ el: no.el, msg: e.msg });
      } else if (no.marca !== e.marca) {
        // Mudou (por exemplo, o arquivo terminou de baixar): troca só esta peça.
        const el = e.criar();
        if (no.el.parentNode === cx) no.el.replaceWith(el);
        no.el = el;
        no.marca = e.marca;
      }
      ordem.push(no.el);
    }
    encaixar(cx, ordem);
    p.pintou = true;
    p.irAoFim = false;

    for (const n of novas) entrarMensagem(n.el);
    if (noFim) {
      cx.scrollTop = cx.scrollHeight;
      p.noFim = true;
      p.novas = 0;
      p.botaoNovas.hidden = true;
    } else if (novas.length) {
      p.novas += novas.length;
      p.textoNovas.textContent = p.novas === 1 ? '1 mensagem nova' : p.novas + ' mensagens novas';
      p.botaoNovas.hidden = false;
    }
    const doCliente = novas.filter((n) => n.msg.de === 'cliente');
    if (doCliente.length) {
      const ultima = doCliente[doCliente.length - 1].msg;
      p.vivo.textContent = 'Nova mensagem de ' + nomeDe(S.lead || resumoDe(p.id)) + ': ' + resumoParaLeitor(ultima);
    }
  }

  function abrirImagem(url, descricao) {
    if (!visor) {
      const img = h('img', { class: 'cv-visor-img', alt: '' });
      const link = h('a', { class: 'btn cv-visor-botao', target: '_blank', rel: 'noopener' }, h('span', { text: 'Abrir em outra aba' }));
      const dlg = h('dialog', { class: 'cv-visor', 'aria-label': 'Imagem ampliada' });
      por(dlg, [
        h('div', { class: 'cv-visor-barra' }, link,
          h('button', { type: 'button', class: 'btn cv-visor-botao', onclick: () => dlg.close() }, icone('fechar'), h('span', { text: 'Fechar' }))),
        img]);
      dlg.addEventListener('click', (ev) => { if (ev.target === dlg) dlg.close(); });
      document.body.appendChild(dlg);
      visor = { dlg, img, link };
    }
    visor.img.src = url;
    visor.img.alt = descricao;
    visor.link.href = url;
    if (!visor.dlg.open) visor.dlg.showModal();
    if (!menosMovimento()) mover(visor.img, { opacity: [0, 1], scale: [0.97, 1] }, { duration: 0.18 });
  }

  // ---------- Campo de escrever ----------

  function ajustarCampo(p) {
    const campoTexto = p.texto;
    campoTexto.style.height = 'auto';
    if (campoTexto.offsetParent === null) return;
    campoTexto.style.height = Math.min(campoTexto.scrollHeight + 2, 150) + 'px';
  }

  function nomearColado(arq) {
    if (arq.name && arq.name !== 'image.png') return arq;
    const extensao = (arq.type.split('/')[1] || 'png').replace(/[^a-z0-9]/gi, '').slice(0, 5) || 'png';
    try { return new File([arq], 'imagem-colada-' + carimbo() + '.' + extensao, { type: arq.type }); } catch (e) { return arq; }
  }

  function carimbo() {
    const d = new Date();
    return '' + d.getFullYear() + doisDigitos(d.getMonth() + 1) + doisDigitos(d.getDate()) + '-' +
      doisDigitos(d.getHours()) + doisDigitos(d.getMinutes()) + doisDigitos(d.getSeconds());
  }

  function adicionarArquivos(lista) {
    const p = painel;
    if (!p) return;
    if (!p.pode) { toast('Inicie o atendimento para enviar arquivos.', 'erro'); return; }
    const r = rascunho(p.id);
    for (const arq of Array.from(lista || [])) {
      if (r.anexos.length >= MAX_ANEXOS) { toast('Dá para enviar até ' + MAX_ANEXOS + ' arquivos de cada vez.', 'erro'); break; }
      if (!arq.size) { toast('"' + arq.name + '" está vazio e não foi anexado.', 'erro'); continue; }
      if (arq.size > LIMITE_ARQUIVO) { toast('"' + arq.name + '" tem mais de 25 MB. O WhatsApp não aceita arquivo desse tamanho.', 'erro'); continue; }
      r.anexos.push({ arq, url: /^image\//.test(arq.type) ? URL.createObjectURL(arq) : null });
    }
    pintarAnexos();
    p.texto.focus();
  }

  function tirarAnexo(id, anexo) {
    const r = rascunho(id);
    const i = r.anexos.indexOf(anexo);
    if (i < 0) return;
    r.anexos.splice(i, 1);
    if (anexo.url) URL.revokeObjectURL(anexo.url);
    if (painel && painel.id === id) { pintarAnexos(); painel.texto.focus(); }
  }

  function iconeDoArquivo(arq) {
    if (/^audio\//.test(arq.type)) return 'microfone';
    if (/^video\//.test(arq.type)) return 'video';
    return 'arquivo';
  }

  function pintarAnexos() {
    const p = painel;
    if (!p) return;
    const r = rascunho(p.id);
    p.anexos.hidden = !p.pode || !r.anexos.length || (!!gravacao && gravacao.id === p.id);
    trocar(p.anexos, r.anexos.map((a) => h('div', { class: 'cv-anexo' },
      a.url ? h('img', { class: 'cv-anexo-mini', src: a.url, alt: '' }) : h('span', { class: 'cv-anexo-mini' }, ic(iconeDoArquivo(a.arq))),
      h('span', { class: 'cv-anexo-texto' },
        h('span', { class: 'cv-anexo-nome', text: a.arq.name }),
        h('span', { class: 'cv-anexo-tamanho', text: tamanhoBonito(a.arq.size) })),
      h('button', { type: 'button', class: 'cv-ib cv-ib-p', 'aria-label': 'Tirar ' + a.arq.name, disabled: p.enviando, onclick: () => tirarAnexo(p.id, a) }, icone('fechar')))));
    const dica = r.anexos.length ? 'Legenda (opcional)' : 'Escreva uma mensagem';
    if (p.texto.placeholder !== dica) p.texto.placeholder = dica;
  }

  function enviarArquivo(id, corpo, opc) {
    const cabecalhos = {
      'Content-Type': 'application/octet-stream',
      'X-Lead': encodeURIComponent(id),
      'X-Nome': encodeURIComponent(opc.nome || 'arquivo'),
      'X-Mime': encodeURIComponent(opc.mime || 'application/octet-stream'),
      'X-Tipo': encodeURIComponent(opc.tipo || 'arquivo'),
    };
    if (opc.legenda) cabecalhos['X-Legenda'] = encodeURIComponent(opc.legenda);
    return pedir('/api/enviar-arquivo', { method: 'POST', headers: cabecalhos, body: corpo });
  }

  function travarEnvio(p, travar) {
    p.enviando = travar;
    for (const b of [p.botaoEnviar, p.botaoAnexo, p.botaoVoz]) b.disabled = travar;
    p.botaoEnviar.classList.toggle('ocupado', travar);
    if (travar) p.botaoEnviar.setAttribute('aria-busy', 'true');
    else p.botaoEnviar.removeAttribute('aria-busy');
    p.texto.readOnly = travar;
    p.anexos.querySelectorAll('button').forEach((b) => { b.disabled = travar; });
  }

  function limparTexto(id) {
    rascunho(id).texto = '';
    if (painel && painel.id === id) { painel.texto.value = ''; ajustarCampo(painel); }
  }

  // enviar manda o texto, ou os anexos com o texto como legenda do primeiro.
  async function enviar() {
    const p = painel;
    if (!p || !p.pode || p.enviando) return;
    const id = p.id;
    const r = rascunho(id);
    const texto = p.texto.value.trim();
    if (!texto && !r.anexos.length) { p.texto.focus(); return; }
    travarEnvio(p, true);
    limparAviso();
    let enviou = false;
    try {
      if (r.anexos.length) {
        let legenda = texto.length <= MAX_LEGENDA ? texto : '';
        let solto = legenda ? '' : texto;
        while (r.anexos.length) {
          const a = r.anexos[0];
          const levaLegenda = !!legenda && !/^audio\//.test(a.arq.type);
          await enviarArquivo(id, a.arq, { nome: a.arq.name, mime: a.arq.type, tipo: 'arquivo', legenda: levaLegenda ? legenda : '' });
          enviou = true;
          if (levaLegenda) { legenda = ''; limparTexto(id); }
          r.anexos.shift();
          if (a.url) URL.revokeObjectURL(a.url);
          if (painel === p) pintarAnexos();
        }
        if (legenda) solto = legenda;
        if (solto) {
          await api.post('/api/responder', { id, texto: solto });
          limparTexto(id);
        }
      } else {
        await api.post('/api/responder', { id, texto });
        enviou = true;
        limparTexto(id);
      }
    } catch (e) {
      toast(e.message || 'Não consegui enviar. Tente de novo.', 'erro');
    }
    travarEnvio(p, false);
    if (painel === p) {
      pintarAnexos();
      if (enviou) p.irAoFim = true;
      p.texto.focus();
    }
    if (enviou) await atualizarTudo();
  }

  // ---------- Arrastar e soltar ----------

  function ligarArrastar(chat) {
    const temArquivos = (ev) => !!ev.dataTransfer && Array.from(ev.dataTransfer.types || []).includes('Files');
    const esconder = () => { tela.arrastes = 0; if (painel) painel.soltar.hidden = true; };
    chat.addEventListener('dragenter', (ev) => {
      if (!tela || !painel || !temArquivos(ev)) return;
      ev.preventDefault();
      tela.arrastes++;
      painel.textoSoltar.textContent = painel.pode ? 'Solte aqui para anexar' : 'Inicie o atendimento para enviar arquivos';
      painel.soltar.hidden = false;
    });
    chat.addEventListener('dragover', (ev) => {
      if (!tela || !painel || !temArquivos(ev)) return;
      ev.preventDefault();
      ev.dataTransfer.dropEffect = 'copy';
    });
    chat.addEventListener('dragleave', () => {
      if (!tela) return;
      tela.arrastes = Math.max(0, tela.arrastes - 1);
      if (!tela.arrastes && painel) painel.soltar.hidden = true;
    });
    chat.addEventListener('drop', (ev) => {
      if (!tela || !painel || !temArquivos(ev)) return;
      ev.preventDefault();
      esconder();
      adicionarArquivos(ev.dataTransfer.files);
    });
  }

  // ---------- Gravação de voz ----------

  function explicarMicrofone(e) {
    const nome = e && e.name;
    if (nome === 'NotAllowedError' || nome === 'SecurityError') {
      return 'O navegador bloqueou o microfone. Clique no cadeado ao lado do endereço da página, permita o microfone e tente de novo.';
    }
    if (nome === 'NotFoundError' || nome === 'OverconstrainedError') return 'Não encontrei um microfone neste computador. Ligue um microfone e tente de novo.';
    if (nome === 'NotReadableError') return 'Outro programa está usando o microfone. Feche o outro programa e tente de novo.';
    return 'Não consegui usar o microfone. Tente de novo, ou anexe um arquivo de áudio.';
  }

  async function comecarGravacao() {
    const p = painel;
    if (!p || !p.pode || p.enviando || gravacao) return;
    const semSuporte = 'Este navegador não grava voz. Use o Chrome ou o Edge, ou anexe um arquivo de áudio.';
    if (!navigator.mediaDevices || !navigator.mediaDevices.getUserMedia || typeof MediaRecorder === 'undefined') { avisarCompor(semSuporte); return; }
    const tipo = TIPOS_VOZ.find((t) => { try { return MediaRecorder.isTypeSupported(t); } catch (e) { return false; } });
    if (!tipo) { avisarCompor(semSuporte); return; }
    p.botaoVoz.disabled = true;
    let fluxo = null;
    try {
      fluxo = await navigator.mediaDevices.getUserMedia({ audio: true });
    } catch (e) {
      if (painel === p) { p.botaoVoz.disabled = p.enviando; avisarCompor(explicarMicrofone(e)); }
      return;
    }
    p.botaoVoz.disabled = p.enviando;
    const parar = () => fluxo.getTracks().forEach((t) => t.stop());
    if (painel !== p || !p.pode || gravacao) { parar(); return; }
    let gravador;
    try {
      gravador = new MediaRecorder(fluxo, { mimeType: tipo });
    } catch (e) {
      parar();
      avisarCompor(semSuporte);
      return;
    }
    const g = { id: p.id, fluxo, gravador, tipo, pedacos: [], inicio: Date.now(), enviar: false, terminou: false, relogio: 0 };
    gravador.addEventListener('dataavailable', (ev) => { if (ev.data && ev.data.size) g.pedacos.push(ev.data); });
    gravador.addEventListener('stop', () => aoPararGravacao(g));
    gravador.addEventListener('error', () => { g.enviar = false; aoPararGravacao(g); toast('A gravação falhou. Tente de novo.', 'erro'); });
    gravacao = g;
    g.relogio = setInterval(() => pintarCronometro(g), 400);
    try {
      gravador.start();
    } catch (e) {
      g.enviar = false;
      aoPararGravacao(g);
      avisarCompor(semSuporte);
      return;
    }
    limparAviso();
    pintarCronometro(g);
    pintarCompor();
    const botaoEnviarVoz = p.gravando.querySelector('.cv-btn-verde');
    if (botaoEnviarVoz) botaoEnviarVoz.focus();
  }

  function pintarCronometro(g) {
    if (!painel || painel.id !== g.id) return;
    const seg = Math.floor((Date.now() - g.inicio) / 1000);
    painel.cronometro.textContent = Math.floor(seg / 60) + ':' + doisDigitos(seg % 60);
  }

  function terminarGravacao(mandar) {
    const g = gravacao;
    if (!g) return;
    g.enviar = !!mandar;
    try {
      if (g.gravador.state !== 'inactive') g.gravador.stop();
      else aoPararGravacao(g);
    } catch (e) {
      aoPararGravacao(g);
    }
  }

  function cancelarGravacao() { if (gravacao) terminarGravacao(false); }

  function aoPararGravacao(g) {
    if (g.terminou) return;
    g.terminou = true;
    clearInterval(g.relogio);
    try { g.fluxo.getTracks().forEach((t) => t.stop()); } catch (e) { /* já parou */ }
    if (gravacao === g) gravacao = null;
    if (painel && painel.id === g.id) { pintarCompor(); pintarAnexos(); }
    if (!g.enviar) return;
    const tipo = g.gravador.mimeType || g.tipo;
    const voz = new Blob(g.pedacos, { type: tipo });
    if (!voz.size || Date.now() - g.inicio < 700) { toast('A gravação ficou curta demais. Grave de novo.', 'erro'); return; }
    if (voz.size > LIMITE_ARQUIVO) { toast('A gravação passou de 25 MB. Grave uma mensagem mais curta.', 'erro'); return; }
    enviarVoz(g.id, voz, tipo);
  }

  async function enviarVoz(id, voz, tipo) {
    const p = painel && painel.id === id ? painel : null;
    const extensao = /ogg/.test(tipo) ? 'ogg' : (/mp4/.test(tipo) ? 'm4a' : 'webm');
    if (p) travarEnvio(p, true);
    let enviou = false;
    try {
      await enviarArquivo(id, voz, { nome: 'voz-' + carimbo() + '.' + extensao, mime: tipo, tipo: 'voz' });
      enviou = true;
    } catch (e) {
      toast(e.message || 'Não consegui enviar a gravação. Tente de novo.', 'erro');
    }
    if (p) {
      travarEnvio(p, false);
      if (painel === p) {
        if (enviou) p.irAoFim = true;
        if (p.pode) p.texto.focus();
      }
    }
    if (enviou) await atualizarTudo();
  }

  // ---------- Ficha do lead ----------

  function bloco(titulo, desenho) {
    const corpo = h('div', { class: 'cv-bloco-corpo' });
    const el = h('section', { class: 'cv-bloco' }, h('h3', null, ic(desenho), h('span', { text: titulo })), corpo);
    return { el, corpo };
  }

  function limparEtiqueta(t) { return String(t).trim().replace(/\s+/g, ' ').slice(0, 30); }

  function montarFicha(p) {
    const f = { marcas: {}, campoEtq: null, salvando: 0, filaEtq: Promise.resolve(), vistoEtq: null };
    f.foto = h('span', { class: 'cv-ficha-foto' });
    f.nome = h('p', { class: 'cv-ficha-nome' });
    f.fone = h('span', { class: 'cv-ficha-fone' });
    f.linhaFone = h('div', { class: 'cv-ficha-linha-fone', hidden: true }, f.fone,
      botao('', {
        classe: 'cv-copiar', icone: null, rotulo: 'Copiar telefone', dica: 'Copiar telefone',
        aoClicar: async () => {
          const l = S.lead && S.lead.id === p.id ? S.lead : resumoDe(p.id);
          const fone = l && (l.telefone_bonito || l.telefone);
          if (!fone) return;
          if (await copiarTexto(fone)) toast('Telefone copiado.');
          else throw new Error('Não consegui copiar. Selecione o número e copie com o teclado.');
        },
      }));
    f.linhaFone.lastChild.appendChild(ic('copiar'));
    f.situacao = bloco('Situação', 'assumir');
    f.temperatura = bloco('Temperatura', 'termometro');
    f.etiquetas = bloco('Etiquetas', 'etiqueta');
    f.resumo = bloco('Resumo', 'texto');
    f.sabemos = bloco('O que já sabemos', 'sabemos');
    f.agenda = bloco('Agenda', 'agenda');
    f.historico = bloco('Histórico interno', 'historico');
    trocar(f.historico.corpo, h('p', { class: 'cv-falta', text: 'Carregando...' }));

    trocar(tela.ficha,
      h('div', { class: 'cv-ficha-topo' },
        h('h2', { text: 'Detalhes do cliente' }),
        h('button', { type: 'button', class: 'btn-icone cv-ficha-fechar', 'aria-label': 'Fechar detalhes', onclick: alternarFicha }, icone('fechar'))),
      h('div', { class: 'cv-ficha-rolagem' },
        h('div', { class: 'cv-ficha-quem' }, f.foto, f.nome, f.linhaFone),
        f.situacao.el, f.temperatura.el, f.etiquetas.el, f.resumo.el, f.sabemos.el, f.agenda.el, f.historico.el,
        botao('Apagar conversa', { classe: 'btn-perigo-suave cv-apagar', icone: 'lixo', aoClicar: () => apagarConversa(p.id) })));
    p.ficha = f;
  }

  async function apagarConversa(id) {
    const l = (S.lead && S.lead.id === id ? S.lead : resumoDe(id)) || {};
    const sim = await confirmar({
      titulo: 'Apagar esta conversa?',
      texto: 'A conversa e a ficha de ' + (l.exibir || 'este lead') + ' saem do painel. As mensagens continuam no WhatsApp do celular. Não dá para desfazer.',
      botao: 'Apagar conversa', perigo: true,
    });
    if (!sim) return;
    await api.post('/api/lead/excluir', { id });
    rascunhos.delete(id);
    toast('Conversa apagada.');
    if (S.aberto === id) fechar();
    await pulsar();
  }

  function pintarFicha(l, agendamentos) {
    const p = painel;
    if (!p || !p.ficha) return;
    const f = p.ficha;
    const nome = nomeDe(l);
    mudou(f, 'foto', chaveFoto(l), () => trocar(f.foto, avatar(l, 'cv-avatar-g')));
    mudou(f, 'nome', nome, () => { f.nome.textContent = nome; });
    const fone = l.telefone_bonito || l.telefone || '';
    mudou(f, 'fone', fone, () => { f.fone.textContent = fone; f.linhaFone.hidden = !fone; });

    // Situação: quem está com a conversa.
    const situacao = situacaoDe(l);
    const eu = euMesmo();
    mudou(f, 'situacao', [situacao, l.atendente || '', l.motivo || '', eu].join('|'), () => {
      let selo;
      let frase;
      if (situacao === 'fila') {
        selo = h('span', { class: 'cv-selo cv-selo-fila' }, icone('relogio'), 'Na fila');
        frase = 'O assistente transferiu. A conversa espera uma pessoa da equipe.';
      } else if (situacao === 'atendimento') {
        selo = h('span', { class: 'cv-selo cv-selo-atend' }, ic('fone'), 'Em atendimento');
        const quem = l.atendente ? (l.atendente === eu ? 'Você' : l.atendente) : 'Uma pessoa da equipe';
        frase = quem + ' está atendendo. O assistente fica parado nesta conversa.';
      } else {
        selo = h('span', { class: 'cv-selo cv-selo-ia' }, icone('assistente'), 'Com o assistente');
        frase = 'O assistente responde sozinho. Se precisar, ele transfere para a equipe.';
      }
      trocar(f.situacao.corpo,
        h('div', null, selo),
        h('p', { class: 'cv-frase', text: frase }),
        situacao !== 'ia' && l.motivo ? h('dl', { class: 'cv-dados' }, h('dt', { text: 'Motivo da transferência' }), h('dd', { text: l.motivo })) : null,
        eu ? h('p', { class: 'cv-meu-nome', text: 'Você aparece aqui como ' + eu + '. O cliente não vê o seu nome.' }) : null);
    });

    // Temperatura: pontos, classificação e etapa.
    const pontos = Math.max(0, Math.min(100, Number(l.pontuacao) || 0));
    const temp = temperatura(l.classificacao);
    mudou(f, 'temperatura', [pontos, temp, l.etapa || ''].join('|'), () => {
      const preenchido = h('span');
      preenchido.style.width = pontos + '%';
      trocar(f.temperatura.corpo,
        h('div', { class: 'cv-pontos', 'data-temp': temp },
          h('div', { class: 'cv-pontos-linha' },
            h('span', { class: 'cv-pontos-numero' }, String(pontos), h('small', { text: ' pontos de 100' })),
            etiquetaTemp(l.classificacao)),
          h('div', { class: 'barra', role: 'img', 'aria-label': pontos + ' pontos de 100' }, preenchido)),
        h('dl', { class: 'cv-dados' }, definicao('Etapa da conversa', l.etapa, 'Começando')));
    });

    pintarEtiquetas(l);

    mudou(f, 'resumo', l.resumo || '', () => {
      trocar(f.resumo.corpo, l.resumo
        ? h('p', { class: 'cv-resumo', text: l.resumo })
        : h('p', { class: 'cv-falta', text: 'Ainda sem resumo. Ele aparece depois de algumas mensagens.' }));
    });

    mudou(f, 'sabemos', [l.necessidade, l.prazo, l.decisor, l.orcamento].join('|'), () => {
      trocar(f.sabemos.corpo, h('dl', { class: 'cv-dados' },
        definicao('O que precisa', l.necessidade),
        definicao('Prazo', l.prazo),
        definicao('Quem decide', l.decisor),
        definicao('Orçamento', l.orcamento)));
    });

    // Agenda: os próximos horários deste cliente.
    const agora = Date.now();
    let proximos;
    if (Array.isArray(agendamentos)) {
      proximos = agendamentos
        .filter((a) => a.status === 'marcado' && new Date(a.inicio).getTime() >= agora - 3600000)
        .sort((a, b) => new Date(a.inicio) - new Date(b.inicio));
    } else {
      proximos = l.proximo_agendamento ? [l.proximo_agendamento] : [];
    }
    mudou(f, 'agenda', proximos.map((a) => [a.inicio, a.servico, quando(a.inicio)].join('~')).join('|'), () => {
      trocar(f.agenda.corpo,
        proximos.length
          ? h('ul', { class: 'cv-horarios' }, proximos.map((a) => h('li', null,
            h('strong', { text: quando(a.inicio) }), a.servico ? h('span', { text: a.servico }) : null)))
          : h('p', { class: 'cv-falta', text: 'Nenhum horário marcado.' }),
        h('a', { class: 'cv-link', href: '#agenda' }, 'Abrir a agenda'));
    });

    // Histórico interno: o que a equipe e o assistente fizeram. O cliente não vê.
    if (Array.isArray(l.historico)) {
      const eventos = l.historico.filter(ehEvento).slice(-30).reverse();
      const marca = [eventos.map((m) => [m.hora, m.evento, m.autor, m.texto, quando(m.hora)].join('~')).join('|'), l.criado_em, l.total_msgs].join('||');
      mudou(f, 'historico', marca, () => {
        const pe = [];
        if (l.criado_em && quando(l.criado_em)) pe.push('Primeiro contato: ' + quando(l.criado_em) + '.');
        if (l.total_msgs != null) pe.push(l.total_msgs === 1 ? '1 mensagem na conversa.' : l.total_msgs + ' mensagens na conversa.');
        trocar(f.historico.corpo,
          eventos.length
            ? h('ol', { class: 'cv-eventos' }, eventos.map((m) => h('li', null,
              h('span', { text: textoDoEvento(m) }), h('time', { datetime: m.hora, text: quando(m.hora) }))))
            : h('p', { class: 'cv-falta', text: 'Nada por aqui ainda. Transferências e atendimentos aparecem nesta lista.' }),
          pe.length ? h('p', { class: 'cv-rodape-bloco', text: pe.join(' ') }) : null);
      });
    }
  }

  // ---------- Etiquetas ----------

  function pintarEtiquetas(l) {
    const p = painel;
    const f = p.ficha;
    const doServidor = (l.etiquetas || []).map(String);
    const sugestoes = etiquetasEmUso();
    const marcaSugestoes = sugestoes.join('\n');
    const digitando = !!f.campoEtq && f.campoEtq.contains(document.activeElement);
    if (!f.campoEtq || (f.marcas.sugestoes !== marcaSugestoes && !digitando && !f.salvando)) {
      f.marcas.sugestoes = marcaSugestoes;
      f.vistoEtq = JSON.stringify(doServidor);
      f.campoEtq = chips(doServidor, {
        rotulo: 'Adicionar etiqueta', placeholder: 'Digite e aperte Enter',
        teclas: [',', ';', 'Tab', 'Enter'], limpar: limparEtiqueta, sugestoes, max: 12,
        aoMudar: (lista) => salvarEtiquetas(p, lista),
      });
      f.ajudaEtq = h('p', { class: 'ajuda' });
      trocar(f.etiquetas.corpo, f.campoEtq, f.ajudaEtq);
      pintarAjudaEtiquetas(l);
      return;
    }
    pintarAjudaEtiquetas(l);
    const marca = JSON.stringify(doServidor);
    if (marca !== f.vistoEtq && !f.salvando && !digitando) {
      f.vistoEtq = marca;
      f.campoEtq.definir(doServidor);
    }
  }

  // pintarAjudaEtiquetas diz quais etiquetas foram criadas pela IA durante o atendimento.
  function pintarAjudaEtiquetas(l) {
    const f = painel && painel.ficha;
    if (!f || !f.ajudaEtq) return;
    const atuais = (l.etiquetas || []).map(String);
    const daIA = (l.etiquetas_ia || []).map(String).filter((e) => atuais.includes(e));
    const texto = daIA.length
      ? 'Postas pela IA: ' + daIA.join(', ') + '. Se você tirar uma, ela não volta neste contato.'
      : 'A IA põe e troca etiquetas sozinha durante o atendimento, usando as que já existem. Você também pode pôr as suas.';
    if (f.ajudaEtq.textContent !== texto) f.ajudaEtq.textContent = texto;
  }

  // salvarEtiquetas grava a lista a cada mudança, uma gravação de cada vez e na ordem.
  function salvarEtiquetas(p, lista) {
    const f = p.ficha;
    f.salvando++;
    f.filaEtq = f.filaEtq.then(async () => {
      try {
        const r = await api.post('/api/lead/etiquetas', { id: p.id, etiquetas: lista });
        const limpas = Array.isArray(r.etiquetas) ? r.etiquetas.map(String) : lista;
        f.vistoEtq = JSON.stringify(limpas);
        if (S.lead && S.lead.id === p.id) {
          S.lead.etiquetas = limpas.slice();
          if (Array.isArray(r.etiquetas_ia)) S.lead.etiquetas_ia = r.etiquetas_ia.map(String);
          if (painel === p) pintarAjudaEtiquetas(S.lead);
        }
        const resumo = resumoDe(p.id);
        if (resumo) resumo.etiquetas = limpas.slice();
        if (painel === p && f.salvando === 1 && JSON.stringify(limpas) !== JSON.stringify(lista)) f.campoEtq.definir(limpas);
      } catch (e) {
        toast(e.message || 'Não consegui salvar as etiquetas. Tente de novo.', 'erro');
        f.vistoEtq = null; // na próxima busca o campo volta ao que está salvo
      }
      f.salvando--;
      if (tela && !f.salvando) { pintarLista(); pulsar(); }
    });
  }

  // avatar e copiarTexto também servem à aba Contatos.
  TELAS.conversas = { montar, atualizar, sair, avatar, copiarTexto };
})();
