'use strict';
// Aba Contatos: todo mundo que já escreveu para o negócio. Serve para ver, achar, baixar
// e acompanhar cada pessoa.
//
// São duas telas. "#contatos" é a lista: a busca e os filtros são feitos pelo programa (a
// cada mudança a tela pede /api/contatos de novo), as duas planilhas levam o mesmo filtro
// no endereço e a lista se atualiza sozinha, sem piscar e sem perder a rolagem.
// "#contatos/<id>" é a ficha da pessoa: dados, o que a IA descobriu, etiquetas, anotações
// da equipe, agendamentos e a linha do tempo (/api/contato). O botão Voltar do navegador
// retorna à lista do jeito que ela estava.
// Regra de ouro: nada que vem da API vira HTML. Tudo entra como texto.

(function () {
  let tela = null;           // peças da lista montada
  let ficha = null;          // peças da ficha aberta
  let saiuDaFicha = -1e9;    // quando a ficha foi fechada (para a lista voltar ao mesmo lugar)

  const ATRASO_BUSCA = 250;  // espera a pessoa parar de digitar
  const PASSO = 200;         // quantas linhas entram na página de cada vez
  const PASSO_HISTORICO = 30;   // quantos registros da linha do tempo entram de cada vez
  const MAX_ANOTACOES = 4000;   // o mesmo limite do programa
  const AVISO_ANOTACOES = 3600; // a partir daqui a tela mostra quantas letras já foram
  const BATIDAS_FICHA = 20;     // sem novidade, a ficha é pedida de novo a cada minuto
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
  const ETIQUETA = ['M12.6 2.6A2 2 0 0 0 11.2 2H4a2 2 0 0 0-2 2v7.2a2 2 0 0 0 .6 1.4l8.7 8.7a2.4 2.4 0 0 0 3.4 0l6.6-6.6a2.4 2.4 0 0 0 0-3.4Z', 'M7.5 7.5h.01'];
  const HISTORICO = ['M3 12a9 9 0 1 0 9-9 9.75 9.75 0 0 0-6.74 2.74L3 8', 'M3 3v5h5M12 7v5l4 2'];
  const SABEMOS = ['M9 2h6a1 1 0 0 1 1 1v2a1 1 0 0 1-1 1H9a1 1 0 0 1-1-1V3a1 1 0 0 1 1-1Z', 'M16 4h2a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2h2M12 11h4M12 16h4M8 11h.01M8 16h.01'];
  const AGENDA_X = ['M8 2v4M16 2v4M3 10h18', 'M5 4h14a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2Z', 'm10 14 4 4M14 14l-4 4'];

  // Os registros da linha do tempo que a tela conhece. Cada um tem um nome curto (para o
  // leitor de tela e a dica), um ícone e a frase usada quando o registro vem sem texto.
  // Tipo que não está aqui aparece do mesmo jeito, com o texto que veio do programa.
  const EVENTOS = {
    primeiro_contato: { rotulo: 'Primeiro contato', icone: 'conversas', falta: 'Primeiro contato pelo WhatsApp' },
    transbordo: { rotulo: 'Transferência para a equipe', icone: 'contatos', falta: 'O assistente transferiu a conversa para a equipe' },
    assumiu: { rotulo: 'Atendimento iniciado', icone: 'assumir', falta: 'A equipe iniciou o atendimento' },
    devolveu: { rotulo: 'Conversa devolvida ao assistente', icone: 'assistente', falta: 'A equipe devolveu a conversa ao assistente' },
    agendou: { rotulo: 'Horário marcado', icone: 'agenda', falta: 'Horário marcado' },
    remarcou: { rotulo: 'Horário remarcado', icone: 'agenda', falta: 'Horário remarcado' },
    cancelou: { rotulo: 'Horário cancelado', tracos: AGENDA_X, falta: 'Horário cancelado' },
    etiqueta: { rotulo: 'Mudança de etiqueta', tracos: ETIQUETA, falta: 'Etiquetas alteradas' },
  };
  const EVENTO_QUALQUER = { rotulo: 'Registro', icone: 'info', falta: 'Registro interno' };

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

  // mudou só refaz um pedaço da tela quando o que ele mostra mudou.
  function mudou(dono, chave, valor, fazer) {
    if (dono.marcas[chave] === valor) return false;
    dono.marcas[chave] = valor;
    fazer();
    return true;
  }

  function nomeDe(c) { return (c && (c.exibir || c.nome || c.nome_whatsapp || c.telefone_bonito)) || 'Sem nome'; }

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
  // Também guarda de onde a pessoa saiu da lista (volta) e as anotações que ela escreveu
  // em uma ficha e ainda não salvou (rascunhos, por id do contato).
  function memoria() {
    if (!S.contatos) S.contatos = { q: '', situacao: '', temperatura: '', etiqueta: '', ordem: 'recentes', dados: null };
    if (!S.contatos.rascunhos) S.contatos.rascunhos = {};
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

  // montar é chamado pela casca. "#contatos" abre a lista; "#contatos/<id>" abre a ficha.
  function montar(main, sub) {
    if (tela || ficha) sair();
    if (sub) montarFicha(main, sub);
    else montarLista(main);
  }

  function montarLista(main) {
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
          typeof ETIQUETAS !== 'undefined'
            ? h('button', { type: 'button', class: 'link-baixar ct-etiquetas', onclick: () => ETIQUETAS.abrir() }, icone('lapis'), h('span', { text: 'Etiquetas' }))
            : null,
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
    // Quem volta de uma ficha encontra a lista como deixou: mesma quantidade de linhas,
    // mesma altura da página e o foco na pessoa que tinha aberto.
    const volta = m.volta;
    m.volta = null;
    const voltando = !!volta && !!m.dados && performance.now() - saiuDaFicha < 500;
    if (voltando) {
      tela.limite = Math.max(PASSO, Number(volta.limite) || PASSO);
      tela.consultaPintada = consulta();
    }
    pintarOpcoes();
    pintarLimpar();
    pintar();
    buscar();
    if (voltando) {
      const minha = tela;
      // A casca leva a página para o topo logo depois de montar a tela: a volta vem em seguida.
      setTimeout(() => {
        if (tela !== minha) return;
        window.scrollTo(0, Number(volta.rolagem) || 0);
        const linha = minha.linhas.get(volta.id);
        if (linha) linha.tr.focus({ preventScroll: true });
      }, 0);
    }
  }

  function sair() {
    if (ficha) sairDaFicha();
    if (!tela) return;
    clearTimeout(tela.esperaBusca);
    clearTimeout(tela.esperaOcupado);
    tela.menu.fechar();
    for (const el of [tela.situacao, tela.temperaturaEl, tela.etiqueta, tela.ordem]) el.fechar();
    tela = null;
  }

  // atualizar é chamado pela casca a cada busca de /api/estado.
  function atualizar() {
    if (ficha) { atualizarFicha(); return; }
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
      h('p', { class: 'so-leitor', id: 'ct-dica', text: 'Aperte Enter em uma linha para abrir a ficha da pessoa.' }),
      h('div', { class: 'ct-tabela-caixa' }, tabela), mais);
    tela.tabela = tabela;
    tela.tbody = tbody;
    tela.mais = mais;
  }

  // Clicar em um contato abre a ficha dele. A conversa fica a um botão de distância, na ficha.
  function abrirFicha(id) {
    memoria().volta = { id, rolagem: window.scrollY, limite: tela ? tela.limite : PASSO };
    location.hash = '#contatos/' + encodeURIComponent(id);
  }

  function criarLinha(id) {
    const tr = h('tr', { class: 'ct-linha', role: 'row', tabindex: '0', 'aria-describedby': 'ct-dica' });
    tr.addEventListener('click', (ev) => {
      if (ev.target instanceof Element && ev.target.closest('button, a')) return;
      // Quem está selecionando um texto da linha não quer sair da tela.
      const selecao = window.getSelection ? String(window.getSelection()) : '';
      if (selecao && tr.contains(window.getSelection().anchorNode)) return;
      abrirFicha(id);
    });
    tr.addEventListener('keydown', (ev) => {
      if (ev.key === 'Enter' && ev.target === tr) { ev.preventDefault(); abrirFicha(id); }
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

  // ==================== Ficha do contato ====================
  //
  // Abre em "#contatos/<id>". Tudo vem de /api/contato: os dados da pessoa, os agendamentos
  // e a linha do tempo, que o programa lê do banco (a conversa em uso guarda só as últimas
  // mensagens). A cada batida da casca a ficha confere se algo mudou e só refaz o pedaço
  // que mudou, para não atrapalhar quem está escrevendo uma anotação ou uma etiqueta.

  let contadorFicha = 0;

  function idDaRota(sub) {
    try { return decodeURIComponent(sub); } catch (e) { return sub; }
  }

  // resumoDe acha o contato no que a página já tem, para o topo aparecer na hora.
  function resumoDe(id) {
    const d = memoria().dados;
    return ((S.estado && S.estado.leads) || []).find((l) => l.id === id)
      || ((d && d.contatos) || []).find((c) => c.id === id) || null;
  }

  function pontosDe(c) { return Math.max(0, Math.min(100, Number(c.pontuacao) || 0)); }

  // quandoFoi: Hoje às 14:02, Ontem às 09:10 ou 12/11/2026 às 14:02.
  function quandoFoi(iso) {
    const completa = dataCompleta(iso);
    if (!completa) return '';
    const dia = diaRotulo(iso);
    return dia === 'Hoje' || dia === 'Ontem' || dia === 'Amanhã' ? dia + ' às ' + horaCurta(iso) : completa;
  }

  // quandoSera: como quandoFoi, com o dia da semana (qua, 12/11/2026 às 14:00).
  function quandoSera(iso) {
    const completa = dataCompleta(iso);
    if (!completa) return '';
    const dia = diaRotulo(iso);
    if (dia === 'Hoje' || dia === 'Ontem' || dia === 'Amanhã') return dia + ' às ' + horaCurta(iso);
    return DIAS_CURTOS[new Date(iso).getDay()] + ', ' + completa;
  }

  // cartaoFicha monta um bloco da ficha: título com ícone, uma frase de apoio e o corpo.
  function cartaoFicha(titulo, simbolo, descricao) {
    const n = ++contadorFicha;
    const idTitulo = 'fc-titulo-' + n;
    const idDescricao = descricao ? 'fc-descricao-' + n : null;
    const corpo = h('div', { class: 'fc-corpo' });
    const el = h('section', { class: 'cartao fc-cartao', 'aria-labelledby': idTitulo },
      h('div', { class: 'fc-cabeca' },
        h('h2', { id: idTitulo }, simbolo, h('span', { text: titulo })),
        descricao ? h('p', { class: 'fc-descricao', id: idDescricao, text: descricao }) : null),
      corpo);
    return { el, corpo, idTitulo, idDescricao };
  }

  // linhaDado monta um par "rótulo e valor" da ficha. Valor vazio vira um aviso discreto.
  function linhaDado(rotulo, valor, falta) {
    const tem = valor != null && String(valor).trim() !== '';
    return h('div', { class: 'fc-dado' },
      h('dt', { text: rotulo }),
      h('dd', { class: tem ? null : 'falta', text: tem ? String(valor) : (falta || 'Ainda não disse') }));
  }

  function montarFicha(main, sub) {
    const id = idDaRota(sub);
    const topo = h('header', { class: 'fc-topo' });
    const corpo = h('div', { class: 'fc-conteudo' });
    const raiz = h('div', { class: 'tela-ficha' },
      h('a', { class: 'fc-voltar', href: '#contatos' }, icone('voltar'), h('span', { text: 'Voltar para Contatos' })),
      topo, corpo);
    main.appendChild(raiz);
    ficha = {
      id, raiz, topo, corpo, dados: null, pecas: null, marcas: {},
      pedido: 0, assinatura: undefined, batidas: 0, limiteHistorico: PASSO_HISTORICO,
    };
    const resumo = resumoDe(id);
    if (resumo) pintarTopo(resumo);
    trocar(corpo, carregandoEl('Carregando a ficha'));
    carregarFicha();
  }

  // rascunhoDe devolve a anotação que a pessoa escreveu nesta ficha e ainda não salvou.
  function rascunhoDe(id) {
    const r = memoria().rascunhos[id];
    return typeof r === 'string' ? r : null;
  }

  function sairDaFicha() {
    const f = ficha;
    ficha = null;
    saiuDaFicha = performance.now();
    if (!f) return;
    // Saiu com anotação por salvar: o texto fica guardado e a pessoa é avisada. Só não
    // avisa quando a mesma ficha vai ser montada de novo (a casca refaz a tela às vezes).
    const destino = location.hash.replace(/^#\/?/, '').split('/');
    const mesma = destino[0] === 'contatos' && !!destino[1] && idDaRota(destino[1]) === f.id;
    if (!mesma && rascunhoDe(f.id) != null && f.dados && f.dados.contato) {
      toast('As anotações de ' + nomeDe(f.dados.contato) + ' não foram salvas. O texto fica guardado: volte à ficha para salvar.', 'erro');
    }
  }

  // assinaturaFicha muda quando qualquer coisa do contato muda na situação geral.
  function assinaturaFicha(id) {
    const leads = (S.estado && S.estado.leads) || null;
    if (!leads) return undefined;
    const l = leads.find((x) => x.id === id);
    return l ? JSON.stringify(l) : 'sumiu';
  }

  // atualizarFicha roda a cada batida da casca. Com novidade no contato (mensagem,
  // etiqueta, transferência, horário), ou de minuto em minuto, pede a ficha de novo.
  function atualizarFicha() {
    const f = ficha;
    if (!f || S.semConexao) return;
    if (!f.dados) {
      // O topo aparece assim que a situação geral chega, mesmo com a ficha ainda carregando.
      const resumo = resumoDe(f.id);
      if (resumo && !f.erro) pintarTopo(resumo);
      return;
    }
    const nova = assinaturaFicha(f.id);
    f.batidas++;
    if (f.assinatura === undefined) f.assinatura = nova;
    if ((nova !== undefined && nova !== f.assinatura) || f.batidas >= BATIDAS_FICHA) {
      f.assinatura = nova;
      f.batidas = 0;
      carregarFicha(true);
      return;
    }
    pintarFicha(); // nada mudou: só os tempos ("há 5 min", "Hoje") andam
  }

  // carregarFicha pede a ficha ao programa. "emSegundoPlano" é a atualização sozinha: se
  // ela falhar, o que está na tela fica onde está e a próxima batida tenta de novo.
  async function carregarFicha(emSegundoPlano) {
    const f = ficha;
    if (!f) return;
    const pedido = ++f.pedido;
    if (emSegundoPlano !== true) f.corpo.setAttribute('aria-busy', 'true');
    let dados = null;
    let erro = null;
    try {
      dados = await api.get('/api/contato?id=' + encodeURIComponent(f.id));
    } catch (e) {
      erro = e;
    }
    if (ficha !== f || f.pedido !== pedido) return;
    f.corpo.removeAttribute('aria-busy');
    if (erro) {
      const sumiu = assinaturaFicha(f.id) === 'sumiu';
      if (emSegundoPlano === true && f.dados && !sumiu) {
        f.assinatura = null; // tenta de novo na próxima batida
        return;
      }
      // O contato foi apagado: a anotação que estava por salvar não tem mais para onde ir.
      if (sumiu) delete memoria().rascunhos[f.id];
      mostrarFalhaDaFicha(erro);
      return;
    }
    f.erro = null;
    f.dados = dados || {};
    if (!f.dados.contato) {
      mostrarFalhaDaFicha(new Error('A ficha veio sem os dados do contato. Tente de novo.'));
      return;
    }
    if (f.assinatura === undefined || f.assinatura === null) f.assinatura = assinaturaFicha(f.id);
    if (!f.pecas) montarPecas();
    pintarFicha();
  }

  function mostrarFalhaDaFicha(erro) {
    const f = ficha;
    f.erro = erro;
    f.dados = null;
    f.pecas = null;
    f.marcas = {};
    trocar(f.topo);
    trocar(f.corpo, h('div', { class: 'cartao' },
      vazio('alerta', 'Não consegui abrir esta ficha', (erro && erro.message) || 'Tente de novo em alguns segundos.',
        h('a', { class: 'btn btn-primario', href: '#contatos' }, icone('voltar'), h('span', { text: 'Voltar para Contatos' })),
        botao('Tentar de novo', {
          classe: 'btn-suave',
          aoClicar: () => {
            if (!ficha) return;
            trocar(ficha.corpo, carregandoEl('Carregando a ficha'));
            return carregarFicha();
          },
        }))));
  }

  // ---------- Topo: quem é, temperatura, situação e o botão da conversa ----------

  function pintarTopo(c) {
    const f = ficha;
    const nome = nomeDe(c);
    const apelido = c.nome_whatsapp && c.nome_whatsapp !== nome ? String(c.nome_whatsapp) : '';
    const fone = c.telefone_bonito || c.telefone || '';
    const pontos = pontosDe(c);
    const temp = temperatura(c.classificacao);
    const marca = JSON.stringify([nome, apelido, fone, !!c.tem_foto, c.foto_versao, temp, pontos, c.status, c.atendente]);
    mudou(f, 'topo', marca, () => {
      // Quem estava com o foco em um botão do topo continua nele depois da troca.
      const ativo = document.activeElement;
      const foco = ativo instanceof Element && f.topo.contains(ativo) ? ativo.getAttribute('data-foco') : null;

      const copiar = h('button', {
        type: 'button', class: 'ct-copiar', 'data-foco': 'copiar', 'aria-label': 'Copiar o telefone de ' + nome, title: 'Copiar telefone',
        onclick: async () => {
          if (await TELAS.conversas.copiarTexto(fone)) toast('Telefone copiado.');
          else toast('Não consegui copiar. Selecione o número e copie com o teclado.', 'erro');
        },
      }, desenho(COPIAR));
      const preenchido = h('span');
      preenchido.style.width = pontos + '%';

      trocar(f.topo,
        TELAS.conversas.avatar(c, 'fc-avatar'),
        h('div', { class: 'fc-quem' },
          h('h1', { class: 'fc-nome', text: nome }),
          h('p', { class: 'fc-sub' },
            fone ? h('span', { class: 'ct-fone' }, h('span', { text: fone }), copiar) : null,
            apelido ? h('span', { class: 'fc-apelido', text: 'No WhatsApp: ' + apelido }) : null),
          h('div', { class: 'fc-selos' },
            etiquetaTemp(c.classificacao),
            h('span', { class: 'fc-pontos', 'data-temp': temp },
              h('span', { class: 'fc-pontos-texto' }, h('strong', { text: String(pontos) }), ' pontos de 100'),
              h('span', { class: 'barra', 'aria-hidden': 'true' }, preenchido)),
            seloSituacao(c))),
        h('div', { class: 'fc-acoes' },
          h('a', { class: 'btn btn-primario fc-abrir', 'data-foco': 'conversa', href: '#conversas/' + encodeURIComponent(c.id || f.id) },
            icone('conversas'), h('span', { text: 'Abrir conversa' }))));

      if (foco) {
        const alvo = f.topo.querySelector('[data-foco="' + (foco === 'copiar' ? 'copiar' : 'conversa') + '"]');
        if (alvo) alvo.focus({ preventScroll: true });
      }
    });
  }

  // ---------- Blocos ----------

  function montarPecas() {
    const f = ficha;
    const p = {};
    p.dados = cartaoFicha('Dados', icone('ficha'));
    p.sabemos = cartaoFicha('O que sabemos', desenho(SABEMOS), 'A IA preenche estes campos durante a conversa.');
    p.etiquetas = cartaoFicha('Etiquetas', desenho(ETIQUETA));
    p.notas = cartaoFicha('Anotações da equipe', icone('lapis'), 'Use para o acompanhamento. Só a equipe vê: nada daqui vai para o cliente.');
    p.agenda = cartaoFicha('Agendamentos', icone('agenda'));
    p.historico = cartaoFicha('Histórico', desenho(HISTORICO), 'Do mais novo para o mais antigo. O cliente não vê nada daqui.');
    p.notas.el.classList.add('fc-cartao-notas');
    f.pecas = p;
    f.etq = { campo: null, ajuda: h('p', { class: 'ajuda fc-ajuda-etq' }), lugar: h('div'), salvando: 0, fila: Promise.resolve(), visto: null, sugestoes: null };
    trocar(p.etiquetas.corpo, f.etq.lugar, f.etq.ajuda);
    montarNotas();
    trocar(f.corpo, h('div', { class: 'fc-colunas' },
      h('div', { class: 'fc-coluna fc-coluna-quem' }, p.dados.el, p.sabemos.el, p.etiquetas.el),
      h('div', { class: 'fc-coluna fc-coluna-vida' }, p.notas.el, p.agenda.el, p.historico.el)));
  }

  function pintarFicha() {
    const f = ficha;
    if (!f || !f.dados || !f.pecas) return;
    const d = f.dados;
    const c = d.contato;
    pintarTopo(c);
    pintarDados(c);
    pintarSabemos(c);
    pintarEtiquetas(c);
    pintarNotas(c);
    pintarAgendamentos(d);
    pintarHistorico(d);
  }

  function pintarDados(c) {
    const f = ficha;
    const proximo = c.proximo_agendamento;
    const ultima = dataCompleta(c.ultima_msg);
    // "há 5 min" ou "ontem" ajudam; para datas mais antigas a data completa já basta.
    const relativo = tempoRelativo(c.ultima_msg);
    const perto = /^(agora|há |ontem)/.test(relativo) ? ' (' + relativo + ')' : '';
    const mensagens = Number(c.total_msgs) || 0;
    const comEquipe = c.status === 'aguardando humano' || c.status === 'em atendimento';
    const linhas = [
      ['Nome', c.nome, 'Ainda não disse'],
      ['Nome no WhatsApp', c.nome_whatsapp, 'Sem nome no WhatsApp'],
      ['Telefone', c.telefone_bonito || c.telefone, 'Sem telefone'],
      ['Primeiro contato', dataCompleta(c.criado_em), 'Sem data'],
      ['Última mensagem', ultima ? ultima + perto : '', 'Nenhuma mensagem ainda'],
      ['Total de mensagens', mensagens ? plural(mensagens, 'mensagem', 'mensagens') : '', 'Nenhuma mensagem ainda'],
      ['Próximo agendamento', proximo ? [proximo.servico, quandoSera(proximo.inicio)].filter(Boolean).join(', ') : '', 'Nenhum horário marcado'],
    ];
    if (comEquipe && c.motivo) linhas.push(['Motivo da transferência', c.motivo, '']);
    mudou(f, 'dados', JSON.stringify(linhas), () => {
      trocar(f.pecas.dados.corpo, h('dl', { class: 'fc-dados' }, linhas.map((l) => linhaDado(l[0], l[1], l[2]))));
    });
  }

  function pintarSabemos(c) {
    const f = ficha;
    const marca = JSON.stringify([c.resumo, c.necessidade, c.orcamento, c.decisor, c.prazo, c.etapa]);
    mudou(f, 'sabemos', marca, () => {
      trocar(f.pecas.sabemos.corpo,
        h('dl', { class: 'fc-dados fc-dados-empilhados' },
          linhaDado('O que precisa', c.necessidade),
          linhaDado('Orçamento', c.orcamento),
          linhaDado('Quem decide', c.decisor),
          linhaDado('Prazo', c.prazo),
          linhaDado('Etapa', c.etapa, 'Começando'),
          linhaDado('Resumo', c.resumo, 'Ainda sem resumo. Ele aparece depois de algumas mensagens.')));
    });
  }

  // ---------- Etiquetas (o mesmo campo e a mesma rota da conversa) ----------

  function limparEtiqueta(t) { return String(t).trim().replace(/\s+/g, ' ').slice(0, 30); }

  function pintarEtiquetas(c) {
    const f = ficha;
    const e = f.etq;
    const doServidor = (c.etiquetas || []).map(String);
    const sugestoes = ((S.estado && S.estado.etiquetas) || []).map(String).join('\n');
    const digitando = !!e.campo && e.campo.contains(document.activeElement);
    if (!e.campo || (e.sugestoes !== sugestoes && !digitando && !e.salvando)) {
      e.sugestoes = sugestoes;
      e.visto = JSON.stringify(doServidor);
      e.campo = chips(doServidor, {
        rotulo: 'Adicionar etiqueta', placeholder: 'Digite e aperte Enter',
        teclas: [',', ';', 'Tab', 'Enter'], limpar: limparEtiqueta, sugestoes: sugestoes ? sugestoes.split('\n') : [], max: 12,
        aoMudar: (lista) => { marcarEtiquetasDaIA(); salvarEtiquetas(f, lista); },
      });
      trocar(e.lugar, e.campo);
    } else {
      const marca = JSON.stringify(doServidor);
      if (marca !== e.visto && !e.salvando && !digitando) {
        e.visto = marca;
        e.campo.definir(doServidor);
      }
    }
    marcarEtiquetasDaIA();
  }

  // marcarEtiquetasDaIA põe um sinal nas etiquetas que a IA pôs e explica o sinal embaixo.
  // O campo redesenha as etiquetas a cada mudança, então a marca é posta de novo depois.
  function marcarEtiquetasDaIA() {
    const f = ficha;
    if (!f || !f.etq || !f.etq.campo || !f.dados || !f.dados.contato) return;
    const daIA = new Set((f.dados.contato.etiquetas_ia || []).map(String));
    let tem = false;
    f.etq.campo.querySelectorAll('.chip').forEach((chip) => {
      if (chip.classList.contains('fc-chip-ia')) { tem = true; return; }
      const texto = chip.querySelector(':scope > span');
      if (!texto || !daIA.has(texto.textContent)) return;
      tem = true;
      chip.classList.add('fc-chip-ia');
      chip.title = 'Posta pela IA';
      chip.insertBefore(icone('ia'), texto);
      texto.after(h('span', { class: 'so-leitor', text: ', posta pela IA' }));
    });
    const marca = tem ? 'ia' : 'sem';
    if (f.etq.ajuda.dataset.marca === marca) return;
    f.etq.ajuda.dataset.marca = marca;
    trocar(f.etq.ajuda, tem
      ? [icone('ia'), h('span', { text: 'Posta pela IA durante o atendimento. Se você tirar uma, ela não volta neste contato.' })]
      : [h('span', { text: 'A IA põe e troca etiquetas sozinha durante o atendimento, usando as que já existem. Você também pode pôr as suas.' })]);
  }

  // salvarEtiquetas grava a lista a cada mudança, uma gravação de cada vez e na ordem.
  function salvarEtiquetas(f, lista) {
    const e = f.etq;
    e.salvando++;
    e.fila = e.fila.then(async () => {
      try {
        const r = await api.post('/api/lead/etiquetas', { id: f.id, etiquetas: lista });
        const limpas = Array.isArray(r.etiquetas) ? r.etiquetas.map(String) : lista;
        e.visto = JSON.stringify(limpas);
        if (f.dados && f.dados.contato) {
          f.dados.contato.etiquetas = limpas.slice();
          if (Array.isArray(r.etiquetas_ia)) f.dados.contato.etiquetas_ia = r.etiquetas_ia.map(String);
        }
        if (ficha === f && e.salvando === 1 && JSON.stringify(limpas) !== JSON.stringify(lista)) e.campo.definir(limpas);
        if (ficha === f) marcarEtiquetasDaIA();
      } catch (erro) {
        toast(erro.message || 'Não consegui salvar as etiquetas. Tente de novo.', 'erro');
        e.visto = null; // na próxima busca o campo volta ao que está salvo
      }
      e.salvando--;
      // A situação geral é buscada de novo. Com a etiqueta nova nela, a ficha se atualiza
      // sozinha (a mudança de etiqueta também entra na linha do tempo).
      if (!e.salvando) pulsar();
    });
  }

  // ---------- Anotações da equipe ----------

  function montarNotas() {
    const f = ficha;
    const cartao = f.pecas.notas;
    const n = { salvo: '', pronto: false, conflito: false, salvou: false };
    n.area = h('textarea', {
      class: 'fc-notas', rows: 7, maxLength: MAX_ANOTACOES, spellcheck: 'true',
      placeholder: 'Ex.: Ligar na sexta para confirmar o orçamento. Prefere falar à tarde.',
      'aria-labelledby': cartao.idTitulo, 'aria-describedby': cartao.idDescricao,
    });
    n.situacao = h('p', { class: 'situacao', 'aria-live': 'polite' });
    n.contador = h('p', { class: 'fc-contador', hidden: true });
    n.conflitoEl = h('p', { class: 'nota fc-conflito', role: 'status', hidden: true }, icone('info'),
      h('span', { text: 'Outra pessoa mudou estas anotações enquanto você escrevia. Se você salvar, fica valendo o seu texto.' }));
    n.salvar = h('button', { type: 'button', class: 'btn btn-primario', disabled: true }, h('span', { text: 'Salvar anotações' }));
    n.salvar.addEventListener('click', async () => {
      await ocupar(n.salvar, () => salvarNotas(f));
      if (ficha === f) refletirNotas();
    });
    n.area.addEventListener('input', () => { n.salvou = false; guardarRascunho(f); refletirNotas(); });
    f.notas = n;
    trocar(cartao.corpo, n.area, n.conflitoEl, h('div', { class: 'acoes fc-notas-pe' }, n.salvar, n.situacao, n.contador));
  }

  function guardarRascunho(f) {
    const m = memoria();
    if (f.notas.area.value !== f.notas.salvo) m.rascunhos[f.id] = f.notas.area.value;
    else delete m.rascunhos[f.id];
  }

  function pintarNotas(c) {
    const f = ficha;
    const n = f.notas;
    const doServidor = String(c.anotacoes == null ? '' : c.anotacoes);
    if (!n.pronto) {
      n.pronto = true;
      n.salvo = doServidor;
      const rascunho = rascunhoDe(f.id);
      n.area.value = rascunho != null ? rascunho : doServidor;
      guardarRascunho(f);
    } else if (doServidor !== n.salvo) {
      // O texto mudou no programa (outra pessoa da equipe salvou). Quem não estava
      // escrevendo recebe o texto novo; quem estava fica com o seu e é avisado.
      const escrevendo = n.area.value !== n.salvo;
      n.salvo = doServidor;
      if (!escrevendo) n.area.value = doServidor;
      else if (n.area.value !== doServidor && !n.salvando) n.conflito = true;
      guardarRascunho(f);
    }
    refletirNotas();
  }

  function refletirNotas() {
    const n = ficha && ficha.notas;
    if (!n) return;
    const sujo = n.area.value !== n.salvo;
    if (!sujo) n.conflito = false;
    const texto = sujo ? 'Alterações não salvas' : (n.salvou ? 'Anotações salvas' : '');
    if (n.situacao.textContent !== texto) n.situacao.textContent = texto;
    n.situacao.classList.toggle('sujo', sujo);
    if (!n.salvando) n.salvar.disabled = !sujo;
    n.conflitoEl.hidden = !n.conflito;
    const letras = n.area.value.length;
    n.contador.hidden = letras < AVISO_ANOTACOES;
    const conta = letras + ' de ' + MAX_ANOTACOES + ' letras';
    if (n.contador.textContent !== conta) n.contador.textContent = conta;
  }

  async function salvarNotas(f) {
    const n = f.notas;
    const enviado = n.area.value;
    n.salvando = true;
    let r;
    try {
      r = await api.post('/api/contato/anotacoes', { id: f.id, anotacoes: enviado });
    } finally {
      n.salvando = false;
    }
    const gravado = String(r.anotacoes == null ? enviado : r.anotacoes);
    n.salvo = gravado;
    if (f.dados && f.dados.contato) f.dados.contato.anotacoes = gravado;
    // O programa tira os espaços das pontas: se a pessoa não escreveu mais nada, o campo acompanha.
    if (n.area.value === enviado) n.area.value = gravado;
    n.conflito = false;
    n.salvou = true;
    guardarRascunho(f);
    toast('Anotações salvas.');
  }

  // ---------- Agendamentos ----------

  function fimDoHorario(a) {
    return new Date(a.inicio).getTime() + (Number(a.duracao_min) || 0) * 60000;
  }

  function itemAgendamento(a, grupo) {
    const cancelado = a.status === 'cancelado';
    let selo;
    if (cancelado) selo = h('span', { class: 'etiqueta etiqueta-perigo', text: 'Cancelado' });
    else if (grupo === 'proximos') selo = h('span', { class: 'etiqueta etiqueta-ok' }, icone('check'), 'Marcado');
    else if (a.status === 'marcado') selo = h('span', { class: 'etiqueta', text: 'Já passou' });
    else selo = h('span', { class: 'etiqueta', text: String(a.status || 'Sem situação') });
    const detalhes = [
      a.duracao_min ? a.duracao_min + ' min' : '',
      a.origem === 'assistente' ? 'Marcado pelo assistente' : (a.origem === 'painel' ? 'Marcado pela equipe' : ''),
    ].filter(Boolean).join(', ');
    return h('li', { class: 'fc-ag' + (cancelado ? ' fc-ag-cancelado' : '') },
      h('div', { class: 'fc-ag-textos' },
        h('time', { class: 'fc-ag-quando', datetime: a.inicio, text: quandoSera(a.inicio) }),
        h('span', { class: 'fc-ag-servico', text: a.servico || 'Serviço não informado' }),
        detalhes ? h('span', { class: 'fc-ag-detalhe', text: detalhes }) : null,
        a.obs ? h('span', { class: 'fc-ag-detalhe fc-ag-obs', text: 'Observação: ' + a.obs }) : null),
      selo);
  }

  function pintarAgendamentos(d) {
    const f = ficha;
    const todos = Array.isArray(d.agendamentos) ? d.agendamentos : [];
    const agora = Date.now();
    const tempo = (a) => new Date(a.inicio).getTime() || 0;
    const proximos = todos.filter((a) => a.status === 'marcado' && fimDoHorario(a) >= agora).sort((a, b) => tempo(a) - tempo(b));
    const outros = todos.filter((a) => !proximos.includes(a)).sort((a, b) => tempo(b) - tempo(a));
    const marca = JSON.stringify([proximos, outros].map((lista) => lista.map((a) => [a.id, a.status, a.servico, a.duracao_min, a.origem, a.obs, quandoSera(a.inicio)])));
    mudou(f, 'agenda', marca, () => {
      const grupo = (titulo, lista, nome) => (lista.length ? [
        h('h3', { class: 'fc-grupo', text: titulo }),
        h('ul', { class: 'fc-ags' }, lista.map((a) => itemAgendamento(a, nome))),
      ] : null);
      trocar(f.pecas.agenda.corpo,
        todos.length ? [
          grupo('Próximos', proximos, 'proximos') || h('p', { class: 'fc-falta', text: 'Nenhum horário pela frente.' }),
          grupo('Anteriores e cancelados', outros, 'outros'),
        ] : h('p', { class: 'fc-falta', text: 'Nenhum horário marcado com este contato.' }),
        h('a', { class: 'cv-link fc-link', href: '#agenda' }, 'Abrir a agenda'));
    });
  }

  // ---------- Histórico: a linha do tempo do contato ----------

  function itemHistorico(ev) {
    const tipo = Object.prototype.hasOwnProperty.call(EVENTOS, ev.evento) ? ev.evento : '';
    const info = tipo ? EVENTOS[tipo] : EVENTO_QUALQUER;
    const texto = ev.texto && String(ev.texto).trim() ? String(ev.texto) : info.falta;
    return h('li', { class: 'fc-ev', 'data-tipo': tipo || 'outro' },
      h('span', { class: 'fc-ev-icone', title: info.rotulo }, info.tracos ? desenho(info.tracos) : icone(info.icone)),
      h('div', { class: 'fc-ev-corpo' },
        h('p', { class: 'fc-ev-texto' }, h('span', { class: 'so-leitor', text: info.rotulo + ': ' }), texto),
        h('time', { class: 'fc-ev-quando', datetime: ev.hora, title: dataCompleta(ev.hora), text: quandoFoi(ev.hora) || 'Sem data' })));
  }

  function pintarHistorico(d) {
    const f = ficha;
    const eventos = Array.isArray(d.historico) ? d.historico : [];
    const visiveis = eventos.slice(0, f.limiteHistorico);
    const faltam = eventos.length - visiveis.length;
    const marca = JSON.stringify([visiveis.map((ev) => [ev.n, ev.hora, ev.evento, ev.texto, quandoFoi(ev.hora)]), faltam, !!d.historico_tem_mais]);
    mudou(f, 'historico', marca, () => {
      const corpo = f.pecas.historico.corpo;
      // Quem clicou em "Mostrar mais" continua com o foco no mesmo lugar depois da troca.
      const tinhaFoco = corpo.contains(document.activeElement);
      const lista = h('ol', { class: 'fc-linha-tempo', tabindex: '-1' }, visiveis.map(itemHistorico));
      const mais = faltam ? botao('Mostrar mais ' + Math.min(PASSO_HISTORICO, faltam), {
        classe: 'btn-suave',
        aoClicar: () => { f.limiteHistorico += PASSO_HISTORICO; if (ficha === f) pintarHistorico(f.dados); },
      }) : null;
      const soPrimeiro = eventos.length <= 1;
      const registros = eventos.filter((ev) => ev.evento !== 'primeiro_contato').length;
      trocar(corpo,
        eventos.length ? lista : h('p', { class: 'fc-falta', text: 'Nada por aqui ainda.' }),
        soPrimeiro ? h('p', { class: 'fc-falta', text: 'Transferências, atendimentos, horários e mudanças de etiqueta aparecem aqui.' }) : null,
        faltam ? h('div', { class: 'fc-mais' }, h('span', { text: 'Mostrando ' + visiveis.length + ' de ' + eventos.length + ' registros.' }), mais) : null,
        !faltam && d.historico_tem_mais ? h('p', { class: 'fc-falta', text: 'Aqui aparecem só os ' + registros + ' registros mais novos e o primeiro contato.' }) : null);
      if (tinhaFoco) (mais || lista).focus({ preventScroll: true });
    });
  }

  // Fechar ou recarregar a página com anotação por salvar pede confirmação.
  window.addEventListener('beforeunload', (ev) => {
    if (!S.contatos || !S.contatos.rascunhos || !Object.keys(S.contatos.rascunhos).length) return;
    ev.preventDefault();
    ev.returnValue = '';
  });

  TELAS.contatos = { montar, atualizar, sair };
})();
