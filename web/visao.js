'use strict';

// Página Início: a visão geral do painel. Mostra se está tudo funcionando, os números de
// hoje e da semana, quem está esperando uma pessoa, os próximos horários e as últimas
// conversas. Os dados vêm de GET /api/visao.
(() => {
  let tela = null; // { corpo, marca, buscando, quando }

  const inteiro = (v) => Number(v) || 0;
  const plural = (n, um, varios) => (n === 1 ? '1 ' + um : n + ' ' + varios);
  const SITUACOES = { fila: 'Na fila', atendimento: 'Com a equipe', assistente: 'Com o assistente' };

  function quandoFoi(iso) {
    const t = tempoRelativo(iso);
    return t || 'ainda não aconteceu';
  }

  async function buscar(forcar) {
    const minha = tela;
    if (!minha || minha.buscando) return;
    if (!forcar && Date.now() - minha.quando < 9000) return;
    minha.buscando = true;
    try {
      const r = await api.get('/api/visao');
      if (tela !== minha) return;
      minha.quando = Date.now();
      // Só redesenha quando algo mudou (sem contar a hora exata do pedido).
      const marca = JSON.stringify(Object.assign({}, r, { agora: '' })) + '|' + Math.floor(Date.now() / 60000);
      if (marca !== minha.marca) {
        minha.marca = marca;
        pintar(r);
      }
    } catch (e) {
      if (tela === minha && !minha.marca) trocar(minha.corpo, falhaCarga(e, () => buscar(true)));
    } finally {
      minha.buscando = false;
    }
  }

  function montar(main) {
    const corpo = h('div', { class: 'vs' }, carregandoEl('Buscando os dados de hoje...'));
    tela = { corpo, marca: '', buscando: false, quando: 0 };
    const nome = SESSAO.usuario && SESSAO.usuario.nome ? String(SESSAO.usuario.nome).trim().split(/\s+/)[0] : '';
    por(main, [
      h('div', { class: 'topo-tela' },
        h('div', { class: 'topo-textos' },
          h('h1', { text: nome ? 'Olá, ' + nome : 'Início' }),
          h('p', { class: 'descricao', text: 'Aqui você vê se o atendimento está funcionando e o que aconteceu hoje.' }))),
      corpo,
    ]);
    return buscar(true);
  }

  function atualizar() { buscar(false); }
  function sair() { tela = null; }

  // ---------- Está funcionando? ----------

  function blocoFuncionando(r) {
    const pecas = Array.isArray(r.pecas) ? r.pecas : [];
    const comProblema = pecas.filter((p) => !p.certo && p.chave !== 'agenda');
    const tudoCerto = comProblema.length === 0;
    const admin = ACESSO.pode('admin');
    const atalho = (p) => {
      if (p.certo) return null;
      if (p.chave === 'ia' && admin && typeof CONEXAO !== 'undefined') {
        return botao('Conectar a IA', { classe: 'btn-primario vs-atalho', icone: 'ia', aoClicar: () => CONEXAO.abrir() });
      }
      if (p.chave === 'whatsapp' && admin) return h('a', { class: 'btn btn-suave vs-atalho', href: '#ajustes' }, h('span', { text: 'Conectar o WhatsApp' }));
      if (p.chave === 'agenda') return h('a', { class: 'btn btn-suave vs-atalho', href: '#agenda' }, h('span', { text: 'Abrir a agenda' }));
      return null;
    };
    return h('section', { class: 'cartao vs-bloco', 'aria-labelledby': 'vs-t-funciona' },
      h('div', { class: 'vs-cabeca' },
        h('h2', { id: 'vs-t-funciona', text: 'Está funcionando?' }),
        h('p', { class: 'vs-veredito ' + (tudoCerto ? 'vs-ok' : 'vs-ruim'), role: 'status' },
          icone(tudoCerto ? 'check' : 'alerta'),
          h('span', { text: tudoCerto
            ? 'Sim. O assistente está respondendo os clientes.'
            : (comProblema.length === 1 ? 'Ainda não. Falta resolver 1 ponto.' : 'Ainda não. Faltam ' + comProblema.length + ' pontos.') }))),
      h('ul', { class: 'vs-pecas' }, pecas.map((p) => {
        // Agenda desligada não é defeito: é uma escolha do negócio.
        const tom = p.certo ? 'ok' : (p.chave === 'agenda' ? 'neutro' : 'ruim');
        return h('li', { class: 'vs-peca vs-peca-' + tom },
          h('span', { class: 'vs-peca-sinal', 'aria-hidden': 'true' }, icone(tom === 'ok' ? 'check' : (tom === 'ruim' ? 'alerta' : 'info'))),
          h('div', { class: 'vs-peca-textos' },
            h('strong', { text: p.titulo }),
            h('span', { text: p.detalhe })),
          atalho(p));
      })),
      h('dl', { class: 'vs-provas' },
        h('div', null, h('dt', { text: 'Última mensagem de cliente' }), h('dd', { text: quandoFoi(r.ultima_mensagem) })),
        h('div', null, h('dt', { text: 'Última resposta do assistente' }), h('dd', { text: quandoFoi(r.ultima_resposta) })),
        h('div', null, h('dt', { text: 'Programa ligado' }), h('dd', { text: quandoFoi(r.no_ar_desde) }))));
  }

  // ---------- Números ----------

  function blocoNumeros(r) {
    const hoje = r.hoje || {}, semana = r.semana || {}, sit = r.situacao || {};
    const naSemana = (chave) => 'Nos últimos 7 dias: ' + inteiro(semana[chave]);
    const itens = [
      { rotulo: 'Mensagens de clientes hoje', valor: hoje.mensagens_clientes, nota: naSemana('mensagens_clientes') },
      { rotulo: 'Respostas do assistente hoje', valor: hoje.respostas_assistente, nota: naSemana('respostas_assistente') },
      { rotulo: 'Contatos novos hoje', valor: hoje.novas, nota: naSemana('novas') },
      { rotulo: 'Horários marcados hoje', valor: hoje.agendamentos, nota: naSemana('agendamentos') },
      { rotulo: 'Passadas para a equipe hoje', valor: hoje.transbordos, nota: naSemana('transbordos') },
      { rotulo: 'Contatos no total', valor: r.contatos_no_total, nota: plural(inteiro(sit.quentes), 'lead quente agora', 'leads quentes agora') },
    ];
    return h('section', { class: 'vs-numeros-area', 'aria-labelledby': 'vs-t-numeros' },
      h('div', { class: 'vs-cabeca' },
        h('h2', { id: 'vs-t-numeros', text: 'Hoje em números' }),
        h('a', { class: 'vs-link', href: '#relatorio' }, 'Ver relatório completo')),
      h('ul', { class: 'vs-numeros' }, itens.map((i) => h('li', { class: 'vs-numero' + (inteiro(i.valor) ? '' : ' vs-numero-zero') },
        h('span', { class: 'vs-numero-rotulo', text: i.rotulo }),
        h('strong', { class: 'vs-numero-valor', text: String(inteiro(i.valor)) }),
        h('span', { class: 'vs-numero-nota', text: i.nota })))));
  }

  // ---------- Movimento da semana ----------

  function blocoSemana(r) {
    const dias = Array.isArray(r.por_dia) ? r.por_dia : [];
    const maior = Math.max(1, ...dias.map((d) => inteiro(d.mensagens)));
    const total = dias.reduce((s, d) => s + inteiro(d.mensagens), 0);
    return h('section', { class: 'cartao vs-bloco', 'aria-labelledby': 'vs-t-semana' },
      h('div', { class: 'vs-cabeca' },
        h('h2', { id: 'vs-t-semana', text: 'Movimento dos últimos 7 dias' }),
        h('p', { class: 'vs-nota', text: plural(total, 'mensagem trocada', 'mensagens trocadas') })),
      total
        ? h('ol', { class: 'vs-barras', 'aria-label': 'Mensagens trocadas por dia, somando clientes, assistente e equipe' }, dias.map((d) => {
          const n = inteiro(d.mensagens);
          return h('li', { class: 'vs-barra-dia', title: d.rotulo + ': ' + plural(n, 'mensagem', 'mensagens') },
            h('span', { class: 'vs-barra-valor', text: String(n) }),
            h('span', { class: 'vs-barra', 'aria-hidden': 'true', style: 'height:' + Math.max(n ? 6 : 2, Math.round((n / maior) * 100)) + '%' }),
            h('span', { class: 'vs-barra-rotulo', text: String(d.rotulo || '').split(' ')[0] }));
        }))
        : h('p', { class: 'vs-vazio', text: 'Ainda não houve conversa nesta semana. Mande um "Oi" de outro celular para o número do negócio e veja aparecer aqui.' }));
  }

  // ---------- Precisa de você ----------

  function blocoFila(r) {
    const fila = Array.isArray(r.fila) ? r.fila : [];
    const total = inteiro(r.fila_total);
    return h('section', { class: 'cartao vs-bloco', 'aria-labelledby': 'vs-t-fila' },
      h('div', { class: 'vs-cabeca' },
        h('h2', { id: 'vs-t-fila' }, 'Esperando uma pessoa', total ? h('span', { class: 'vs-conta vs-conta-alerta', text: String(total) }) : null),
        h('a', { class: 'vs-link', href: '#conversas' }, 'Abrir as conversas')),
      fila.length
        ? h('ul', { class: 'vs-lista' }, fila.map((f) => h('li', null,
          h('a', { class: 'vs-item', href: '#conversas/' + encodeURIComponent(f.id) },
            h('span', { class: 'vs-item-textos' },
              h('strong', { text: f.exibir }),
              h('span', { text: f.motivo ? 'Motivo: ' + f.motivo : 'O assistente passou a conversa para a equipe' })),
            h('span', { class: 'vs-item-lado' }, etiquetaTemp(f.classificacao), h('span', { class: 'vs-quando', text: 'espera ' + quandoFoi(f.desde) }))))))
        : h('p', { class: 'vs-vazio' }, icone('check'), h('span', { text: 'Ninguém está esperando. O assistente está dando conta.' })),
      total > fila.length ? h('p', { class: 'vs-nota', text: 'E mais ' + (total - fila.length) + ' na fila.' }) : null);
  }

  // ---------- Próximos horários ----------

  function blocoAgenda(r) {
    const prox = Array.isArray(r.proximos) ? r.proximos : [];
    const total = inteiro(r.proximos_total);
    const quando = (iso) => {
      const d = new Date(iso);
      if (isNaN(d)) return '';
      return diaRotulo(iso) + ', ' + horaCurta(iso);
    };
    return h('section', { class: 'cartao vs-bloco', 'aria-labelledby': 'vs-t-agenda' },
      h('div', { class: 'vs-cabeca' },
        h('h2', { id: 'vs-t-agenda' }, 'Próximos horários', total ? h('span', { class: 'vs-conta', text: String(total) }) : null),
        h('a', { class: 'vs-link', href: '#agenda' }, 'Abrir a agenda')),
      prox.length
        ? h('ul', { class: 'vs-lista' }, prox.map((ag) => h('li', null,
          h('div', { class: 'vs-item vs-item-parado' },
            h('span', { class: 'vs-item-textos' },
              h('strong', { text: ag.nome || 'Sem nome' }),
              h('span', { text: (ag.servico || 'Horário marcado') + (ag.origem === 'assistente' ? ', marcado pelo assistente' : '') })),
            h('span', { class: 'vs-item-lado' }, h('span', { class: 'vs-quando vs-quando-forte', text: quando(ag.inicio) }))))))
        : h('p', { class: 'vs-vazio', text: 'Nenhum horário marcado daqui para frente.' }),
      total > prox.length ? h('p', { class: 'vs-nota', text: 'E mais ' + (total - prox.length) + ' na agenda.' }) : null);
  }

  // ---------- Últimas conversas ----------

  function blocoRecentes(r) {
    const lista = Array.isArray(r.recentes) ? r.recentes : [];
    const quem = { cliente: '', atendente: 'Assistente: ', humano: 'Equipe: ' };
    return h('section', { class: 'cartao vs-bloco', 'aria-labelledby': 'vs-t-recentes' },
      h('div', { class: 'vs-cabeca' },
        h('h2', { id: 'vs-t-recentes', text: 'Últimas conversas' }),
        h('a', { class: 'vs-link', href: '#contatos' }, 'Ver todos os contatos')),
      lista.length
        ? h('ul', { class: 'vs-lista' }, lista.map((c) => h('li', null,
          h('a', { class: 'vs-item', href: '#conversas/' + encodeURIComponent(c.id) },
            h('span', { class: 'vs-item-textos' },
              h('strong', { text: c.exibir }),
              h('span', { text: c.previa ? (quem[c.previa_de] || '') + c.previa : 'Sem mensagens ainda' })),
            h('span', { class: 'vs-item-lado' },
              h('span', { class: 'vs-situacao vs-situacao-' + (c.situacao || 'assistente'), text: SITUACOES[c.situacao] || SITUACOES.assistente }),
              h('span', { class: 'vs-quando', text: tempoRelativo(c.ultima_msg) }))))))
        : h('p', { class: 'vs-vazio', text: 'Ainda não há conversas. Elas aparecem aqui assim que um cliente escrever.' }));
  }

  function pintar(r) {
    if (!tela) return;
    trocar(tela.corpo,
      blocoFuncionando(r),
      blocoNumeros(r),
      h('div', { class: 'vs-colunas' }, blocoFila(r), blocoAgenda(r)),
      h('div', { class: 'vs-colunas' }, blocoRecentes(r), blocoSemana(r)));
  }

  TELAS.visao = { montar, atualizar, sair };
})();
