'use strict';
// Aba Assistente: dados do negócio e os três textos que formam o assistente.

(function () {
  let tela = null;

  const SUBS = [
    { id: 'negocio', rotulo: 'Negócio' },
    { id: 'personalidade', rotulo: 'Personalidade' },
    { id: 'cerebro', rotulo: 'Cérebro' },
    { id: 'regras', rotulo: 'Transbordo' },
  ];

  const TEXTOS = {
    personalidade: {
      titulo: 'Personalidade', salvar: 'Salvar personalidade',
      descricao: 'Quem o assistente é e como ele fala: o tom, a primeira mensagem, o jeito de fazer as perguntas e como ele pontua cada lead.',
      exemplo: 'Ex.: deixe o tom mais descontraído e use o nome do cliente nas respostas',
    },
    cerebro: {
      titulo: 'Cérebro', salvar: 'Salvar cérebro',
      descricao: 'Tudo o que o assistente sabe sobre o negócio. Ele só responde com o que está escrito aqui e nos materiais da base de conhecimento.',
      exemplo: 'Ex.: acrescente que aceitamos Pix e cartão em até 3 vezes',
    },
    regras: {
      titulo: 'Regras de transbordo', salvar: 'Salvar regras',
      descricao: 'Quando o assistente para de responder e chama uma pessoa da equipe.',
      exemplo: 'Ex.: chame uma pessoa sempre que o cliente falar em cancelar a assinatura',
    },
  };

  const CAMPOS_NEGOCIO = ['negocio', 'segmento', 'cidade', 'endereco', 'telefone', 'site', 'instagram',
    'produtos', 'horario', 'pagamento', 'diferenciais', 'promocoes', 'politicas', 'assistente', 'tom'];

  function novoNegocio() {
    const n = {};
    for (const c of CAMPOS_NEGOCIO) n[c] = S.config[c] || '';
    n.extras = copiar(S.config.extras || []).map((x) => ({ nome: x.nome || '', valor: x.valor || '' }));
    return n;
  }

  function temRascunho(id) {
    if (id === 'negocio') return S.negocioSujo;
    const ed = S.editores[id];
    return !!ed && ed.atual !== ed.salvo;
  }

  function pintarSubabas() {
    if (!tela) return;
    trocar(tela.subabas, SUBS.map((s) => h('a', {
      href: '#assistente/' + s.id, class: s.id === tela.sub ? 'ativo' : null,
      'aria-current': s.id === tela.sub ? 'page' : null,
    }, s.rotulo, temRascunho(s.id) ? h('span', { class: 'marca-sujo', title: 'Tem alterações não salvas' }) : null)));
  }

  async function montar(main, sub) {
    if (!SUBS.some((s) => s.id === sub)) sub = 'negocio';
    const subabas = h('nav', { class: 'subabas', 'aria-label': 'Partes do assistente' });
    const corpo = h('div');
    const minha = tela = { main, sub, subabas, corpo };
    por(main, [
      h('div', { class: 'topo-tela' }, h('div', { class: 'topo-textos' },
        h('h1', { text: 'Assistente' }),
        h('p', { class: 'descricao', text: 'O que você muda aqui vale já na próxima mensagem. Não precisa religar nada.' }))),
      subabas, corpo,
    ]);
    pintarSubabas();
    trocar(corpo, carregandoEl());
    try {
      await carregarConfig();
      if (sub !== 'negocio') await carregarTextos();
    } catch (e) {
      if (tela === minha) trocar(corpo, falhaCarga(e, () => { main.replaceChildren(); montar(main, sub); }));
      return;
    }
    if (tela !== minha) return;
    if (sub === 'negocio') pintarNegocio();
    else pintarEditor(sub);
  }

  function sair() { tela = null; }

  // ---------- Negócio ----------

  function pintarNegocio() {
    if (!S.negocio) S.negocio = novoNegocio();
    const n = S.negocio;
    const situacao = h('p', { class: 'situacao', 'aria-live': 'polite' });
    const refletir = () => {
      situacao.textContent = S.negocioSujo ? 'Alterações não salvas' : '';
      situacao.classList.toggle('sujo', S.negocioSujo);
      pintarSubabas();
    };
    const sujar = () => { if (!S.negocioSujo) { S.negocioSujo = true; refletir(); } };

    // Preencher com IA
    const ia = { descricao: '' };
    const caixaIA = h('details', { class: 'cartao' },
      h('summary', { class: 'resumo-linha' }, icone('ia'), h('span', { text: 'Preencher com a ajuda da IA' })),
      h('div', { class: 'pilha' },
        campo('Conte sobre o seu negócio', areaTexto(ia, 'descricao', {
          rows: 4, placeholder: 'Ex.: Barbearia em Vitória. Corte a R$ 40 e plano mensal de R$ 150. Aberta de segunda a sábado, das 9h às 19h. Aceita Pix e cartão.',
        }), 'Escreva do seu jeito ou cole o texto do seu site ou Instagram. A IA distribui nos campos abaixo e você revisa.'),
        h('div', { class: 'acoes' },
          botao('Preencher com IA', {
            classe: 'btn-primario', icone: 'ia',
            aoClicar: async () => {
              if (!ia.descricao.trim()) throw new Error('Escreva alguma coisa sobre o negócio primeiro.');
              const r = await api.post('/api/ia/negocio', { descricao: ia.descricao.trim() });
              let quantos = 0;
              for (const [k, v] of Object.entries(r.campos || {})) {
                if (CAMPOS_NEGOCIO.includes(k) && typeof v === 'string' && v.trim()) { n[k] = v.trim(); quantos++; }
              }
              if (!quantos) throw new Error('A IA não encontrou dados para preencher. Tente escrever com mais detalhes.');
              S.negocioSujo = true;
              toast('Campos preenchidos. Revise e salve.');
              pintarNegocio();
            },
          }),
          h('span', { class: 'espera-ia', text: 'Pode levar até um minuto.' }))));

    // Variáveis extras
    const extras = h('div', { class: 'pilha' });
    const nomeLimpo = (s) => String(s || '').normalize('NFD').replace(/[̀-ͯ]/g, '').toUpperCase().replace(/[^A-Z0-9]+/g, '_').replace(/^_+|_+$/g, '');
    const pintarExtras = () => {
      trocar(extras, n.extras.map((x, i) => {
        // A dica fica em uma linha própria, embaixo dos dois campos, para não desalinhar
        // o Nome do Valor.
        const chave = h('code');
        const mostra = h('p', { class: 'extra-chave' }, 'Nos textos, escreva ', chave);
        const pintarChave = () => {
          const limpo = nomeLimpo(x.nome);
          chave.textContent = limpo ? '{{' + limpo + '}}' : '';
          mostra.hidden = !limpo;
        };
        pintarChave();
        const nome = entrada(x, 'nome', { placeholder: 'Ex.: ESTACIONAMENTO', autocomplete: 'off' }, () => { pintarChave(); sujar(); });
        nome.addEventListener('blur', () => { x.nome = nomeLimpo(x.nome); nome.value = x.nome; });
        return h('div', { class: 'linha-extra' },
          h('div', { class: 'campo-nome' }, campo('Nome', nome)),
          h('div', { class: 'campo-valor' }, campo('Valor', entrada(x, 'valor', { placeholder: 'Ex.: Gratuito na rua ao lado', autocomplete: 'off' }, sujar))),
          botao('', {
            icone: 'lixo', classe: 'btn-icone', rotulo: 'Remover a variável ' + (x.nome || (i + 1)),
            aoClicar: () => { n.extras.splice(i, 1); sujar(); pintarExtras(); },
          }),
          mostra);
      }));
    };
    pintarExtras();

    const tom = entrada(n, 'tom', { list: 'lista-tons', autocomplete: 'off' }, sujar);
    const txt = (chaveCampo, attrs) => entrada(n, chaveCampo, Object.assign({ autocomplete: 'off' }, attrs || {}), sujar);
    const area = (chaveCampo, attrs) => areaTexto(n, chaveCampo, attrs, sujar);

    const salvar = botao('Salvar dados do negócio', {
      classe: 'btn-primario',
      aoClicar: async () => {
        if (!n.negocio.trim()) throw new Error('Escreva o nome do negócio.');
        const envio = {};
        for (const c of CAMPOS_NEGOCIO) envio[c] = String(n[c] || '').trim();
        envio.extras = n.extras.map((x) => ({ nome: nomeLimpo(x.nome), valor: String(x.valor || '').trim() })).filter((x) => x.nome);
        await salvarConfig(envio);
        S.negocio = novoNegocio();
        S.negocioSujo = false;
        toast('Dados do negócio salvos. Valem já na próxima mensagem.');
        pintarNegocio();
      },
    });

    trocar(tela.corpo, h('div', { class: 'pilha' },
      caixaIA,
      h('form', { class: 'cartao pilha', novalidate: true, onsubmit: (ev) => { ev.preventDefault(); salvar.click(); } },
        h('fieldset', { class: 'grupo' },
          h('legend', { text: 'O básico' }),
          h('div', { class: 'grade-campos' },
            campo('Nome do negócio', txt('negocio', { required: true })),
            campo('Segmento', txt('segmento', { placeholder: 'Ex.: Barbearia' })),
            campo('Cidade', txt('cidade', { placeholder: 'Ex.: São Mateus, ES' })),
            campo('Endereço', txt('endereco')),
            campo('Telefone para contato', txt('telefone', { type: 'tel', inputMode: 'tel' })),
            campo('Horário de atendimento', txt('horario', { placeholder: 'Ex.: segunda a sábado, das 9h às 19h' })),
            campo('Site', txt('site', { inputMode: 'url' })),
            campo('Instagram', txt('instagram', { placeholder: 'Ex.: @seunegocio' })))),
        h('fieldset', { class: 'grupo' },
          h('legend', { text: 'O que você vende' }),
          campo('Produtos, serviços e preços', area('produtos', { rows: 6 }), 'Um por linha fica mais fácil. O assistente só informa preço que estiver escrito aqui ou no cérebro.'),
          h('div', { class: 'grade-campos' },
            campo('Formas de pagamento', txt('pagamento', { placeholder: 'Ex.: Pix, dinheiro e cartão' })),
            campo('Promoções', txt('promocoes')),
            h('div', { class: 'largo' }, campo('Diferenciais', area('diferenciais', { rows: 3 }), 'O que faz as pessoas escolherem você.')),
            h('div', { class: 'largo' }, campo('Políticas', area('politicas', { rows: 3 }), 'Cancelamento, troca, garantia, atraso, o que valer para o seu negócio.')))),
        h('fieldset', { class: 'grupo' },
          h('legend', { text: 'Seu assistente' }),
          h('div', { class: 'grade-campos' },
            campo('Nome do assistente', txt('assistente', { placeholder: 'Ex.: Ana' }), 'É como ele se apresenta no WhatsApp.'),
            campo('Tom de voz', tom, 'Escolha um da lista ou escreva o seu.')),
          h('datalist', { id: 'lista-tons' }, (S.tons || []).map((t) => h('option', { value: t })))),
        h('fieldset', { class: 'grupo' },
          h('legend', { text: 'Variáveis extras' }),
          h('p', { class: 'ajuda', text: 'Crie as informações que quiser e use nos textos do assistente pelo nome entre chaves duplas. Por exemplo, a variável ESTACIONAMENTO vira {{ESTACIONAMENTO}}.' }),
          extras,
          h('div', null, botao('Adicionar variável', {
            icone: 'mais',
            aoClicar: () => { n.extras.push({ nome: '', valor: '' }); sujar(); pintarExtras(); },
          }))),
        h('div', { class: 'acoes' }, salvar, situacao))));
    refletir();
  }

  // ---------- Editor de texto ----------

  function inserirNoCursor(area, texto) {
    area.focus();
    area.setRangeText(texto, area.selectionStart, area.selectionEnd, 'end');
    area.dispatchEvent(new Event('input', { bubbles: true }));
  }

  function pintarEditor(arquivo) {
    const info = TEXTOS[arquivo];
    const ed = S.editores[arquivo];
    if (!ed) { trocar(tela.corpo, falhaCarga(new Error('Este texto não veio do programa.'), () => location.reload())); return; }

    const area = h('textarea', { class: 'editor', rows: 22, spellcheck: 'true', 'aria-label': 'Texto: ' + info.titulo });
    area.value = ed.atual;
    const situacao = h('p', { class: 'situacao', 'aria-live': 'polite' });
    const rascunho = h('div', { class: 'aviso-rascunho', role: 'status' },
      icone('ia'),
      h('span', { text: 'Rascunho da IA, ainda não salvo. Revise antes de salvar.' }),
      botao('Desfazer', {
        classe: 'btn-fantasma', icone: 'desfazer',
        aoClicar: () => {
          ed.atual = ed.antesDaIA;
          ed.antesDaIA = null;
          area.value = ed.atual;
          refletir();
        },
      }));

    const salvar = botao(info.salvar, {
      classe: 'btn-primario',
      aoClicar: async () => {
        if (!ed.atual.trim()) throw new Error('O texto está vazio. Escreva algo ou volte ao modelo original.');
        await api.post('/api/textos', { arquivo, texto: ed.atual });
        ed.salvo = ed.atual;
        ed.antesDaIA = null;
        ed.padrao = false;
        refletir();
        toast('Salvo. Vale já na próxima mensagem.');
      },
    });

    function refletir() {
      const sujo = ed.atual !== ed.salvo;
      situacao.textContent = sujo ? 'Alterações não salvas' : (ed.padrao ? 'Tudo salvo. Este é o modelo original.' : 'Tudo salvo');
      situacao.classList.toggle('sujo', sujo);
      rascunho.hidden = ed.antesDaIA == null;
      salvar.disabled = !sujo;
      pintarSubabas();
    }
    area.addEventListener('input', () => { ed.atual = area.value; refletir(); });

    const pedido = { texto: '' };
    const campoPedido = areaTexto(pedido, 'texto', { rows: 3, placeholder: info.exemplo });
    const pedir = botao('Pedir à IA', {
      classe: 'btn-suave', icone: 'ia',
      aoClicar: async () => {
        if (!pedido.texto.trim()) { campoPedido.focus(); throw new Error('Escreva o que você quer mudar no texto.'); }
        const r = await api.post('/api/ia/melhorar', { arquivo, texto: ed.atual, pedido: pedido.texto.trim() });
        if (!r.texto || !r.texto.trim()) throw new Error('A IA devolveu um texto vazio. Tente pedir de outro jeito.');
        if (ed.antesDaIA == null) ed.antesDaIA = ed.atual;
        ed.atual = r.texto;
        area.value = r.texto;
        pedido.texto = '';
        campoPedido.value = '';
        refletir();
        toast('Rascunho pronto. Revise e salve.');
      },
    });

    const variaveis = (S.variaveis || []).map((v) => h('button', {
      type: 'button', class: 'variavel',
      title: 'Inserir ' + v.chave + ' no texto',
      onclick: () => inserirNoCursor(area, v.chave),
    },
      h('code', { text: v.chave }),
      h('span', { text: v.rotulo + (v.valor ? ': ' + cortarTexto(v.valor, 60) : ' (vazio)') })));

    trocar(tela.corpo,
      h('div', { class: 'editor-grade' },
        h('div', { class: 'editor-principal' },
          h('p', { class: 'descricao', text: info.descricao }),
          arquivo === 'cerebro'
            ? h('div', { class: 'nota' }, icone('conhecimento'),
              h('span', null, 'Tem tabela de preços, catálogo ou regulamento? Envie em ',
                h('a', { href: '#conhecimento', text: 'Conhecimento' }), ' e a IA monta o cérebro para você.'))
            : null,
          rascunho,
          area,
          h('div', { class: 'acoes' },
            salvar,
            botao('Voltar ao modelo original', {
              classe: 'btn-fantasma',
              aoClicar: async () => {
                const sim = await confirmar({
                  titulo: 'Voltar ao modelo original?',
                  texto: 'O texto atual de ' + info.titulo + ' é trocado pelo modelo de fábrica. O que você escreveu aqui se perde.',
                  botao: 'Voltar ao modelo', perigo: true,
                });
                if (!sim) return;
                const r = await api.post('/api/textos/restaurar', { arquivo });
                ed.salvo = ed.atual = r.texto || '';
                ed.padrao = true;
                ed.antesDaIA = null;
                area.value = ed.atual;
                refletir();
                toast('Voltou ao modelo original.');
              },
            }),
            situacao)),
        h('aside', { class: 'editor-lado' },
          h('section', { class: 'cartao pilha' },
            h('h2', { text: 'Peça uma mudança para a IA' }),
            campo('O que você quer mudar?', campoPedido, 'A IA reescreve o texto e mostra um rascunho. Nada é salvo sem você conferir.'),
            h('div', { class: 'acoes' }, pedir, h('span', { class: 'espera-ia', text: 'Pode levar até um minuto.' }))),
          h('section', { class: 'cartao' },
            h('h2', { text: 'Variáveis' }),
            h('p', { class: 'descricao', text: 'Clique para inserir no texto. Na hora de responder, cada uma vira o valor cadastrado em Negócio.' }),
            variaveis.length ? h('div', { class: 'variaveis' }, variaveis) : h('p', { class: 'ajuda', text: 'Nenhuma variável disponível.' })))));
    refletir();
  }

  TELAS.assistente = { montar, sair };
})();
