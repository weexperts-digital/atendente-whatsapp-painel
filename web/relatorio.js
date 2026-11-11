'use strict';
// Aba Relatório: os números do período, a foto de agora e o movimento de cada dia.
// Só leitura. Busca de novo a cada 30 segundos enquanto a tela está aberta.

(function () {
  let tela = null;

  const INTERVALO = 30000;
  const PERIODOS = [
    { dias: 1, rotulo: 'Hoje', titulo: 'Hoje' },
    { dias: 7, rotulo: '7 dias', titulo: 'Últimos 7 dias' },
    { dias: 30, rotulo: '30 dias', titulo: 'Últimos 30 dias' },
    { dias: 0, rotulo: 'Tudo', titulo: 'Desde o começo' },
  ];
  const TEMPERATURAS_AGORA = [
    { chave: 'quentes', rotulo: 'Quentes', temp: 'quente' },
    { chave: 'mornos', rotulo: 'Mornos', temp: 'morno' },
    { chave: 'frios', rotulo: 'Frios', temp: 'frio' },
    { chave: 'indefinidos', rotulo: 'Sem classificação', temp: 'indefinido' },
  ];
  const IMPRESSORA = [
    'M6 18H4a2 2 0 0 1-2-2v-5a2 2 0 0 1 2-2h16a2 2 0 0 1 2 2v5a2 2 0 0 1-2 2h-2',
    'M6 9V3a1 1 0 0 1 1-1h10a1 1 0 0 1 1 1v6',
    'M7 14h10a1 1 0 0 1 1 1v6a1 1 0 0 1-1 1H7a1 1 0 0 1-1-1v-6a1 1 0 0 1 1-1z',
  ];
  const GRAFICO = ['M3 3v16a2 2 0 0 0 2 2h16', 'M18 17V9', 'M13 17V5', 'M8 17v-3'];

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

  const formato = new Intl.NumberFormat('pt-BR');
  function inteiro(n) { return Math.max(0, Math.round(Number(n) || 0)); }
  function fmt(n) { return formato.format(inteiro(n)); }
  function plural(n, um, varios) { return fmt(n) + ' ' + (inteiro(n) === 1 ? um : varios); }

  function memoria() {
    if (!S.relatorio) S.relatorio = { dias: 7, cache: {}, tabelaAberta: false };
    return S.relatorio;
  }

  function periodoAtual() {
    const dias = memoria().dias;
    return PERIODOS.find((p) => p.dias === dias) || PERIODOS[1];
  }

  // tetoBonito arredonda o maior valor para um número redondo, com metade inteira.
  function tetoBonito(v) {
    if (v <= 10) return [2, 4, 6, 8, 10].find((x) => v <= x);
    const base = Math.pow(10, Math.floor(Math.log10(v)));
    for (const m of [1, 2, 3, 4, 5, 6, 8, 10]) {
      if (v <= m * base) return m * base;
    }
    return 10 * base;
  }

  // ---------- Tela ----------

  function montar(main) {
    if (tela) sair();
    const m = memoria();
    const raiz = h('div', { class: 'tela-relatorio' });
    const corpo = h('div', { class: 'rl-corpo' });
    const quando = h('p', { class: 'rl-quando' });
    const pilulas = h('div', { class: 'rl-periodo', role: 'group', 'aria-label': 'Período do relatório' });
    const impressao = h('div', { class: 'rl-impressao' });
    const minha = tela = { main, raiz, corpo, quando, pilulas, impressao, pedido: 0, ultima: 0, pintado: '', erro: null, relogio: 0, observador: null, tabela: null };

    por(raiz, [
      h('div', { class: 'topo-tela' },
        h('div', { class: 'topo-textos' },
          h('h1', { text: 'Relatório' }),
          h('p', { class: 'descricao', text: 'Um resumo simples do atendimento. Os números se atualizam sozinhos a cada 30 segundos.' })),
        h('div', { class: 'rl-acoes' },
          h('button', { type: 'button', class: 'btn btn-suave', onclick: () => window.print() }, desenho(IMPRESSORA), h('span', { text: 'Imprimir' })))),
      h('div', { class: 'rl-filtros' }, pilulas, quando),
      impressao,
      corpo,
    ]);
    trocar(main, raiz);

    trocar(pilulas, PERIODOS.map((p) => h('button', {
      type: 'button', class: 'rl-pilula', 'data-dias': String(p.dias), 'aria-pressed': 'false', text: p.rotulo,
      onclick: () => {
        if (!tela || m.dias === p.dias) return;
        m.dias = p.dias;
        tela.erro = null;
        pintarPilulas();
        pintarCorpo();
        buscar();
      },
    })));

    window.addEventListener('beforeprint', antesDeImprimir);
    window.addEventListener('afterprint', depoisDeImprimir);
    // O relógio próprio garante a atualização mesmo se a casca não chamar atualizar().
    minha.relogio = setInterval(() => {
      if (tela !== minha) { clearInterval(minha.relogio); return; }
      if (!raiz.isConnected) { sair(); return; } // a casca trocou de tela sem avisar
      atualizar();
    }, 5000);

    pintarPilulas();
    pintarCorpo();
    buscar();
    // O nome do negócio entra só no cabeçalho da impressão.
    if (!S.config && typeof carregarConfig === 'function') carregarConfig().catch(() => {});
  }

  function sair() {
    if (!tela) return;
    guardarTabela();
    clearInterval(tela.relogio);
    soltarGrafico();
    window.removeEventListener('beforeprint', antesDeImprimir);
    window.removeEventListener('afterprint', depoisDeImprimir);
    tela = null;
  }

  // A casca chama a cada 3 segundos. Só busca se a última busca já passou de 30 segundos.
  function atualizar() {
    if (!tela || document.hidden) return;
    if (Date.now() - tela.ultima >= INTERVALO) buscar();
  }

  async function buscar() {
    if (!tela) return;
    const minha = tela;
    const m = memoria();
    const dias = m.dias;
    const pedido = ++minha.pedido;
    minha.ultima = Date.now();
    let dados = null;
    let erro = null;
    try {
      dados = await api.get('/api/relatorio?dias=' + dias);
    } catch (e) {
      erro = e;
    }
    if (dados) m.cache[dias] = { dados, quando: Date.now() };
    if (tela !== minha || minha.pedido !== pedido || m.dias !== dias) return;
    minha.erro = erro;
    pintarCorpo();
  }

  function pintarPilulas() {
    const dias = String(memoria().dias);
    for (const b of tela.pilulas.children) b.setAttribute('aria-pressed', b.dataset.dias === dias ? 'true' : 'false');
  }

  function guardarTabela() {
    if (tela && tela.tabela && !tela.imprimindo) memoria().tabelaAberta = tela.tabela.open;
  }

  function soltarGrafico() {
    if (!tela) return;
    if (tela.observador) { tela.observador.disconnect(); tela.observador = null; }
    tela.ajustarGrafico = null;
  }

  function pintarCorpo() {
    if (!tela) return;
    const m = memoria();
    const guardado = m.cache[m.dias];
    const periodo = periodoAtual();

    if (!guardado) {
      tela.quando.textContent = '';
      tela.pintado = '';
      guardarTabela();
      soltarGrafico();
      tela.tabela = null;
      trocar(tela.corpo, tela.erro
        ? falhaCarga(tela.erro, () => { tela.erro = null; pintarCorpo(); return buscar(); })
        : carregandoEl('Montando o relatório...'));
      return;
    }

    tela.quando.textContent = tela.erro
      ? 'Não consegui atualizar agora. Estes números são das ' + horaCurta(new Date(guardado.quando).toISOString()) + '.'
      : 'Atualizado às ' + horaCurta(new Date(guardado.quando).toISOString());
    tela.quando.classList.toggle('rl-quando-erro', !!tela.erro);

    const marca = m.dias + '|' + JSON.stringify(guardado.dados);
    if (marca === tela.pintado) return; // nada mudou: não redesenha
    guardarTabela();
    soltarGrafico();
    tela.tabela = null;
    tela.pintado = marca;
    trocar(tela.corpo, montarCorpo(guardado.dados, periodo));
    pintarImpressao();
  }

  function pintarImpressao(agora) {
    if (!tela) return;
    const negocio = S.config && S.config.negocio ? S.config.negocio : '';
    const d = agora || new Date();
    trocar(tela.impressao,
      h('strong', { text: 'Relatório do atendimento' + (negocio ? ' de ' + negocio : '') }),
      h('span', { text: 'Período: ' + periodoAtual().titulo }),
      h('span', { text: 'Impresso em ' + doisDigitos(d.getDate()) + '/' + doisDigitos(d.getMonth() + 1) + '/' + d.getFullYear() + ' às ' + doisDigitos(d.getHours()) + ':' + doisDigitos(d.getMinutes()) }));
  }

  // Na impressão a tabela de cada dia sai aberta.
  function antesDeImprimir() {
    if (!tela) return;
    pintarImpressao(new Date());
    tela.imprimindo = true;
    if (tela.ajustarGrafico) tela.ajustarGrafico(640); // largura útil de uma folha A4 em pé
    if (tela.tabela) {
      tela.tabelaEra = tela.tabela.open;
      tela.tabela.open = true;
    }
  }

  function depoisDeImprimir() {
    if (!tela || !tela.imprimindo) return;
    if (tela.tabela) tela.tabela.open = !!tela.tabelaEra;
    if (tela.ajustarGrafico) tela.ajustarGrafico();
    tela.imprimindo = false;
  }

  // ---------- Conteúdo ----------

  function montarCorpo(d, periodo) {
    const p = d.periodo || {};
    const a = d.agora || {};
    const dias = Array.isArray(d.por_dia) ? d.por_dia : [];
    const teveAlgo = inteiro(a.total) > 0 ||
      Object.keys(p).some((k) => inteiro(p[k]) > 0) ||
      dias.some((x) => inteiro(x.mensagens) + inteiro(x.novas) + inteiro(x.transbordos) + inteiro(x.agendamentos) > 0);

    if (!teveAlgo) {
      return h('div', { class: 'cartao' }, h('div', { class: 'vazio' },
        desenho(GRAFICO, 'ic-grande'),
        h('h2', { text: 'Ainda não há números para mostrar' }),
        h('p', { text: 'Assim que os clientes escreverem para o WhatsApp do negócio, o relatório se preenche sozinho.' }),
        h('p', { text: 'Enquanto isso, você pode testar o assistente na aba Treinar.' }),
        h('div', { class: 'vazio-acoes' },
          h('a', { class: 'btn btn-primario', href: '#treinar' }, icone('treinar'), h('span', { text: 'Abrir a aba Treinar' })),
          h('a', { class: 'btn btn-suave', href: '#conversas' }, icone('conversas'), h('span', { text: 'Ver as conversas' })))));
    }

    return h('div', { class: 'rl-grade' },
      blocoNumeros(p, periodo),
      blocoAgora(a),
      blocoPorDia(dias),
      blocoEtiquetas(Array.isArray(d.por_etiqueta) ? d.por_etiqueta : []),
      blocoEquipe(Array.isArray(d.atendentes) ? d.atendentes : [], periodo),
      blocoPlanilhas());
  }

  function notaVazia(texto) {
    return h('p', { class: 'rl-vazio', text: texto });
  }

  function blocoNumeros(p, periodo) {
    const itens = [
      { rotulo: 'Conversas', valor: p.conversas, nota: plural(p.mensagens_clientes, 'mensagem de cliente', 'mensagens de clientes') },
      { rotulo: 'Conversas novas', valor: p.novas, nota: 'Clientes que escreveram pela primeira vez' },
      { rotulo: 'Transferências para a equipe', valor: p.transbordos, nota: 'Vezes em que o assistente chamou uma pessoa' },
      { rotulo: 'Agendamentos', valor: p.agendamentos, nota: inteiro(p.cancelamentos) ? plural(p.cancelamentos, 'cancelamento', 'cancelamentos') : 'Nenhum cancelamento' },
      { rotulo: 'Respostas do assistente', valor: p.respostas_assistente, nota: 'Enviadas pela IA', marca: 'ia' },
      { rotulo: 'Respostas da equipe', valor: p.respostas_equipe, nota: 'Enviadas por pessoas', marca: 'equipe' },
    ];
    const parado = !itens.some((i) => inteiro(i.valor) > 0);
    return h('section', { class: 'rl-area-numeros', 'aria-labelledby': 'rl-titulo-periodo' },
      h('h2', { id: 'rl-titulo-periodo', text: periodo.titulo }),
      parado ? notaVazia('Nada aconteceu neste período. Escolha um período maior para ver mais.') : null,
      h('ul', { class: 'rl-numeros' }, itens.map((i) => h('li', { class: 'rl-numero' + (inteiro(i.valor) ? '' : ' rl-numero-zero') },
        h('span', { class: 'rl-numero-rotulo' }, i.marca ? h('span', { class: 'rl-marca rl-marca-' + i.marca, 'aria-hidden': 'true' }) : null, i.rotulo),
        h('strong', { class: 'rl-numero-valor', text: fmt(i.valor) }),
        h('span', { class: 'rl-numero-nota', text: i.nota })))));
  }

  function blocoAgora(a) {
    const total = inteiro(a.total);
    const partes = TEMPERATURAS_AGORA.map((t) => Object.assign({ valor: inteiro(a[t.chave]) }, t));
    const soma = partes.reduce((s, x) => s + x.valor, 0);
    const base = Math.max(total, soma);
    const pct = (v) => (base ? Math.round((v / base) * 100) : 0);

    let barra = null;
    if (soma > 0) {
      barra = h('div', {
        class: 'rl-partes', role: 'img',
        'aria-label': 'Conversas por temperatura: ' + partes.map((x) => fmt(x.valor) + ' ' + x.rotulo.toLowerCase()).join(', ') + '.',
      }, partes.filter((x) => x.valor > 0).map((x) => {
        const seg = h('span', { class: 'rl-parte', 'data-temp': x.temp, title: x.rotulo + ': ' + fmt(x.valor) + ' (' + pct(x.valor) + '%)' },
          h('span', { text: fmt(x.valor) }));
        seg.style.flexGrow = String(x.valor);
        return seg;
      }));
    }

    return h('section', { class: 'cartao rl-bloco rl-area-agora', 'aria-labelledby': 'rl-titulo-agora' },
      h('h2', { id: 'rl-titulo-agora', text: 'Agora' }),
      h('p', { class: 'descricao', text: 'Como estão todas as conversas neste momento.' }),
      h('p', { class: 'rl-total' }, h('strong', { text: fmt(total) }), ' ', total === 1 ? 'conversa no total' : 'conversas no total'),
      barra || notaVazia('Nenhuma conversa por aqui ainda.'),
      barra ? h('ul', { class: 'rl-legenda' }, partes.map((x) => h('li', { 'data-temp': x.temp },
        h('span', { class: 'rl-amostra', 'aria-hidden': 'true' }),
        h('span', { class: 'rl-legenda-nome', text: x.rotulo }),
        h('strong', { text: fmt(x.valor) }),
        h('span', { class: 'rl-legenda-pct', text: pct(x.valor) + '%' })))) : null,
      h('ul', { class: 'rl-situacoes' },
        h('li', null, h('strong', { text: fmt(a.com_assistente) }), h('span', { text: 'Com o assistente' })),
        h('li', null, h('strong', { text: fmt(a.aguardando) }), h('span', { text: 'Na fila, esperando uma pessoa' })),
        h('li', null, h('strong', { text: fmt(a.em_atendimento) }), h('span', { text: 'Em atendimento pela equipe' }))));
  }

  function blocoPorDia(lista) {
    const dias = lista.map((d) => {
      const rotulo = String(d.rotulo || d.data || '').trim();
      const corte = rotulo.indexOf(' ');
      return {
        rotulo,
        semana: corte > 0 ? rotulo.slice(0, corte) : '',
        data: corte > 0 ? rotulo.slice(corte + 1) : rotulo,
        mensagens: inteiro(d.mensagens), novas: inteiro(d.novas),
        transbordos: inteiro(d.transbordos), agendamentos: inteiro(d.agendamentos),
      };
    });
    const maior = dias.reduce((m, d) => Math.max(m, d.mensagens), 0);
    const total = dias.reduce((m, d) => m + d.mensagens, 0);
    const temDados = dias.some((d) => d.mensagens + d.novas + d.transbordos + d.agendamentos > 0);
    const secao = h('section', { class: 'cartao rl-bloco rl-area-dia', 'aria-labelledby': 'rl-titulo-dia' },
      h('h2', { id: 'rl-titulo-dia', text: 'Por dia' }),
      h('p', { class: 'descricao', text: dias.length > 1 ? 'Mensagens de cada dia, nos últimos ' + dias.length + ' dias.' : 'Mensagens de cada dia.' }));
    if (!temDados) {
      secao.appendChild(notaVazia('Nenhuma mensagem nestes dias. Quando os clientes escreverem, as barras aparecem aqui.'));
      return secao;
    }

    const n = dias.length;
    const teto = tetoBonito(Math.max(1, maior));
    const dica = h('div', { class: 'rl-dica', hidden: true });
    const plot = h('div', { class: 'rl-plot' });
    const eixo = h('div', { class: 'rl-eixo-x' });
    const novas = h('div', { class: 'rl-nums rl-nums-novas' });
    const transf = h('div', { class: 'rl-nums rl-nums-transf' });
    const rotulos = [];
    const diaMaior = dias.find((d) => d.mensagens === maior);

    dias.forEach((d) => {
      const barra = h('span', { class: 'rl-barra' });
      barra.style.setProperty('--p', String(d.mensagens / teto));
      plot.appendChild(h('div', { class: 'rl-col' + (d.mensagens ? '' : ' rl-col-zero') + (d === diaMaior ? ' rl-col-maior' : '') },
        h('span', { class: 'rl-valor', text: fmt(d.mensagens) }), barra));
      const rot = h('span', { class: 'rl-dia' },
        d.semana ? h('span', { class: 'rl-dia-semana', text: d.semana }) : null,
        h('span', { text: d.data }));
      rotulos.push(rot);
      eixo.appendChild(rot);
      novas.appendChild(h('span', { class: d.novas ? null : 'rl-zero', text: fmt(d.novas) }));
      transf.appendChild(h('span', { class: d.transbordos ? null : 'rl-zero', text: fmt(d.transbordos) }));
    });

    const grafico = h('div', {
      class: 'rl-grafico', role: 'img',
      'aria-label': 'Gráfico de barras com as mensagens de cada dia. Total de ' + plural(total, 'mensagem', 'mensagens') + ' em ' + n + ' dias.' +
        (diaMaior && maior ? ' O dia com mais mensagens foi ' + diaMaior.rotulo + ', com ' + fmt(maior) + '.' : '') + ' A tabela logo abaixo traz todos os números.',
    },
      h('div', { class: 'rl-eixo-y' }, h('span', { text: fmt(teto) }), h('span', { text: fmt(teto / 2) }), h('span', { text: '0' })),
      plot, eixo,
      h('span', { class: 'rl-cab rl-cab-novas', text: 'Novas' }), novas,
      h('span', { class: 'rl-cab rl-cab-transf', text: 'Transferências' }), transf,
      dica);
    grafico.style.setProperty('--n', String(n));

    // A dica acompanha o dia sob o ponteiro (ou sob o dedo).
    function mostrarDica(col) {
      const i = Array.prototype.indexOf.call(plot.children, col);
      const d = dias[i];
      if (!d) return;
      for (const c of plot.children) c.classList.toggle('rl-ativa', c === col);
      trocar(dica,
        h('strong', { text: d.rotulo }),
        h('span', { text: plural(d.mensagens, 'mensagem', 'mensagens') }),
        h('span', { text: plural(d.novas, 'conversa nova', 'conversas novas') }),
        h('span', { text: plural(d.transbordos, 'transferência', 'transferências') }),
        h('span', { text: plural(d.agendamentos, 'agendamento', 'agendamentos') }));
      dica.hidden = false;
      const caixa = grafico.getBoundingClientRect();
      const alvo = col.getBoundingClientRect();
      // Fica acima do número da barra (ou da barra, quando o número está escondido).
      const numero = col.querySelector('.rl-valor');
      const barra = (numero && numero.offsetParent ? numero : col.querySelector('.rl-barra')).getBoundingClientRect();
      const centro = alvo.left + alvo.width / 2 - caixa.left;
      const esquerda = Math.max(0, Math.min(caixa.width - dica.offsetWidth, centro - dica.offsetWidth / 2));
      dica.style.left = Math.round(esquerda) + 'px';
      dica.style.top = Math.round(barra.top - caixa.top - dica.offsetHeight - 10) + 'px';
    }
    function esconderDica() {
      dica.hidden = true;
      for (const c of plot.children) c.classList.remove('rl-ativa');
    }
    plot.addEventListener('pointermove', (ev) => {
      const col = ev.target.closest ? ev.target.closest('.rl-col') : null;
      if (col) mostrarDica(col);
    });
    plot.addEventListener('pointerdown', (ev) => {
      const col = ev.target.closest ? ev.target.closest('.rl-col') : null;
      if (col) mostrarDica(col);
    });
    plot.addEventListener('pointerleave', esconderDica);

    // Com pouco espaço, somem os números sobre as barras, depois as linhas, e os rótulos se espaçam.
    // Na impressão a largura vem forçada: o papel é mais estreito que a tela.
    function ajustar(forcada) {
      const largura = typeof forcada === 'number' ? forcada : grafico.clientWidth;
      if (!largura) return;
      const linhas = (largura - 110) / n >= 17;
      const coluna = (largura - (linhas ? 110 : 36)) / n;
      grafico.dataset.linhas = linhas ? 'sim' : 'nao';
      grafico.dataset.valores = coluna >= 24 ? 'sim' : 'nao';
      const duas = coluna >= 46;
      grafico.dataset.rotulo = duas ? 'duas' : 'uma';
      const passo = duas ? 1 : Math.max(1, Math.ceil(42 / coluna));
      rotulos.forEach((el, i) => el.classList.toggle('rl-oculto', (n - 1 - i) % passo !== 0));
    }
    if (typeof ResizeObserver === 'function') {
      tela.observador = new ResizeObserver(() => ajustar());
      tela.observador.observe(grafico);
    }
    tela.ajustarGrafico = ajustar;
    requestAnimationFrame(() => ajustar());

    const linhaTotal = dias.reduce((t, d) => ({
      mensagens: t.mensagens + d.mensagens, novas: t.novas + d.novas,
      transbordos: t.transbordos + d.transbordos, agendamentos: t.agendamentos + d.agendamentos,
    }), { mensagens: 0, novas: 0, transbordos: 0, agendamentos: 0 });
    const celulas = (x) => [x.mensagens, x.novas, x.transbordos, x.agendamentos].map((v) => h('td', { class: v ? null : 'rl-zero', text: fmt(v) }));
    const tabela = h('details', { class: 'rl-tabela', open: memoria().tabelaAberta },
      h('summary', { class: 'resumo-linha', text: 'Ver os números de cada dia em tabela' }),
      h('div', { class: 'rl-tabela-rolagem' }, h('table', null,
        h('caption', { class: 'so-leitor', text: 'Mensagens, conversas novas, transferências e agendamentos de cada dia' }),
        // O hífen invisível (\u00AD) só aparece se a palavra precisar quebrar, em tela estreita.
        h('thead', null, h('tr', null, ['Dia', 'Mensagens', 'Novas', 'Transfe\u00ADrências', 'Agenda\u00ADmentos'].map((t) => h('th', { scope: 'col', text: t })))),
        h('tbody', null, dias.map((d) => h('tr', null, h('th', { scope: 'row', text: d.rotulo }), celulas(d)))),
        h('tfoot', null, h('tr', null, h('th', { scope: 'row', text: 'Total' }), celulas(linhaTotal))))));
    tela.tabela = tabela;

    por(secao, [
      grafico,
      h('p', { class: 'rl-nota-linhas ajuda', text: 'Os números de conversas novas e de transferências de cada dia estão na tabela abaixo.' }),
      tabela,
    ]);
    return secao;
  }

  // listaBarras monta barras horizontais: nome, número e uma barra fina embaixo.
  function listaBarras(itens, unidade) {
    const maior = itens.reduce((m, i) => Math.max(m, i.valor), 0) || 1;
    return h('ul', { class: 'rl-hbarras' }, itens.map((i) => {
      const cheio = h('span');
      cheio.style.setProperty('--p', String(i.valor / maior));
      return h('li', { class: 'rl-hbarra' },
        h('span', { class: 'rl-hbarra-nome', text: i.nome }),
        h('span', { class: 'rl-hbarra-num' }, h('strong', { text: fmt(i.valor) }), unidade ? ' ' + (i.valor === 1 ? unidade[0] : unidade[1]) : null),
        h('span', { class: 'rl-hbarra-trilho', 'aria-hidden': 'true' }, cheio));
    }));
  }

  function blocoEtiquetas(lista) {
    const todas = lista.map((e) => ({ nome: String(e.nome || '').trim(), valor: inteiro(e.total) }))
      .filter((e) => e.nome && e.valor > 0)
      .sort((a, b) => b.valor - a.valor);
    const itens = todas.slice(0, 8);
    return h('section', { class: 'cartao rl-bloco rl-area-etiquetas', 'aria-labelledby': 'rl-titulo-etiquetas' },
      h('h2', { id: 'rl-titulo-etiquetas', text: 'Etiquetas mais usadas' }),
      h('p', { class: 'descricao', text: 'Quantas conversas têm cada etiqueta.' }),
      itens.length
        ? [listaBarras(itens, ['conversa', 'conversas']),
          todas.length > itens.length ? h('p', { class: 'ajuda', text: 'E mais ' + plural(todas.length - itens.length, 'etiqueta', 'etiquetas') + ' com menos uso.' }) : null]
        : notaVazia('Nenhuma etiqueta ainda. Abra uma conversa e marque com palavras como "orçamento" ou "cliente antigo". Elas aparecem aqui.'));
  }

  function blocoEquipe(lista, periodo) {
    const itens = lista.map((a) => ({ nome: String(a.nome || '').trim() || 'Sem nome', valor: inteiro(a.respostas) }))
      .filter((a) => a.valor > 0)
      .sort((a, b) => b.valor - a.valor)
      .slice(0, 10);
    return h('section', { class: 'cartao rl-bloco rl-area-equipe', 'aria-labelledby': 'rl-titulo-equipe' },
      h('h2', { id: 'rl-titulo-equipe', text: 'Equipe' }),
      h('p', { class: 'descricao', text: 'Respostas de cada atendente. Período: ' + periodo.titulo.toLowerCase() + '.' }),
      itens.length
        ? listaBarras(itens, ['resposta', 'respostas'])
        : notaVazia('Ninguém da equipe respondeu neste período. Quando alguém assumir uma conversa e responder, o nome aparece aqui.'));
  }

  function blocoPlanilhas() {
    return h('section', { class: 'cartao rl-bloco rl-area-planilhas', 'aria-labelledby': 'rl-titulo-planilhas' },
      h('h2', { id: 'rl-titulo-planilhas', text: 'Planilhas' }),
      h('p', { class: 'descricao', text: 'Baixe os dados para abrir no Excel ou no Google Planilhas.' }),
      h('div', { class: 'rl-baixar' },
        h('a', { class: 'btn btn-suave', href: '/leads.csv', download: 'leads.csv' }, icone('baixar'), h('span', { text: 'Baixar planilha de leads' })),
        h('a', { class: 'btn btn-suave', href: '/agenda.csv', download: 'agenda.csv' }, icone('baixar'), h('span', { text: 'Baixar planilha da agenda' }))));
  }

  TELAS.relatorio = { montar, atualizar, sair };
})();
