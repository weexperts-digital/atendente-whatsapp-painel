'use strict';
// Aba Contatos: todo mundo que já escreveu para o negócio. Serve para ver, achar e baixar.
//
// A busca e os filtros são feitos pelo programa: a cada mudança a tela pede /api/contatos
// de novo. As duas planilhas levam o mesmo filtro no endereço. A lista se atualiza sozinha
// quando chega gente nova ou mensagem nova, sem piscar e sem perder a rolagem.
// Regra de ouro: nada que vem da API vira HTML. Tudo entra como texto.

(function () {
  let tela = null;

  const ATRASO_BUSCA = 250;  // espera a pessoa parar de digitar
  const PASSO = 200;         // quantas linhas entram na página de cada vez
  const SITUACOES = [
    { valor: '', texto: 'Todas as situações' },
    { valor: 'fila', texto: 'Na fila' },
    { valor: 'atendimento', texto: 'Em atendimento' },
    { valor: 'assistente', texto: 'Com o assistente' },
  ];
  const TEMPERATURAS_FILTRO = [
    { valor: '', texto: 'Todas as temperaturas' },
    { valor: 'quente', texto: 'Quentes', cor: 'var(--quente)' },
    { valor: 'morno', texto: 'Mornos', cor: 'var(--morno)' },
    { valor: 'frio', texto: 'Frios', cor: 'var(--frio)' },
    { valor: 'indefinido', texto: 'Sem classificação', cor: 'var(--linha-forte)' },
  ];
  const ORDENS = [
    { valor: 'recentes', texto: 'Mais recentes primeiro', dica: 'Pela última mensagem' },
    { valor: 'antigos', texto: 'Mais antigos primeiro', dica: 'Pelo primeiro contato' },
    { valor: 'nome', texto: 'Por nome, de A a Z' },
    { valor: 'pontuacao', texto: 'Maior pontuação primeiro' },
  ];
  const COLUNAS = ['Contato', 'Telefone', 'Temperatura', 'Situação', 'Etiquetas', 'Primeiro contato', 'Última mensagem', 'Mensagens'];
  const BUSCA = ['M11 19a8 8 0 1 0 0-16a8 8 0 0 0 0 16z', 'm21 21-4.3-4.3'];
  const COPIAR = ['M10 8h10a2 2 0 0 1 2 2v10a2 2 0 0 1-2 2H10a2 2 0 0 1-2-2V10a2 2 0 0 1 2-2z', 'M4 16c-1.1 0-2-.9-2-2V4c0-1.1.9-2 2-2h10c1.1 0 2 .9 2 2'];
  const FONE = ['M3 14h3a2 2 0 0 1 2 2v3a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-7a9 9 0 0 1 18 0v7a2 2 0 0 1-2 2h-1a2 2 0 0 1-2-2v-3a2 2 0 0 1 2-2h3'];

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

  function plural(n, um, varios) { return n + ' ' + (n === 1 ? um : varios); }

  // dataCurta: 09/11/2026. dataCompleta: 09/11/2026 às 14:02.
  function dataCurta(iso) {
    const d = new Date(iso);
    if (isNaN(d) || d.getFullYear() < 2000) return '';
    return doisDigitos(d.getDate()) + '/' + doisDigitos(d.getMonth() + 1) + '/' + d.getFullYear();
  }
  function dataCompleta(iso) {
    const dia = dataCurta(iso);
    return dia ? dia + ' às ' + horaCurta(iso) : '';
  }

  // A foto e o copiar são os mesmos da aba Conversas.
  function avatarDe(c) {
    const el = TELAS.conversas.avatar(c, 'cv-avatar-p');
    const img = el.querySelector('img');
    if (img) img.loading = 'lazy'; // com muita gente na lista, a foto só carrega perto da tela
    return el;
  }

  // memoria guarda a busca, os filtros e a última resposta, para a volta à aba ser imediata.
  function memoria() {
    if (!S.contatos) S.contatos = { q: '', situacao: '', temperatura: '', etiqueta: '', ordem: 'recentes', dados: null };
    return S.contatos;
  }

  function temFiltro() {
    const m = memoria();
    return !!(m.q || m.situacao || m.temperatura || m.etiqueta);
  }

  // consulta monta a parte do endereço com a busca e os filtros que estão valendo.
  // É a mesma para a lista e para as duas planilhas.
  function consulta() {
    const m = memoria();
    const p = new URLSearchParams();
    if (m.q) p.set('q', m.q);
    if (m.situacao) p.set('situacao', m.situacao);
    if (m.temperatura) p.set('temperatura', m.temperatura);
    if (m.etiqueta) p.set('etiqueta', m.etiqueta);
    if (m.ordem && m.ordem !== 'recentes') p.set('ordem', m.ordem);
    const texto = p.toString();
    return texto ? '?' + texto : '';
  }

  // assinatura muda quando chega contato novo, mensagem nova, ou quando alguma conversa
  // troca de situação, temperatura ou etiquetas. Só aí a lista é pedida de novo.
  function assinatura() {
    const leads = (S.estado && S.estado.leads) || [];
    let ultima = 0;
    const resto = [];
    for (const l of leads) {
      const t = Date.parse(l.ultima_msg) || 0;
      if (t > ultima) ultima = t;
      resto.push([l.status, l.classificacao, (l.etiquetas || []).join(','), l.atendente || ''].join('~'));
    }
    return leads.length + '|' + ultima + '|' + resto.join(';');
  }

  // ---------- Tela ----------

  function montar(main) {
    if (tela) sair();
    const m = memoria();

    const total = h('p', { class: 'descricao ct-total', role: 'status' });
    const botaoBaixar = h('button', { type: 'button', class: 'btn btn-primario ct-baixar' },
      icone('baixar'), h('span', { text: 'Baixar' }), icone('seta-baixo', 'ct-baixar-seta'));
    const menu = suspenso(botaoBaixar, montarMenuBaixar, { papel: 'menu', alinhar: 'fim', largura: 340 });

    const campoBusca = h('input', {
      type: 'search', class: 'ct-busca-campo', placeholder: 'Buscar nome, telefone ou etiqueta',
      'aria-label': 'Buscar contato por nome, telefone ou etiqueta', autocomplete: 'off', spellcheck: 'false', value: m.q,
    });
    const limparBusca = h('button', {
      type: 'button', class: 'ct-busca-limpar', 'aria-label': 'Limpar busca', hidden: !m.q,
      onclick: () => { campoBusca.value = ''; definirBusca('', true); campoBusca.focus(); },
    }, icone('fechar'));
    campoBusca.addEventListener('input', () => definirBusca(campoBusca.value, false));
    campoBusca.addEventListener('keydown', (ev) => { if (ev.key === 'Enter') definirBusca(campoBusca.value, true); });

    const filtro = (chave, rotulo, opcoes) => listaSuspensa({
      rotulo, opcoes, valor: m[chave],
      aoMudar: (v) => { m[chave] = v; pintarLimpar(); buscar(); },
    });
    const situacao = filtro('situacao', 'Filtrar por situação', SITUACOES);
    const temperaturaEl = filtro('temperatura', 'Filtrar por temperatura', TEMPERATURAS_FILTRO);
    const etiqueta = filtro('etiqueta', 'Filtrar por etiqueta', [{ valor: '', texto: 'Todas as etiquetas' }]);
    const ordem = listaSuspensa({
      rotulo: 'Ordem da lista', opcoes: ORDENS, valor: m.ordem, alinhar: 'fim',
      aoMudar: (v) => { m.ordem = v; buscar(); },
    });
    // O lugar do botão fica guardado na barra: quando ele aparece, nada sai do lugar.
    const limpar = h('button', {
      type: 'button', class: 'btn btn-fantasma ct-limpar ct-limpar-fora', disabled: true,
      'aria-label': 'Limpar filtros', title: 'Limpar filtros', onclick: limparFiltros,
    }, icone('fechar'), h('span', { text: 'Limpar' }));

    const corpo = h('div', { class: 'ct-corpo' });
    const raiz = h('div', { class: 'tela-contatos' },
      h('div', { class: 'topo-tela' },
        h('div', { class: 'topo-textos' }, h('h1', { text: 'Contatos' }), total),
        h('div', { class: 'ct-acoes' },
          h('a', { class: 'link-baixar', href: '#relatorio' }, icone('relatorio'), h('span', { text: 'Ver relatório' })),
          botaoBaixar)),
      h('div', { class: 'ct-barra', role: 'search', 'aria-label': 'Buscar e filtrar contatos' },
        h('div', { class: 'ct-busca' }, desenho(BUSCA), campoBusca, limparBusca),
        h('div', { class: 'ct-filtros' }, situacao, temperaturaEl, etiqueta, ordem),
        limpar),
      corpo);
    main.appendChild(raiz);

    tela = {
      raiz, corpo, total, menu, campoBusca, limparBusca, situacao, temperaturaEl, etiqueta, ordem, limpar,
      linhas: new Map(), tabela: null, tbody: null, mais: null, modo: '', limite: PASSO,
      pedido: 0, esperaBusca: 0, esperaOcupado: 0, assinatura: S.estado ? assinatura() : undefined, erro: null, marcaOpcoes: '', consultaPintada: null,
    };
    pintarOpcoes();
    pintarLimpar();
    pintar();
    buscar();
  }

  function sair() {
    if (!tela) return;
    clearTimeout(tela.esperaBusca);
    clearTimeout(tela.esperaOcupado);
    tela.menu.fechar();
    for (const el of [tela.situacao, tela.temperaturaEl, tela.etiqueta, tela.ordem]) el.fechar();
    tela = null;
  }

  // atualizar é chamado pela casca a cada busca de /api/estado.
  function atualizar() {
    if (!tela) return;
    pintarOpcoes();
    const nova = assinatura();
    // A tela abriu antes de a situação chegar: a lista já foi pedida, só guarda a marca.
    if (S.estado && tela.assinatura === undefined) tela.assinatura = nova;
    if (S.estado && nova !== tela.assinatura) {
      tela.assinatura = nova;
      buscar(true);
      return;
    }
    // Nada mudou: só os tempos ("há 5 min") andam.
    const d = memoria().dados;
    if (tela.modo === 'tabela' && d) pintarLinhas(d.contatos || []);
  }

  // ---------- Busca e filtros ----------

  function definirBusca(valor, agora) {
    if (!tela) return;
    const m = memoria();
    tela.limparBusca.hidden = !valor;
    clearTimeout(tela.esperaBusca);
    const aplicar = () => {
      const limpo = valor.trim();
      if (limpo === m.q) return;
      m.q = limpo;
      pintarLimpar();
      buscar();
    };
    if (agora) aplicar();
    else tela.esperaBusca = setTimeout(aplicar, ATRASO_BUSCA);
  }

  function limparFiltros() {
    if (!tela) return;
    const m = memoria();
    clearTimeout(tela.esperaBusca);
    m.q = '';
    m.situacao = '';
    m.temperatura = '';
    m.etiqueta = '';
    tela.campoBusca.value = '';
    tela.limparBusca.hidden = true;
    tela.situacao.valor = '';
    tela.temperaturaEl.valor = '';
    tela.etiqueta.valor = '';
    pintarLimpar();
    buscar();
    // O botão que foi clicado some: o foco vai para o primeiro filtro (sem abrir o teclado do celular).
    tela.situacao.focus({ preventScroll: true });
  }

  function pintarLimpar() {
    if (!tela) return;
    const ativo = temFiltro();
    tela.limpar.disabled = !ativo;
    tela.limpar.classList.toggle('ct-limpar-fora', !ativo);
  }

  // pintarOpcoes põe em cada opção dos filtros quantos contatos ela tem, e mantém a lista
  // de etiquetas igual à que está em uso.
  function pintarOpcoes() {
    if (!tela) return;
    const m = memoria();
    const leads = (S.estado && S.estado.leads) || null;
    const etiquetas = ((S.estado && S.estado.etiquetas) || (m.dados && m.dados.etiquetas) || []).map(String);
    if (m.etiqueta && !etiquetas.includes(m.etiqueta)) etiquetas.push(m.etiqueta);
    const conta = (passa) => (leads ? leads.filter(passa).length : null);
    const situacaoDe = (l) => (l.status === 'aguardando humano' ? 'fila' : (l.status === 'em atendimento' ? 'atendimento' : 'assistente'));
    const situacoes = SITUACOES.map((o) => Object.assign({}, o, { conta: conta((l) => !o.valor || situacaoDe(l) === o.valor) }));
    const temperaturas = TEMPERATURAS_FILTRO.map((o) => Object.assign({}, o, { conta: conta((l) => !o.valor || temperatura(l.classificacao) === o.valor) }));
    const etqs = [{ valor: '', texto: 'Todas as etiquetas', conta: conta(() => true) }]
      .concat(etiquetas.map((e) => ({ valor: e, texto: e, conta: conta((l) => (l.etiquetas || []).map(String).includes(e)) })));
    const marca = JSON.stringify([situacoes, temperaturas, etqs]);
    if (marca === tela.marcaOpcoes) return;
    tela.marcaOpcoes = marca;
    tela.situacao.definirOpcoes(situacoes);
    tela.temperaturaEl.definirOpcoes(temperaturas);
    tela.etiqueta.definirOpcoes(etqs);
  }

  // buscar pede a lista com a busca e os filtros de agora. Resposta atrasada, de um pedido
  // que já foi trocado por outro, é jogada fora. "emSegundoPlano" é a atualização sozinha:
  // se ela falhar, a lista que está na tela fica onde está e a próxima batida tenta de novo.
  async function buscar(emSegundoPlano) {
    if (!tela) return;
    const minha = tela;
    const m = memoria();
    const pedido = ++minha.pedido;
    const filtros = consulta();
    clearTimeout(minha.esperaOcupado);
    if (emSegundoPlano !== true) {
      // Se a resposta demorar, a lista fica mais clara para mostrar que está buscando.
      minha.esperaOcupado = setTimeout(() => {
        if (tela === minha && minha.pedido === pedido) minha.corpo.classList.add('ct-buscando');
      }, 300);
      minha.corpo.setAttribute('aria-busy', 'true');
    }
    let dados = null;
    let erro = null;
    try {
      dados = await api.get('/api/contatos' + filtros);
    } catch (e) {
      erro = e;
    }
    if (tela !== minha || minha.pedido !== pedido) return;
    clearTimeout(minha.esperaOcupado);
    minha.corpo.classList.remove('ct-buscando');
    minha.corpo.removeAttribute('aria-busy');
    if (erro) {
      minha.assinatura = null; // tenta de novo na próxima batida
      if (emSegundoPlano === true && (minha.modo === 'tabela' || minha.modo === 'erro')) return;
      minha.erro = erro;
      m.dados = null;
    } else {
      minha.erro = null;
      m.dados = dados || {};
      // Só uma busca diferente volta a lista para o começo. A atualização sozinha não.
      if (minha.consultaPintada !== filtros) minha.limite = PASSO;
      minha.consultaPintada = filtros;
    }
    pintar();
    pintarOpcoes();
  }

  // ---------- Pintura ----------

  function pintarTotal() {
    const d = memoria().dados;
    let texto = '';
    if (d) {
      const total = Number(d.total) || 0;
      const geral = Number(d.total_geral) || 0;
      if (!geral) texto = 'Nenhum contato';
      else if (temFiltro() || total !== geral) texto = total + ' de ' + plural(geral, 'contato', 'contatos');
      else texto = plural(geral, 'contato', 'contatos');
    }
    if (tela.total.textContent !== texto) tela.total.textContent = texto;
  }

  function mostrar(modo, ...filhos) {
    tela.modo = modo;
    tela.tabela = null;
    tela.tbody = null;
    tela.mais = null;
    tela.linhas.clear();
    trocar(tela.corpo, filhos);
  }

  function pintar() {
    if (!tela) return;
    const d = memoria().dados;
    pintarTotal();
    if (tela.erro) {
      mostrar('erro', falhaCarga(tela.erro, () => buscar()));
      return;
    }
    if (!d) {
      if (tela.modo !== 'carregando') mostrar('carregando', carregandoEl('Carregando contatos'));
      return;
    }
    const contatos = Array.isArray(d.contatos) ? d.contatos : [];
    if (!(Number(d.total_geral) || 0)) {
      mostrar('vazio', h('div', { class: 'cartao' },
        vazio('contatos', 'Nenhum contato ainda', 'Quem escrever para o WhatsApp do negócio aparece aqui.')));
      return;
    }
    if (!contatos.length) {
      mostrar('filtro', h('div', { class: 'cartao' },
        vazio('contatos', 'Nenhum contato com esse filtro', 'Mude a busca ou os filtros para ver mais gente.',
          botao('Limpar filtros', { classe: 'btn-primario', aoClicar: limparFiltros }))));
      return;
    }
    if (tela.modo !== 'tabela') montarTabela();
    pintarLinhas(contatos);
  }

  function montarTabela() {
    const tbody = h('tbody', { role: 'rowgroup' });
    const tabela = h('table', { class: 'ct-tabela', role: 'table', 'aria-label': 'Contatos' },
      h('thead', { role: 'rowgroup' }, h('tr', { role: 'row' },
        COLUNAS.map((nome, i) => h('th', { scope: 'col', role: 'columnheader', class: i === COLUNAS.length - 1 ? 'ct-num' : null, text: nome })))),
      tbody);
    const mais = h('div', { class: 'ct-mais', hidden: true });
    mostrar('tabela',
      h('p', { class: 'so-leitor', id: 'ct-dica', text: 'Aperte Enter em uma linha para abrir a conversa da pessoa.' }),
      h('div', { class: 'ct-tabela-caixa' }, tabela), mais);
    tela.tabela = tabela;
    tela.tbody = tbody;
    tela.mais = mais;
  }

  function abrirConversa(id) {
    location.hash = '#conversas/' + encodeURIComponent(id);
  }

  function criarLinha(id) {
    const tr = h('tr', { class: 'ct-linha', role: 'row', tabindex: '0', 'aria-describedby': 'ct-dica' });
    tr.addEventListener('click', (ev) => {
      if (ev.target instanceof Element && ev.target.closest('button, a')) return;
      // Quem está selecionando um texto da linha não quer sair da tela.
      const selecao = window.getSelection ? String(window.getSelection()) : '';
      if (selecao && tr.contains(window.getSelection().anchorNode)) return;
      abrirConversa(id);
    });
    tr.addEventListener('keydown', (ev) => {
      if (ev.key === 'Enter' && ev.target === tr) { ev.preventDefault(); abrirConversa(id); }
    });
    return { tr, marca: null, ultima: null };
  }

  function seloSituacao(c) {
    if (c.status === 'aguardando humano') return h('span', { class: 'cv-selo cv-selo-fila' }, icone('relogio'), h('span', { text: 'Na fila' }));
    if (c.status === 'em atendimento') {
      const texto = c.atendente ? 'Com ' + c.atendente : 'Em atendimento';
      return h('span', { class: 'cv-selo cv-selo-atend', title: texto }, desenho(FONE), h('span', { text: texto }));
    }
    return h('span', { class: 'cv-selo cv-selo-ia' }, icone('assistente'), h('span', { text: 'Com o assistente' }));
  }

  function celula(classe, rotulo, ...filhos) {
    return h('td', { class: classe, role: 'cell', 'data-rotulo': rotulo }, filhos);
  }

  function pintarLinha(p, c) {
    const nome = c.exibir || c.nome || c.nome_whatsapp || c.telefone_bonito || 'Sem nome';
    const apelido = c.nome_whatsapp && c.nome_whatsapp !== nome ? c.nome_whatsapp : '';
    const fone = c.telefone_bonito || c.telefone || '';
    const pontos = Math.max(0, Math.min(100, Number(c.pontuacao) || 0));
    const etiquetas = (c.etiquetas || []).map(String);
    const relativo = tempoRelativo(c.ultima_msg);
    const marca = JSON.stringify([nome, apelido, fone, c.tem_foto, c.foto_versao, c.classificacao, pontos, c.status, c.atendente,
      etiquetas, c.criado_em, c.ultima_msg, c.total_msgs]);
    if (p.marca === marca) {
      // Nada mudou no contato: só o "há 5 min" anda, sem refazer a linha.
      if (p.ultima && p.ultima.textContent !== relativo) p.ultima.textContent = relativo;
      return;
    }
    p.marca = marca;
    p.ultima = h('time', { datetime: c.ultima_msg, title: dataCompleta(c.ultima_msg), text: relativo });

    const copiar = h('button', {
      type: 'button', class: 'ct-copiar', 'aria-label': 'Copiar o telefone de ' + nome, title: 'Copiar telefone',
      onclick: async () => {
        if (await TELAS.conversas.copiarTexto(fone)) toast('Telefone copiado.');
        else toast('Não consegui copiar. Selecione o número e copie com o teclado.', 'erro');
      },
    }, desenho(COPIAR));
    const mensagens = Number(c.total_msgs) || 0;

    trocar(p.tr,
      celula('ct-c-quem', null,
        h('div', { class: 'ct-quem' }, avatarDe(c),
          h('div', { class: 'ct-quem-textos' },
            h('span', { class: 'ct-nome', text: nome }),
            apelido ? h('span', { class: 'ct-apelido', text: 'No WhatsApp: ' + apelido }) : null))),
      celula('ct-c-fone', null, fone ? h('span', { class: 'ct-fone' }, h('span', { text: fone }), copiar) : null),
      celula('ct-c-temp', null,
        h('span', { class: 'ct-temp' }, etiquetaTemp(c.classificacao),
          pontos || temperatura(c.classificacao) !== 'indefinido'
            ? h('span', { class: 'ct-pontos', title: pontos + ' pontos de 100' }, String(pontos), h('small', { text: ' pts' }))
            : null)),
      celula('ct-c-sit', null, seloSituacao(c)),
      celula('ct-c-etqs', null, etiquetas.length ? h('span', { class: 'ct-tags' },
        etiquetas.slice(0, 3).map((e) => h('span', { class: 'ct-tag', text: e })),
        etiquetas.length > 3 ? h('span', { class: 'ct-tag ct-tag-mais', title: etiquetas.slice(3).join(', ') },
          '+' + (etiquetas.length - 3), h('span', { class: 'so-leitor', text: ' etiquetas' })) : null) : null),
      celula('ct-c-data', 'Primeiro contato',
        h('time', { datetime: c.criado_em, title: dataCompleta(c.criado_em), text: dataCurta(c.criado_em) })),
      celula('ct-c-data', 'Última mensagem', p.ultima),
      celula('ct-c-num ct-num', 'Mensagens', String(mensagens)));
  }

  // encaixar deixa as linhas na ordem pedida, mexendo só no que saiu do lugar.
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

  function pintarLinhas(contatos) {
    const visiveis = contatos.slice(0, tela.limite);
    const vivos = new Set();
    const ordem = [];
    for (const c of visiveis) {
      vivos.add(c.id);
      let p = tela.linhas.get(c.id);
      if (!p) { p = criarLinha(c.id); tela.linhas.set(c.id, p); }
      pintarLinha(p, c);
      ordem.push(p.tr);
    }
    for (const [id, p] of Array.from(tela.linhas)) {
      if (!vivos.has(id)) { p.tr.remove(); tela.linhas.delete(id); }
    }
    encaixar(tela.tbody, ordem);

    const faltam = contatos.length - visiveis.length;
    const marca = String(faltam);
    if (tela.mais.dataset.marca !== marca) {
      tela.mais.dataset.marca = marca;
      tela.mais.hidden = !faltam;
      trocar(tela.mais, faltam ? [
        h('span', { text: 'Mostrando ' + visiveis.length + ' de ' + contatos.length + '.' }),
        botao('Mostrar mais ' + Math.min(PASSO, faltam), {
          classe: 'btn-suave',
          aoClicar: () => { tela.limite += PASSO; pintarLinhas((memoria().dados || {}).contatos || []); },
        })] : []);
    }
  }

  // ---------- Baixar ----------

  // O menu é montado a cada abertura, então os endereços levam sempre o filtro de agora.
  function montarMenuBaixar(painel, fechar) {
    const d = memoria().dados || {};
    const filtrado = temFiltro();
    const total = Number(d.total) || 0;
    const filtros = consulta();
    painel.setAttribute('role', 'menu');
    painel.setAttribute('aria-label', 'Baixar contatos');
    const item = (arquivo, nomeIcone, titulo, dica) => {
      const a = h('a', {
        class: 'sus-item sus-item-menu', role: 'menuitem', 'data-sus-item': '', 'data-sus-texto': titulo,
        href: '/' + arquivo + filtros, download: arquivo,
      },
        icone(nomeIcone),
        h('span', { class: 'sus-textos' }, h('span', { class: 'sus-texto', text: titulo }), h('span', { class: 'sus-dica', text: dica })));
      // O menu fecha depois que o navegador começou a baixar.
      a.addEventListener('click', () => setTimeout(fechar, 0));
      return a;
    };
    painel.append(
      item('contatos.csv', 'contatos', 'Contatos em planilha', 'Nome, telefone, temperatura, situação, etiquetas e datas.'),
      item('leads.csv', 'arquivo', 'Relatório completo em planilha', 'O que cada pessoa precisa, orçamento, prazo, quem decide e o resumo da conversa.'),
      h('p', { class: 'ct-menu-nota' },
        filtrado
          ? 'Vai só o que está no filtro: ' + plural(total, 'contato', 'contatos') + '.'
          : 'Vão todos os contatos. Para baixar só uma parte, filtre a lista antes.',
        ' Abre no Excel e no Google Planilhas.'));
  }

  TELAS.contatos = { montar, atualizar, sair };
})();
