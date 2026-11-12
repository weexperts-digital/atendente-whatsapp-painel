'use strict';
// Aba Agenda: calendário dos horários (semana, dia, mês e lista), marcar e cancelar pelo
// painel, e as regras da agenda. O calendário é o FullCalendar (web/vendor/fullcalendar.js).
// Nada que vem da API entra como HTML: o conteúdo dos eventos é montado com nós do DOM.

(function () {
  let tela = null;
  const ORDEM_DIAS = [1, 2, 3, 4, 5, 6, 0]; // nas regras, a semana começa na segunda
  const INTERVALOS = [10, 15, 20, 30, 45, 60, 90, 120];
  const VISOES = [
    { id: 'timeGridWeek', rotulo: 'Semana', anterior: 'Semana anterior', proximo: 'Próxima semana' },
    { id: 'timeGridDay', rotulo: 'Dia', anterior: 'Dia anterior', proximo: 'Próximo dia' },
    { id: 'dayGridMonth', rotulo: 'Mês', anterior: 'Mês anterior', proximo: 'Próximo mês' },
    { id: 'lista', rotulo: 'Lista', anterior: '30 dias antes', proximo: '30 dias depois' },
  ];
  const FORMATO_HORA = { hour: '2-digit', minute: '2-digit', hour12: false };

  const telaEstreita = () => window.matchMedia('(max-width: 699px)').matches;

  // ---------- Contas com datas e horas ----------

  function minutosDe(texto) {
    const m = /^(\d{1,2}):(\d{2})/.exec(texto || '');
    return m ? Number(m[1]) * 60 + Number(m[2]) : null;
  }

  function horaDe(min) {
    return doisDigitos(Math.floor(min / 60)) + ':' + doisDigitos(min % 60);
  }

  function dataISO(d) {
    return d.getFullYear() + '-' + doisDigitos(d.getMonth() + 1) + '-' + doisDigitos(d.getDate());
  }

  function fimDe(a) {
    return new Date(new Date(a.inicio).getTime() + (Number(a.duracao_min) || 0) * 60000);
  }

  function maiuscula(texto) {
    return texto ? texto.charAt(0).toUpperCase() + texto.slice(1) : '';
  }

  // diaCompleto: "Hoje, 11/11", "Amanhã, 12/11" ou "qui, 13/11".
  function diaCompleto(iso) {
    const d = new Date(iso);
    const data = doisDigitos(d.getDate()) + '/' + doisDigitos(d.getMonth() + 1);
    const rotulo = diaRotulo(iso);
    return rotulo.includes('/') ? rotulo : rotulo + ', ' + data;
  }

  function agendamentos() {
    return (S.agenda && S.agenda.agendamentos) || [];
  }

  // ---------- Regras ----------

  function normalizarRegras(a) {
    a = a || {};
    const dias = [];
    for (let d = 0; d < 7; d++) {
      const atual = (a.dias || []).find((x) => Number(x.dia) === d) || {};
      dias.push({
        dia: d, aberto: !!atual.aberto,
        inicio: atual.inicio || '09:00', fim: atual.fim || '18:00',
        pausa_inicio: atual.pausa_inicio || '', pausa_fim: atual.pausa_fim || '',
      });
    }
    return {
      ativa: !!a.ativa,
      servicos: (a.servicos || []).map((s) => ({ nome: s.nome || '', duracao_min: Number(s.duracao_min) || 30, preco: s.preco || '' })),
      dias,
      intervalo_min: Number(a.intervalo_min) || 30,
      vagas: Number(a.vagas) || 1,
      antecedencia_min: a.antecedencia_min == null ? 60 : Number(a.antecedencia_min) || 0,
      dias_a_frente: Number(a.dias_a_frente) || 7,
      avisar_humano: a.avisar_humano !== false,
    };
  }

  function regrasSalvas() {
    return normalizarRegras(S.config && S.config.agenda);
  }

  // divisao: de quanto em quanto tempo o calendário desenha uma linha.
  function divisao(regras) {
    const n = regras.intervalo_min;
    if (n <= 60) return Math.max(5, n);
    if (n % 60 === 0) return 60;
    if (n % 30 === 0) return 30;
    return 15;
  }

  // expediente: o horário de atendimento no formato do calendário. A pausa parte o dia em dois.
  function expediente(regras) {
    const lista = [];
    for (const d of regras.dias) {
      if (!d.aberto) continue;
      const ini = minutosDe(d.inicio);
      const fim = minutosDe(d.fim);
      if (ini == null || fim == null || fim <= ini) continue;
      const pi = minutosDe(d.pausa_inicio);
      const pf = minutosDe(d.pausa_fim);
      if (pi != null && pf != null && pf > pi && pi >= ini && pf <= fim) {
        if (pi > ini) lista.push({ daysOfWeek: [d.dia], startTime: horaDe(ini), endTime: horaDe(pi) });
        if (fim > pf) lista.push({ daysOfWeek: [d.dia], startTime: horaDe(pf), endTime: horaDe(fim) });
      } else {
        lista.push({ daysOfWeek: [d.dia], startTime: horaDe(ini), endTime: horaDe(fim) });
      }
    }
    // Nenhum dia aberto: tudo aparece como fora do expediente.
    if (!lista.length) lista.push({ daysOfWeek: [], startTime: '00:00', endTime: '00:00' });
    return lista;
  }

  // faixaDeHoras: o trecho do dia que o calendário mostra. Vai de uma hora antes de abrir até
  // uma hora depois de fechar, e cresce para caber qualquer horário marcado fora disso.
  function faixaDeHoras(regras) {
    const passo = divisao(regras);
    let abre = 24 * 60;
    let fecha = 0;
    for (const d of regras.dias) {
      if (!d.aberto) continue;
      const ini = minutosDe(d.inicio);
      const fim = minutosDe(d.fim);
      if (ini == null || fim == null || fim <= ini) continue;
      abre = Math.min(abre, ini);
      fecha = Math.max(fecha, fim);
    }
    if (fecha <= abre) { abre = 8 * 60; fecha = 18 * 60; }
    let primeiro = abre;
    let ultimo = fecha;
    for (const a of agendamentos()) {
      if (a.status === 'cancelado' && !S.agendaCancelados) continue;
      const inicio = new Date(a.inicio);
      if (isNaN(inicio)) continue;
      const m = inicio.getHours() * 60 + inicio.getMinutes();
      primeiro = Math.min(primeiro, m);
      ultimo = Math.max(ultimo, Math.min(24 * 60, m + (Number(a.duracao_min) || passo)));
    }
    // As linhas ficam alinhadas com a hora de abrir.
    let min = abre - Math.ceil(60 / passo) * passo;
    while (min > primeiro) min -= passo;
    while (min < 0) min += passo;
    let max = min + Math.ceil((Math.max(ultimo, fecha) + 60 - min) / passo) * passo;
    if (max > 24 * 60) max = 24 * 60;
    return { min, max, abre };
  }

  // rolarPara: a hora em que a grade abre na tela. No começo do dia, um pouco antes de abrir.
  // Com o dia adiantado, perto da hora de agora.
  function rolarPara(faixa) {
    const agora = new Date();
    const m = agora.getHours() * 60 + agora.getMinutes();
    const cedo = Math.max(0, faixa.abre - 30);
    if (m < faixa.min || m > faixa.max) return cedo;
    return Math.max(cedo, Math.floor((m - 150) / 60) * 60);
  }

  function duracaoISO(min) {
    return doisDigitos(Math.floor(min / 60)) + ':' + doisDigitos(min % 60) + ':00';
  }

  // opcoesDasRegras: o que as regras mudam no calendário.
  function opcoesDasRegras() {
    const regras = regrasSalvas();
    const passo = divisao(regras);
    const faixa = faixaDeHoras(regras);
    return {
      businessHours: expediente(regras),
      slotDuration: duracaoISO(passo),
      snapDuration: duracaoISO(passo),
      slotLabelInterval: 60 % passo === 0 ? '01:00:00' : duracaoISO(passo),
      slotMinTime: duracaoISO(faixa.min),
      slotMaxTime: faixa.max >= 24 * 60 ? '24:00:00' : duracaoISO(faixa.max),
      scrollTime: duracaoISO(rolarPara(faixa)),
      // No celular as linhas são mais altas, para o toque acertar o horário.
      alturaLinha: telaEstreita() ? (passo >= 30 ? 44 : (passo >= 15 ? 32 : 26)) : (passo >= 30 ? 26 : (passo >= 15 ? 22 : 20)),
    };
  }

  // A hora de rolar muda com o relógio: fica fora da comparação de "as regras mudaram?".
  function marcaDasRegras(regras) {
    return JSON.stringify(Object.assign({}, regras, { scrollTime: '' }));
  }

  // ---------- Eventos do calendário ----------

  function eventos() {
    const agora = Date.now();
    const passo = regrasSalvas().intervalo_min;
    const lista = [];
    for (const a of agendamentos()) {
      const cancelado = a.status === 'cancelado';
      if (cancelado && !S.agendaCancelados) continue;
      const inicio = new Date(a.inicio);
      if (isNaN(inicio)) continue;
      const fim = new Date(inicio.getTime() + Math.max(5, Number(a.duracao_min) || passo) * 60000);
      const classes = [cancelado ? 'ev-cancelado' : 'ev-marcado'];
      if (!cancelado && fim.getTime() < agora) classes.push('ev-passado');
      lista.push({
        id: String(a.id), title: a.nome || 'Sem nome',
        start: inicio, end: fim, classNames: classes, extendedProps: { ag: a },
      });
    }
    return lista;
  }

  function seloIA() {
    return h('span', { class: 'selo-ia', title: 'Marcado pelo assistente', text: 'IA' });
  }

  function descricaoFalada(a) {
    return [
      horaCurta(a.inicio), a.nome || 'Sem nome', a.servico,
      a.origem === 'assistente' ? 'marcado pelo assistente' : 'marcado no painel',
      a.status === 'cancelado' ? 'cancelado' : '',
    ].filter(Boolean).join(', ');
  }

  // conteudoEvento monta o que aparece dentro de cada evento. Só nós do DOM, nunca HTML em texto.
  function conteudoEvento(arg) {
    const a = arg.event.extendedProps.ag || {};
    const visao = arg.view.type;
    const hora = h('span', { class: 'ev-hora', text: horaCurta(a.inicio) });
    const nome = h('span', { class: 'ev-nome', text: a.nome || 'Sem nome' });
    const ia = a.origem === 'assistente' ? seloIA() : null;
    const cancelado = a.status === 'cancelado' ? h('span', { class: 'etiqueta etiqueta-perigo', text: 'Cancelado' }) : null;
    if (visao === 'lista') {
      return { domNodes: [h('div', { class: 'ev ev-lista' },
        h('div', { class: 'ev-linha' }, nome, ia, cancelado),
        h('div', { class: 'ev-servico', text: [a.servico, a.telefone_bonito].filter(Boolean).join(', ') }))] };
    }
    if (visao === 'dayGridMonth') {
      return { domNodes: [h('div', { class: 'ev ev-mes' }, hora, nome, ia)] };
    }
    return { domNodes: [h('div', { class: 'ev ev-grade' },
      h('div', { class: 'ev-linha' }, hora, ia),
      nome,
      a.servico ? h('span', { class: 'ev-servico', text: a.servico }) : null)] };
  }

  function cabecalhoDia(arg) {
    if (arg.view.type === 'lista') {
      // Na lista, o dia vem por extenso, com a marca de hoje ou amanhã.
      const perto = diaRotulo(arg.date);
      return { domNodes: [h('span', { class: 'lista-dia' },
        perto === 'Hoje' || perto === 'Amanhã' ? h('span', { class: 'etiqueta', text: perto }) : null,
        h('span', { text: maiuscula(arg.text || '') }))] };
    }
    // No mês a coluna é um dia da semana, não uma data: vale arg.dow.
    const semana = h('span', { class: 'cab-semana', text: DIAS_CURTOS[arg.dow != null ? arg.dow : arg.date.getDay()] });
    if (arg.view.type === 'dayGridMonth') return { domNodes: [semana] };
    return { domNodes: [semana, h('span', { class: 'cab-dia' + (arg.isToday ? ' hoje' : ''), text: String(arg.date.getDate()) })] };
  }

  // ---------- Montagem da tela ----------

  function topo(acoes) {
    return h('div', { class: 'topo-tela' },
      h('div', { class: 'topo-textos' },
        h('h1', { text: 'Agenda' }),
        h('p', { class: 'descricao', text: 'O assistente oferece os horários livres e marca pelo WhatsApp. Para marcar por aqui, clique em um espaço vazio do calendário.' })),
      acoes ? h('div', { class: 'acoes' }, acoes) : null);
  }

  async function montar(main) {
    const minha = tela = { main, janelas: new Set() };
    trocar(main, topo(), carregandoEl('Carregando a agenda...'));
    try {
      await Promise.all([carregarConfig(), carregarAgenda()]);
    } catch (e) {
      if (tela === minha) trocar(main, topo(), falhaCarga(e, () => montar(main)));
      return;
    }
    if (tela !== minha) return;
    desenhar();
  }

  function sair() {
    if (!tela) return;
    const antiga = tela;
    tela = null;
    for (const j of Array.from(antiga.janelas)) j.fechar(true);
    if (antiga.cal) {
      try { antiga.cal.destroy(); } catch (e) { console.error(e); }
    }
  }

  function atualizar(batidas) {
    if (!tela || !tela.pronta || tela.buscando) return;
    const mudouLigada = !!(S.estado && S.agenda) && !!S.estado.agenda_ativa !== !!S.agenda.ativa;
    if (batidas % 5 !== 0 && !mudouLigada) return; // a cada 15 segundos
    const minha = tela;
    minha.buscando = true;
    minha.voltas = (minha.voltas || 0) + 1;
    // As regras mudam pouco: são relidas uma vez por minuto, ou quando a agenda liga ou desliga.
    const comRegras = mudouLigada || minha.voltas % 4 === 0;
    Promise.all([carregarAgenda(), comRegras ? carregarConfig(true).catch(() => {}) : null])
      .then(() => { if (tela === minha) refletir(); })
      .catch(() => { /* tenta de novo na próxima volta */ })
      .then(() => { minha.buscando = false; });
  }

  // recarregar busca a agenda de novo depois de uma ação da pessoa.
  async function recarregar() {
    await carregarAgenda();
    refletir();
  }

  function desenhar() {
    const t = tela;
    const novo = botao('Novo agendamento', { classe: 'btn-primario', icone: 'mais', aoClicar: () => abrirMarcar() });

    t.faixa = h('div', { class: 'agenda-aviso', role: 'status', hidden: true },
      icone('info'),
      h('p', null, h('strong', { text: 'A agenda está desligada: o assistente não marca horários.' }),
        h('span', { text: 'Você ainda pode marcar e cancelar por aqui.' })),
      botao('Ligar agora', { classe: 'btn-primario', icone: 'ligar', aoClicar: ligarAgenda }));

    // Barra do calendário: hoje, anterior, próximo, título do período, visões e cancelados.
    t.titulo = h('h2', { class: 'cal-titulo', 'aria-live': 'polite' });
    // Estes botões agem na hora: não passam por ocupar(), para o foco do teclado não sair deles.
    const aoClicar = (b, acao) => { b.addEventListener('click', () => { if (t.cal) acao(t.cal); }); return b; };
    t.anterior = aoClicar(botao('', { classe: 'btn-icone', icone: 'seta-esq', rotulo: 'Período anterior' }), (cal) => cal.prev());
    t.proximo = aoClicar(botao('', { classe: 'btn-icone', icone: 'seta-dir', rotulo: 'Próximo período' }), (cal) => cal.next());
    t.visoes = h('div', { class: 'segmentado cal-visoes', role: 'group', 'aria-label': 'Como ver a agenda' },
      VISOES.map((v) => h('button', {
        type: 'button', 'data-visao': v.id, 'aria-pressed': 'false',
        onclick: () => { if (t.cal) t.cal.changeView(v.id); },
      }, v.rotulo)));
    const cancelados = chave('Mostrar cancelados', S.agendaCancelados, (ligado) => {
      S.agendaCancelados = ligado;
      t.marcaEventos = null;
      refletir();
    });
    const barra = h('div', { class: 'cal-barra' },
      h('div', { class: 'cal-nav' },
        aoClicar(botao('Hoje', { classe: 'btn-suave', dica: 'Voltar para hoje' }), (cal) => cal.today()),
        t.anterior, t.proximo),
      t.titulo,
      t.visoes);

    t.calEl = h('div', { class: 'cal-corpo' });
    t.legenda = h('div', { class: 'cal-legenda' });
    t.pe = h('div', { class: 'cal-pe' }, t.legenda, cancelados);

    // Coluna lateral.
    t.hoje = h('ul', { class: 'lado-lista' });
    t.contaHoje = h('span', { class: 'lado-conta' });
    t.proximos = h('ul', { class: 'lado-lista' });
    t.resumoRegras = h('p', { class: 'ajuda' });
    const lado = h('aside', { class: 'agenda-lado', 'aria-label': 'Resumo da agenda' },
      h('section', { class: 'cartao lado-cartao' },
        h('div', { class: 'lado-topo' }, h('h2', { text: 'Hoje' }), t.contaHoje),
        t.hoje),
      h('section', { class: 'cartao lado-cartao' },
        h('div', { class: 'lado-topo' }, h('h2', { text: 'Próximos' })),
        t.proximos,
        aoClicar(botao('Ver todos em lista', { classe: 'btn-fantasma lado-ver', icone: 'lista' }), (cal) => {
          cal.changeView('lista', new Date());
          t.calEl.scrollIntoView({ block: 'nearest', behavior: semMovimento() ? 'auto' : 'smooth' });
        })),
      h('section', { class: 'cartao lado-cartao lado-acoes' },
        botao('Regras da agenda', { classe: 'btn-suave', icone: 'ajustes', aoClicar: () => abrirRegras() }),
        t.resumoRegras,
        h('a', { class: 'link-baixar', href: '/agenda.csv', download: 'agenda.csv' }, icone('baixar'), 'Baixar planilha da agenda')));

    trocar(t.main, topo(novo), t.faixa,
      h('div', { class: 'agenda-grade' },
        h('section', { class: 'cartao agenda-cal', 'aria-label': 'Calendário' }, barra, t.calEl, t.pe),
        lado));

    criarCalendario();
    t.pronta = true;
    refletir();
  }

  function criarCalendario() {
    const t = tela;
    if (!window.FullCalendar || !window.FullCalendar.Calendar) {
      // A biblioteca não carregou: a tela segue funcionando pelas listas e pelos botões.
      trocar(t.calEl, vazio('agenda', 'O calendário não carregou',
        'Recarregue a página. Enquanto isso, use as listas ao lado e o botão Novo agendamento.'));
      t.titulo.textContent = 'Calendário';
      return;
    }
    const regras = opcoesDasRegras();
    t.marcaRegras = marcaDasRegras(regras);
    t.calEl.style.setProperty('--altura-linha', regras.alturaLinha + 'px');
    const estreita = telaEstreita();
    t.altura = alturaIdeal();
    const visao = VISOES.some((v) => v.id === S.agendaVisao) ? S.agendaVisao : (estreita ? 'timeGridDay' : 'timeGridWeek');

    const cal = new window.FullCalendar.Calendar(t.calEl, {
      locale: 'pt-br',
      timeZone: 'local',
      initialView: visao,
      initialDate: S.agendaData || undefined,
      headerToolbar: false,
      height: t.altura,
      expandRows: true,
      stickyHeaderDates: true,
      firstDay: 0,
      nowIndicator: true,
      allDaySlot: false,
      slotEventOverlap: false,
      eventShortHeight: 46, // evento baixo: hora e nome na mesma linha
      eventDisplay: 'block',
      displayEventEnd: false,
      dayMaxEvents: true,
      fixedWeekCount: false,
      navLinks: true,
      navLinkDayClick: 'timeGridDay',
      eventInteractive: true,
      businessHours: regras.businessHours,
      slotDuration: regras.slotDuration,
      snapDuration: regras.snapDuration,
      slotLabelInterval: regras.slotLabelInterval,
      slotMinTime: regras.slotMinTime,
      slotMaxTime: regras.slotMaxTime,
      scrollTime: regras.scrollTime,
      slotLabelFormat: FORMATO_HORA,
      eventTimeFormat: FORMATO_HORA,
      titleFormat: { year: 'numeric', month: 'long', day: 'numeric' },
      titleRangeSeparator: ' a ', // "8 a 14 de novembro de 2026"
      listDayFormat: { weekday: 'long', day: 'numeric', month: 'long' },
      listDaySideFormat: false,
      noEventsContent: 'Nenhum horário marcado neste período.',
      moreLinkContent: (arg) => '+' + arg.num + ' horários',
      views: {
        timeGridDay: { titleFormat: { weekday: 'long', year: 'numeric', month: 'long', day: 'numeric' } },
        dayGridMonth: { titleFormat: { year: 'numeric', month: 'long' } },
        lista: { type: 'list', duration: { days: 30 }, buttonText: 'Lista', titleFormat: { year: 'numeric', month: 'short', day: 'numeric' } },
      },
      events: (info, pronto) => pronto(eventos()),
      eventContent: conteudoEvento,
      dayHeaderContent: cabecalhoDia,
      eventDidMount: (info) => {
        const a = info.event.extendedProps.ag;
        if (a) info.el.setAttribute('aria-label', descricaoFalada(a) + '. Abrir os detalhes.');
      },
      eventClick: (info) => {
        info.jsEvent.preventDefault();
        const a = info.event.extendedProps.ag;
        if (a) abrirDetalhes(a);
      },
      dateClick: (info) => {
        const d = info.date;
        if (info.allDay) {
          if (dataISO(d) < dataISO(new Date())) { toast('Esse dia já passou. Escolha de hoje em diante.', 'erro'); return; }
          abrirMarcar({ data: dataISO(d) });
          return;
        }
        if (d.getTime() <= Date.now()) { toast('Esse horário já passou. Clique em um horário daqui para frente.', 'erro'); return; }
        abrirMarcar({ data: dataISO(d), hora: horaDe(d.getHours() * 60 + d.getMinutes()) });
      },
      datesSet: (info) => {
        if (tela !== t) return;
        const agora = new Date();
        const temHoje = info.view.currentStart <= agora && agora < info.view.currentEnd;
        S.agendaVisao = info.view.type;
        // Se a pessoa está no período de hoje, a volta à tela abre no hoje de então.
        S.agendaData = temHoje ? null : info.view.calendar.getDate();
        t.titulo.textContent = maiuscula(info.view.title);
        const v = VISOES.find((x) => x.id === info.view.type) || VISOES[0];
        t.anterior.setAttribute('aria-label', v.anterior);
        t.anterior.title = v.anterior;
        t.proximo.setAttribute('aria-label', v.proximo);
        t.proximo.title = v.proximo;
        t.visoes.querySelectorAll('button').forEach((b) => {
          b.setAttribute('aria-pressed', b.dataset.visao === info.view.type ? 'true' : 'false');
        });
      },
      windowResize: () => ajustarAltura(),
    });
    t.cal = cal;
    cal.render();
    // A altura depende de onde o calendário ficou na página: confere de novo depois do desenho.
    requestAnimationFrame(() => { if (tela === t) ajustarAltura(); });
  }

  // alturaIdeal: no computador o calendário ocupa a altura que sobra na janela. No celular
  // ele cresce com o conteúdo e a página rola.
  function alturaIdeal() {
    const t = tela;
    if (telaEstreita()) return 'auto';
    // Soma as posições dos pais em vez de medir na tela: a entrada animada da tela desloca
    // a medida por um instante e a conta sairia errada.
    let topoEl = 0;
    for (let el = t.calEl; el; el = el.offsetParent) topoEl += el.offsetTop;
    const pe = t.pe ? t.pe.offsetHeight : 0;
    return Math.max(480, Math.round(window.innerHeight - topoEl - pe - 34));
  }

  function ajustarAltura() {
    const t = tela;
    if (!t || !t.cal) return;
    const altura = alturaIdeal();
    if (altura === t.altura) return;
    t.altura = altura;
    t.cal.setOption('height', altura);
  }

  // refletir põe na tela o que está em S.agenda e S.config, sem mexer na data nem na visão.
  function refletir() {
    const t = tela;
    if (!t || !t.pronta) return;
    const ativa = !!(S.agenda && S.agenda.ativa);
    if (t.faixa.hidden !== ativa) {
      t.faixa.hidden = ativa;
      if (!ativa) animar(t.faixa, { opacity: [0, 1], y: [-6, 0] }, { duration: 0.2 });
    }
    pintarLegenda();
    if (t.cal) {
      const regras = opcoesDasRegras();
      const marcaRegras = marcaDasRegras(regras);
      if (marcaRegras !== t.marcaRegras) {
        t.marcaRegras = marcaRegras;
        t.calEl.style.setProperty('--altura-linha', regras.alturaLinha + 'px');
        t.cal.batchRendering(() => {
          for (const k of ['businessHours', 'slotDuration', 'snapDuration', 'slotLabelInterval', 'slotMinTime', 'slotMaxTime']) {
            t.cal.setOption(k, regras[k]);
          }
        });
      }
      const agora = Date.now();
      const marcaEventos = JSON.stringify([S.agendaCancelados, agendamentos().map((a) => [
        a.id, a.status, a.inicio, a.duracao_min, a.nome, a.servico, a.origem, a.obs, a.telefone_bonito,
        fimDe(a).getTime() < agora,
      ])]);
      if (marcaEventos !== t.marcaEventos) {
        t.marcaEventos = marcaEventos;
        t.cal.refetchEvents();
      }
      ajustarAltura();
    }
    pintarLado();
  }

  function pintarLegenda() {
    const t = tela;
    const marca = String(S.agendaCancelados);
    if (t.marcaLegenda === marca) return;
    t.marcaLegenda = marca;
    trocar(t.legenda,
      h('span', null, h('span', { class: 'amostra amostra-marcado', 'aria-hidden': 'true' }), 'Marcado'),
      h('span', null, seloIA(), 'Marcado pelo assistente'),
      S.agendaCancelados ? h('span', null, h('span', { class: 'amostra amostra-cancelado', 'aria-hidden': 'true' }), 'Cancelado') : null,
      h('span', null, h('span', { class: 'amostra amostra-fechado', 'aria-hidden': 'true' }), 'Fora do horário de atendimento'));
  }

  // ---------- Coluna lateral: hoje e próximos ----------

  function itemLado(a, comDia) {
    const passou = fimDe(a).getTime() < Date.now();
    return h('li', null, h('button', {
      type: 'button', class: 'lado-item' + (passou ? ' passado' : ''),
      'aria-label': (comDia ? diaCompleto(a.inicio) + ', ' : '') + descricaoFalada(a) + '. Abrir os detalhes.',
      onclick: () => abrirDetalhes(a),
    },
      h('span', { class: 'lado-quando' },
        comDia ? h('span', { class: 'lado-dia', text: diaRotulo(a.inicio) }) : null,
        h('span', { class: 'lado-hora', text: horaCurta(a.inicio) })),
      h('span', { class: 'lado-textos' },
        h('span', { class: 'lado-nome' }, h('span', { text: a.nome || 'Sem nome' }), a.origem === 'assistente' ? seloIA() : null),
        h('span', { class: 'lado-servico', text: (a.servico || 'Horário') + (passou ? ', já passou' : '') }))));
  }

  function pintarLado() {
    const t = tela;
    if (!t || !t.hoje) return;
    const agora = new Date();
    const fimDeHoje = new Date(agora.getFullYear(), agora.getMonth(), agora.getDate() + 1);
    const marcados = agendamentos()
      .filter((a) => a.status === 'marcado' && !isNaN(new Date(a.inicio)))
      .sort((a, b) => new Date(a.inicio) - new Date(b.inicio));
    const deHoje = marcados.filter((a) => mesmoDia(new Date(a.inicio), agora));
    const depois = marcados.filter((a) => new Date(a.inicio) >= fimDeHoje);
    const proximos = depois.slice(0, 6);
    const curto = (a) => [a.id, a.inicio, a.duracao_min, a.nome, a.servico, a.origem, fimDe(a).getTime() < agora.getTime()];
    const regras = regrasSalvas();
    const marca = JSON.stringify([deHoje.map(curto), proximos.map(curto), depois.length, regras.servicos.length, regras.intervalo_min]);
    if (marca === t.marcaLado) return;
    const primeira = t.marcaLado == null;
    t.marcaLado = marca;

    t.contaHoje.textContent = deHoje.length === 1 ? '1 horário' : deHoje.length + ' horários';
    trocar(t.hoje, deHoje.length
      ? deHoje.map((a) => itemLado(a, false))
      : h('li', { class: 'lado-vazio', text: 'Nenhum horário marcado para hoje.' }));
    trocar(t.proximos, proximos.length
      ? proximos.map((a) => itemLado(a, true))
      : h('li', { class: 'lado-vazio', text: 'Nenhum horário marcado para os próximos dias.' }));
    if (depois.length > proximos.length) {
      t.proximos.appendChild(h('li', { class: 'lado-vazio', text: 'E mais ' + (depois.length - proximos.length) + ' depois destes.' }));
    }
    const n = regras.servicos.length;
    t.resumoRegras.textContent = (n === 0 ? 'Nenhum serviço cadastrado' : (n === 1 ? '1 serviço cadastrado' : n + ' serviços cadastrados')) +
      '. Horários de ' + regras.intervalo_min + ' em ' + regras.intervalo_min + ' minutos.';
    // As listas entram em sequência só no primeiro desenho.
    if (primeira) entrarEmSequencia(t.main.querySelectorAll('.lado-lista > li'));
  }

  // ---------- Ligar a agenda ----------

  async function ligarAgenda() {
    await carregarConfig(true);
    await salvarConfig({ agenda: Object.assign({}, S.config.agenda || {}, { ativa: true }) });
    await carregarAgenda();
    toast('Agenda ligada. O assistente já oferece os horários livres.');
    refletir();
  }

  // ---------- Janelas ----------

  function abrirJanela(opc) {
    const t = tela;
    const j = janela(Object.assign({}, opc, {
      aoFechar: () => {
        if (t) t.janelas.delete(j);
        if (opc.aoFechar) opc.aoFechar();
      },
    }));
    if (t) t.janelas.add(j);
    return j;
  }

  // Marcar um horário. pre: { data: '2026-11-12', hora: '14:00' } quando vem de um clique no calendário.
  function abrirMarcar(pre) {
    const m = S.marcar || (S.marcar = { nome: '', telefone: '', servico: '', data: '', hora: '', obs: '' });
    const regras = regrasSalvas();
    const servicos = (S.agenda && S.agenda.servicos) || [];
    const livres = (S.agenda && S.agenda.livres) || [];
    const hoje = dataISO(new Date());
    const horasLivres = (data) => ((livres.find((d) => d.data === data) || {}).horas || []);

    if (pre && pre.data) { m.data = pre.data; m.hora = pre.hora || ''; }
    if (!m.data || m.data < hoje) {
      m.data = (livres.find((d) => (d.horas || []).length) || {}).data || hoje;
      m.hora = '';
    }
    if (!m.hora) m.hora = horasLivres(m.data)[0] || '';
    if (servicos.length && !servicos.some((s) => s.nome === m.servico)) m.servico = servicos[0].nome;

    const idForm = 'form-marcar';
    const campoNome = entrada(m, 'nome', { autocomplete: 'off', maxLength: 120, required: true });
    const campoHora = entrada(m, 'hora', { type: 'time', step: divisao(regras) * 60, required: true }, () => pintarLivres());
    const campoData = entrada(m, 'data', { type: 'date', min: hoje, required: true }, () => {
      if (!horasLivres(m.data).includes(m.hora) && horasLivres(m.data).length) {
        m.hora = horasLivres(m.data)[0];
        campoHora.value = m.hora;
      }
      pintarLivres();
    });
    const campoServico = servicos.length
      ? seletor(m, 'servico', servicos.map((s) => ({
        valor: s.nome,
        texto: s.nome + (s.duracao_min ? ' (' + s.duracao_min + ' min' + (s.preco ? ', ' + s.preco : '') + ')' : ''),
      })))
      : entrada(m, 'servico', { placeholder: 'Ex.: Consulta', autocomplete: 'off', maxLength: 120 });

    // Atalho: os horários livres do dia escolhido, pelas regras da agenda.
    const livresEl = h('div', { class: 'livres' });
    function pintarLivres() {
      const horas = horasLivres(m.data);
      if (!horas.length) {
        trocar(livresEl, h('p', { class: 'ajuda', text: 'Pelas regras, não há horário livre neste dia. Por aqui você pode marcar mesmo assim.' }));
        return;
      }
      trocar(livresEl,
        h('p', { class: 'rotulo', id: 'livres-rotulo', text: 'Horários livres neste dia' }),
        h('div', { class: 'livres-lista', role: 'group', 'aria-labelledby': 'livres-rotulo' },
          horas.map((x) => h('button', {
            type: 'button', class: 'livre', 'aria-pressed': x === m.hora ? 'true' : 'false',
            onclick: () => { m.hora = x; campoHora.value = x; pintarLivres(); },
          }, x))));
    }
    pintarLivres();

    const enviar = botao('Marcar horário', { classe: 'btn-primario', icone: 'check', tipo: 'submit' });
    enviar.setAttribute('form', idForm);
    const form = h('form', { class: 'pilha', id: idForm, novalidate: true },
      h('div', { class: 'grade-marcar' },
        h('div', { class: 'largo' }, campo('Nome do cliente', campoNome)),
        campo('Telefone', entrada(m, 'telefone', { type: 'tel', inputMode: 'tel', placeholder: '5527999990000', autocomplete: 'off', maxLength: 20 }), 'Com DDI e DDD. Pode ficar vazio.'),
        campo('Serviço', campoServico),
        campo('Dia', campoData),
        campo('Hora', campoHora),
        h('div', { class: 'largo' }, livresEl),
        h('div', { class: 'largo' }, campo('Observação', entrada(m, 'obs', { autocomplete: 'off', maxLength: 500 }), 'Só a equipe vê. Pode ficar vazio.'))));

    const j = abrirJanela({
      titulo: 'Marcar um horário',
      corpo: form,
      pe: [botao('Cancelar', { classe: 'btn-suave', aoClicar: () => j.fechar() }), enviar],
    });
    form.addEventListener('submit', (ev) => {
      ev.preventDefault();
      ocupar(enviar, async () => {
        if (!m.nome.trim()) { campoNome.focus(); throw new Error('Escreva o nome do cliente.'); }
        if (!m.data || !m.hora) throw new Error('Escolha o dia e a hora.');
        if (!(new Date(m.data + 'T' + m.hora) > new Date())) throw new Error('Esse horário já passou. Escolha um horário daqui para frente.');
        const data = m.data;
        await api.post('/api/agenda/marcar', {
          nome: m.nome.trim(), telefone: m.telefone.trim(), servico: m.servico || '',
          data, hora: m.hora, obs: m.obs.trim(),
        });
        m.nome = ''; m.telefone = ''; m.obs = ''; m.hora = '';
        j.fechar(true);
        toast('Horário marcado.');
        await recarregar();
        // Se o dia marcado não está na tela, o calendário vai até ele.
        if (tela && tela.cal) {
          const v = tela.cal.view;
          const dia = new Date(data + 'T12:00');
          if (dia < v.currentStart || dia >= v.currentEnd) tela.cal.gotoDate(dia);
        }
      });
    });
    if (!telaEstreita()) campoNome.focus();
  }

  // Detalhes de um horário, com o botão de cancelar.
  function abrirDetalhes(a) {
    const inicio = new Date(a.inicio);
    const fim = fimDe(a);
    const cancelado = a.status === 'cancelado';
    const passou = fim.getTime() < Date.now();
    const quando = maiuscula(DIAS_LONGOS[inicio.getDay()].toLowerCase()) + ', ' +
      doisDigitos(inicio.getDate()) + '/' + doisDigitos(inicio.getMonth() + 1) + '/' + inicio.getFullYear() +
      ', das ' + horaCurta(a.inicio) + ' às ' + horaCurta(fim) +
      (a.duracao_min ? ' (' + a.duracao_min + ' min)' : '');
    const conversa = a.lead_id && S.estado && (S.estado.leads || []).some((l) => l.id === a.lead_id);

    const situacao = cancelado
      ? h('span', { class: 'etiqueta etiqueta-perigo', text: 'Cancelado' })
      : (passou ? h('span', { class: 'etiqueta', text: 'Já passou' }) : h('span', { class: 'etiqueta etiqueta-ok' }, icone('check'), 'Marcado'));

    const acoes = [];
    let j = null;
    if (conversa) {
      acoes.push(botao('Abrir a conversa', {
        classe: 'btn-fantasma', icone: 'conversas',
        aoClicar: () => { S.aberto = a.lead_id; j.fechar(true); location.hash = '#conversas'; },
      }));
    }
    if (!cancelado && !passou) {
      acoes.push(botao('Cancelar horário', {
        classe: 'btn-perigo-suave', icone: 'lixo',
        aoClicar: async () => {
          const sim = await confirmar({
            titulo: 'Cancelar este horário?',
            texto: (a.servico || 'Horário') + ' de ' + (a.nome || 'cliente') + ', ' + diaCompleto(a.inicio) + ' às ' + horaCurta(a.inicio) +
              '. O cliente não é avisado: se precisar, fale com ele.',
            botao: 'Cancelar horário', cancelar: 'Manter horário', perigo: true,
          });
          if (!sim) return;
          await api.post('/api/agenda/cancelar', { id: a.id });
          j.fechar(true);
          toast('Horário cancelado.');
          await recarregar();
        },
      }));
    }
    acoes.push(botao('Fechar', { classe: 'btn-suave', aoClicar: () => j.fechar() }));

    j = abrirJanela({
      titulo: a.servico || 'Horário marcado',
      fecharFora: true,
      corpo: [
        h('div', { class: 'ficha-linha' }, situacao,
          a.origem === 'assistente' ? h('span', { class: 'etiqueta' }, icone('ia'), 'Marcado pelo assistente') : null),
        h('dl', { class: 'dados detalhes' },
          definicao('Cliente', a.nome, 'Sem nome'),
          definicao('Telefone', a.telefone_bonito || a.telefone, 'Não informado'),
          definicao('Serviço', a.servico, 'Não informado'),
          definicao('Horário', quando),
          definicao('Quem marcou', a.origem === 'assistente' ? 'O assistente, pelo WhatsApp' : 'Alguém da equipe, pelo painel'),
          definicao('Observação', a.obs, 'Nenhuma')),
      ],
      pe: acoes,
    });
  }

  // Regras da agenda: serviços, dias, horários, intervalo, vagas, antecedência e avisos.
  function abrirRegras() {
    if (!S.regras) S.regras = regrasSalvas();
    const r = S.regras;
    const situacao = h('p', { class: 'situacao' + (S.regrasSujas ? ' sujo' : ''), 'aria-live': 'polite', text: S.regrasSujas ? 'Alterações não salvas' : '' });
    const sujar = () => {
      S.regrasSujas = true;
      situacao.textContent = 'Alterações não salvas';
      situacao.classList.add('sujo');
    };

    const servicos = h('div', { class: 'pilha' });
    const pintarServicos = () => {
      trocar(servicos,
        r.servicos.map((s, i) => h('div', { class: 'linha-servico' },
          campo('Serviço', entrada(s, 'nome', { placeholder: 'Ex.: Consulta', autocomplete: 'off' }, sujar)),
          campo('Duração (min)', entrada(s, 'duracao_min', { type: 'number', min: 5, max: 600, step: 5, inputMode: 'numeric' }, sujar)),
          h('div', { class: 'campo-preco' }, campo('Preço', entrada(s, 'preco', { placeholder: 'Ex.: R$ 80', autocomplete: 'off' }, sujar))),
          botao('', {
            icone: 'lixo', classe: 'btn-icone', rotulo: 'Remover o serviço ' + (s.nome || (i + 1)),
            aoClicar: () => { r.servicos.splice(i, 1); sujar(); pintarServicos(); },
          }))),
        r.servicos.length ? null : h('p', { class: 'ajuda', text: 'Sem serviços cadastrados, cada horário dura o tempo do intervalo.' }));
    };
    pintarServicos();

    const dias = h('div', { class: 'dias' },
      h('div', { class: 'linha-dia cabeca', 'aria-hidden': 'true' },
        h('span', { text: 'Dia' }), h('span', { text: 'Abre' }), h('span', { text: 'Fecha' }),
        h('span', { text: 'Pausa de' }), h('span', { text: 'Pausa até' })),
      ORDEM_DIAS.map((d) => {
        const dia = r.dias[d];
        const nome = DIAS_LONGOS[d];
        const linha = h('div', { class: 'linha-dia' + (dia.aberto ? '' : ' fechado') });
        const marca = h('input', { type: 'checkbox', checked: dia.aberto });
        marca.addEventListener('change', () => {
          dia.aberto = marca.checked;
          linha.classList.toggle('fechado', !dia.aberto);
          sujar();
        });
        const hora = (chaveDia, curto, longo) => h('label', { class: 'hora-campo' },
          h('span', { class: 'hora-rotulo', text: curto }),
          entrada(dia, chaveDia, { type: 'time', 'aria-label': nome + ', ' + longo }, sujar));
        return por(linha, [
          h('label', { class: 'marcar dia-rotulo' }, marca, h('span', { text: nome })),
          hora('inicio', 'Abre', 'abre às'), hora('fim', 'Fecha', 'fecha às'),
          hora('pausa_inicio', 'Pausa de', 'pausa a partir de'), hora('pausa_fim', 'Pausa até', 'pausa até'),
        ]);
      }));

    const intervalos = INTERVALOS.includes(r.intervalo_min) ? INTERVALOS : INTERVALOS.concat(r.intervalo_min).sort((a, b) => a - b);
    const avisar = h('input', { type: 'checkbox', checked: r.avisar_humano });
    avisar.addEventListener('change', () => { r.avisar_humano = avisar.checked; sujar(); });

    let j = null;
    const salvar = botao('Salvar regras', {
      classe: 'btn-primario',
      aoClicar: async () => {
        const limpa = normalizarRegras(r);
        limpa.servicos = limpa.servicos.filter((s) => s.nome.trim());
        S.regrasSujas = false; // para o rascunho nascer de novo da configuração salva
        try {
          await salvarConfig({ agenda: limpa });
        } catch (e) {
          S.regrasSujas = true;
          throw e;
        }
        S.regras = null;
        await carregarAgenda();
        j.fechar(true);
        toast(S.config.agenda && S.config.agenda.ativa ? 'Regras salvas. A agenda está ligada.' : 'Regras salvas. A agenda está desligada.');
        refletir();
      },
    });

    const corpo = h('div', { class: 'pilha regras' },
      h('div', { class: 'regras-ligar' },
        chave('Agenda ligada', r.ativa, (novo) => { r.ativa = novo; sujar(); }),
        h('p', { class: 'ajuda', text: 'Com a agenda ligada, o assistente mostra os horários livres para o cliente e marca na hora.' })),
      h('fieldset', { class: 'grupo' },
        h('legend', { text: 'Serviços' }),
        servicos,
        h('div', null, botao('Adicionar serviço', {
          icone: 'mais',
          aoClicar: () => { r.servicos.push({ nome: '', duracao_min: r.intervalo_min || 30, preco: '' }); sujar(); pintarServicos(); },
        }))),
      h('fieldset', { class: 'grupo' },
        h('legend', { text: 'Dias e horários de atendimento' }),
        h('p', { class: 'ajuda', text: 'Marque os dias em que você atende. A pausa é opcional, por exemplo o horário de almoço.' }),
        dias),
      h('fieldset', { class: 'grupo' },
        h('legend', { text: 'Como os horários são oferecidos' }),
        h('div', { class: 'grade-campos' },
          campo('Intervalo entre horários', seletor(r, 'intervalo_min', intervalos.map((n) => ({ valor: n, texto: n + ' minutos' })), sujar, true), 'De quanto em quanto tempo começa um horário.'),
          campo('Vagas ao mesmo tempo', entrada(r, 'vagas', { type: 'number', min: 1, max: 50, inputMode: 'numeric' }, sujar), 'Quantas pessoas você atende no mesmo horário.'),
          campo('Antecedência mínima, em minutos', entrada(r, 'antecedencia_min', { type: 'number', min: 0, max: 10080, step: 5, inputMode: 'numeric' }, sujar), 'Quanto tempo antes a pessoa precisa marcar.'),
          campo('Até quantos dias à frente', entrada(r, 'dias_a_frente', { type: 'number', min: 1, max: 60, inputMode: 'numeric' }, sujar), 'Até que dia o assistente oferece horários.')),
        h('label', { class: 'marcar' }, avisar, h('span', { text: 'Avisar a equipe pelo WhatsApp a cada horário marcado pelo assistente' }))));

    j = abrirJanela({
      titulo: 'Regras da agenda',
      descricao: 'O assistente só oferece horários dentro destas regras.',
      classe: 'janela-larga',
      corpo,
      pe: [situacao, botao('Cancelar', { classe: 'btn-suave', aoClicar: () => j.fechar() }), salvar],
      // Com alterações por salvar, a janela pergunta antes de fechar.
      podeFechar: async () => {
        if (!S.regrasSujas) { S.regras = null; return true; }
        const sim = await confirmar({
          titulo: 'Sair sem salvar?',
          texto: 'As alterações que você fez nas regras da agenda se perdem.',
          botao: 'Sair sem salvar', cancelar: 'Continuar editando', perigo: true,
        });
        if (sim) { S.regras = null; S.regrasSujas = false; }
        return sim;
      },
    });
  }

  TELAS.agenda = { montar, atualizar, sair };
})();
