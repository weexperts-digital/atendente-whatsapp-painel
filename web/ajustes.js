'use strict';
// Aba Ajustes: WhatsApp, avisos, números ignorados, nome de quem atende, ritmo, cérebro e aparência.
// Um salvar só, com aviso de alterações não salvas. Só vai para o programa o que mudou.

(function () {
  let tela = null;

  const EXEMPLO = '5527999990000';
  const TECLAS = [' ', ',', 'Tab', 'Enter'];
  const MODOS = {
    claude: 'Em uso agora: o Claude Code deste computador.',
    openai: 'Em uso agora: o ChatGPT, pela chave da OpenAI.',
    api: 'Em uso agora: a API da Anthropic, como reserva.',
    teste: 'Em uso agora: modo teste. O Claude não foi encontrado neste computador, então as respostas são fixas.',
  };
  const SINO = [
    'M10.268 21a2 2 0 0 0 3.464 0',
    'M3.262 15.326A1 1 0 0 0 4 17h16a1 1 0 0 0 .74-1.673C19.41 13.956 18 12.499 18 8A6 6 0 0 0 6 8c0 4.499-1.411 5.956-2.738 7.326',
  ];
  const BLOQUEIO = ['M12 2a10 10 0 1 0 0 20a10 10 0 0 0 0-20z', 'M4.9 4.9l14.2 14.2'];

  // ---------- Peças pequenas ----------

  // desenho cria um ícone que não está na folha de ícones do index.html.
  function desenho(tracos) {
    const svg = document.createElementNS(SVG, 'svg');
    svg.setAttribute('class', 'ic');
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

  function soDigitos(s) {
    return String(s == null ? '' : s).replace(/\D/g, '');
  }

  function listaLimpa(lista) {
    const v = [];
    for (const x of Array.isArray(lista) ? lista : []) {
      const d = soDigitos(x);
      if (d && !v.includes(d)) v.push(d);
    }
    return v;
  }

  function numero(v, padrao) {
    const n = Number(v);
    return v == null || v === '' || isNaN(n) ? padrao : n;
  }

  function iguais(a, b) {
    return a.length === b.length && a.every((x, i) => x === b[i]);
  }

  function temaDoPainel() {
    if (typeof temaAtual === 'function') return temaAtual();
    return document.documentElement.dataset.theme === 'dark' ? 'dark' : 'light';
  }

  function trocarTema(tema) {
    if (typeof aplicarTema === 'function') { aplicarTema(tema); return; }
    document.documentElement.dataset.theme = tema;
    try { localStorage.setItem('tema', tema); } catch (e) { /* sem armazenamento */ }
  }

  // ---------- O que está salvo e o que mudou ----------

  function salvos() {
    const c = S.config || {};
    const avisos = Array.isArray(c.numeros_humanos) ? c.numeros_humanos : (c.numero_humano ? [c.numero_humano] : []);
    return {
      numeros_humanos: listaLimpa(avisos),
      nunca_responder: listaLimpa(c.nunca_responder),
      atraso_min: numero(c.atraso_min, 3),
      atraso_max: numero(c.atraso_max, 8),
      limite_msgs: numero(c.limite_msgs, 6),
      modelo: c.modelo || '',
    };
  }

  // mudancas devolve só as chaves da configuração que estão diferentes do que está salvo.
  function mudancas() {
    const a = S.ajustes;
    const b = salvos();
    const m = {};
    for (const k of ['numeros_humanos', 'nunca_responder']) {
      if (!iguais(a[k], b[k])) m[k] = a[k].slice();
    }
    for (const k of ['atraso_min', 'atraso_max', 'limite_msgs']) {
      if (numero(a[k], 0) !== b[k]) m[k] = numero(a[k], 0);
    }
    if (a.modelo !== b.modelo) m.modelo = a.modelo;
    return m;
  }

  // ---------- Tela ----------

  function casca(...filhos) {
    return h('div', { class: 'tela-ajustes' },
      h('div', { class: 'topo-tela' }, h('div', { class: 'topo-textos' },
        h('h1', { text: 'Ajustes' }),
        h('p', { class: 'descricao', text: 'A conexão com o WhatsApp e os detalhes de como o atendente trabalha.' }))),
      filhos);
  }

  async function montar(main) {
    const minha = tela = { main };
    trocar(main, casca(carregandoEl()));
    try {
      await carregarConfig();
    } catch (e) {
      if (tela === minha) trocar(main, casca(falhaCarga(e, () => montar(main))));
      return;
    }
    if (tela !== minha) return;
    const rascunho = S.ajustes;
    if (!S.ajustesSujos || !rascunho || !Array.isArray(rascunho.numeros_humanos)) S.ajustes = salvos();
    document.addEventListener('tema', atualizar); // o botão de tema do menu avisa por aqui
    pintar();
  }

  function sair() {
    document.removeEventListener('tema', atualizar);
    tela = null;
  }

  // pintarConexao mostra se a conta do Claude está conectada e o botão para conectar.
  function pintarConexao(ia) {
    if (!tela.conexao) return;
    const chatgpt = !!ia && ia.provedor === 'openai';
    // Com o ChatGPT em uso, a lista de modelos do Claude não vale: o modelo é escolhido na conexão.
    if (tela.campoModelo) tela.campoModelo.hidden = chatgpt;
    if (tela.notaModelo) tela.notaModelo.hidden = !chatgpt;
    const situacao = ia ? (!ia.instalado ? 'falta' : (ia.conectado ? 'ok' : 'fora')) : '';
    const marca = situacao + (chatgpt ? '|chatgpt' : '');
    if (tela.conexao.dataset.marca === marca) return;
    tela.conexao.dataset.marca = marca;
    const textos = {
      ok: chatgpt ? 'ChatGPT em uso, pela chave da OpenAI' : 'Conta do Claude conectada',
      fora: 'IA desconectada', falta: 'Falta o Claude Code neste computador',
    };
    trocar(tela.conexao, situacao ? [
      h('span', { class: 'cx-situacao' }, h('span', { class: 'cx-ponto', 'data-estado': situacao === 'ok' ? 'ok' : 'fora' }), textos[situacao]),
      botao(situacao === 'ok' ? 'Trocar ou ver a conexão' : 'Conectar a IA', {
        classe: situacao === 'ok' ? 'btn-suave' : 'btn-primario', icone: 'ia',
        aoClicar: () => CONEXAO.abrir(),
      }),
    ] : []);
  }

  // atualizar roda a cada 3 segundos: só mexe no que não é formulário.
  function atualizar() {
    if (!tela || !tela.barra) return;
    const e = S.estado || {};
    if (tela.whats) tela.whats.atualizar();
    tela.desconectar.hidden = e.status !== 'conectado';
    tela.modo.textContent = MODOS[e.modo] || '';
    tela.modo.hidden = !MODOS[e.modo];
    pintarConexao(e.ia);
    const tema = temaDoPainel();
    for (const b of tela.temas) b.setAttribute('aria-pressed', b.dataset.tema === tema ? 'true' : 'false');
  }

  function refletir() {
    if (!tela || !tela.barra || !S.ajustes) return;
    const sujo = Object.keys(mudancas()).length > 0;
    S.ajustesSujos = sujo;
    const marca = sujo ? 'sim' : 'nao';
    if (tela.barra.dataset.sujo === marca) return;
    tela.barra.dataset.sujo = marca;
    trocar(tela.situacao, sujo
      ? [h('span', { class: 'aj-ponto', 'aria-hidden': 'true' }), 'Você tem alterações não salvas']
      : [icone('check'), 'Tudo salvo']);
    tela.salvar.disabled = !sujo;
    tela.descartar.hidden = !sujo;
  }

  // bloco monta um cartão: ícone, título, uma frase de explicação e o conteúdo.
  function bloco(figura, titulo, explicacao, ...filhos) {
    return h('section', { class: 'cartao aj-bloco' },
      h('div', { class: 'aj-topo' },
        h('span', { class: 'aj-figura' }, figura),
        h('div', { class: 'aj-titulo' }, h('h2', { text: titulo }), h('p', { class: 'descricao', text: explicacao }))),
      filhos);
  }

  // campoNumeros monta um campo de etiquetas que só aceita telefones de 10 a 15 dígitos.
  function campoNumeros(opc) {
    let carregando = true;
    let tecla = '';
    let recusado = '';
    let entradaEl = null;
    const aviso = h('p', { class: 'aj-recusa', role: 'status', hidden: true });

    function avisar(texto) {
      aviso.hidden = !texto;
      trocar(aviso, texto ? [icone('alerta'), h('span', { text: texto })] : []);
    }

    // O campo de etiquetas apaga o que foi digitado. Aqui o texto recusado volta, para a pessoa corrigir.
    function devolver(texto) {
      queueMicrotask(() => { if (entradaEl && !entradaEl.value) entradaEl.value = texto; });
    }

    function limpar(texto) {
      const t = String(texto == null ? '' : texto).trim();
      if (!t) return '';
      const d = soDigitos(t);
      if (carregando) return d;
      if (d.length >= 10 && d.length <= 15) {
        if (S.ajustes[opc.chave].includes(d)) {
          avisar('O número ' + d + ' já está na lista.');
          return '';
        }
        avisar('');
        return d;
      }
      // Espaço no meio do número: a pessoa ainda está digitando (55 27 99999 0000).
      if (tecla === ' ' && d.length < 10) {
        devolver(t + ' ');
        return '';
      }
      recusado = t;
      avisar('O número "' + cortarTexto(t, 24) + '" não foi aceito. Digite com DDI e DDD, de 10 a 15 dígitos. Exemplo: ' + EXEMPLO + '.');
      devolver(t);
      return '';
    }

    const caixa = chips(S.ajustes[opc.chave], {
      rotulo: opc.rotulo, placeholder: 'Digite um número', teclas: TECLAS, limpar, inputMode: 'numeric',
      aoMudar: (lista) => { S.ajustes[opc.chave] = lista; refletir(); },
    });
    carregando = false;
    entradaEl = caixa.querySelector('.chips-entrada');
    const idAjuda = 'aj-' + opc.chave + '-ajuda';
    if (entradaEl) {
      entradaEl.id = 'aj-' + opc.chave;
      entradaEl.setAttribute('aria-describedby', idAjuda);
      entradaEl.addEventListener('input', () => { if (!aviso.hidden) avisar(''); });
    }
    // Guarda qual tecla fechou a etiqueta. A captura roda antes do campo, a outra depois.
    caixa.addEventListener('keydown', (ev) => { tecla = ev.key; }, true);
    caixa.addEventListener('keydown', () => { tecla = ''; });

    return {
      el: h('div', { class: 'campo' },
        h('label', { htmlFor: 'aj-' + opc.chave, text: opc.rotulo }),
        caixa,
        aviso,
        h('p', { class: 'ajuda', id: idAjuda, text: opc.ajuda })),
      // confirmar fecha o que ainda estava digitado. Devolve o texto recusado, se houver.
      confirmar() {
        recusado = '';
        caixa.valores();
        return recusado;
      },
      focar() { if (entradaEl) entradaEl.focus(); },
    };
  }

  async function salvarTudo() {
    const a = S.ajustes;
    for (const c of [tela.avisos, tela.nunca]) {
      const recusado = c.confirmar();
      if (recusado) {
        c.focar();
        throw new Error('O número "' + cortarTexto(recusado, 24) + '" não foi aceito. Corrija ou apague antes de salvar.');
      }
    }
    const min = numero(a.atraso_min, 0);
    const max = numero(a.atraso_max, 0);
    const limite = numero(a.limite_msgs, 0);
    if (min < 0 || max < min) throw new Error('No ritmo, a espera máxima precisa ser igual ou maior que a mínima.');
    if (limite < 1) throw new Error('O limite de mensagens precisa ser de pelo menos 1.');

    const parte = mudancas();
    if (Object.keys(parte).length) await salvarConfig(parte);
    S.ajustes = salvos();
    S.ajustesSujos = false;
    if (tela) pintar();
    toast('Ajustes salvos.');
  }

  function pintar() {
    const a = S.ajustes;
    tela.whats = typeof criarBlocoWhats === 'function' ? criarBlocoWhats() : null;
    tela.modo = h('p', { class: 'ajuda' });
    tela.conexao = h('div', { class: 'cx-linha', role: 'status' });
    tela.notaModelo = h('p', { class: 'ajuda', hidden: true, text: 'O modelo do ChatGPT é escolhido em Trocar ou ver a conexão.' });
    tela.desconectar = h('div', { class: 'acoes' }, botao('Desconectar este aparelho', {
      classe: 'btn-perigo-suave', icone: 'celular',
      aoClicar: async () => {
        const sim = await confirmar({
          titulo: 'Desconectar o WhatsApp?',
          texto: 'O atendente para de responder até você ler um novo código QR com o celular do negócio.',
          botao: 'Desconectar', perigo: true,
        });
        if (!sim) return;
        await api.post('/api/whatsapp/desconectar', {});
        toast('Aparelho desconectado. Um novo código QR aparece em instantes.');
        await pulsar();
      },
    }));

    tela.avisos = campoNumeros({
      chave: 'numeros_humanos', rotulo: 'Números que recebem os avisos',
      ajuda: 'Digite o número e aperte Espaço, Vírgula, Tab ou Enter. Com DDI e DDD, exemplo ' + EXEMPLO + '. Por esses números você também manda /lista e /retomar.',
    });
    tela.nunca = campoNumeros({
      chave: 'nunca_responder', rotulo: 'Nunca responder a estes números',
      ajuda: 'Digite o número e aperte Espaço, Vírgula, Tab ou Enter. Com DDI e DDD, exemplo ' + EXEMPLO + '.',
    });

    const segundos = (chaveNum, attrs) => entrada(a, chaveNum, Object.assign({ type: 'number', inputMode: 'numeric' }, attrs), refletir);

    const modelos = S.modelos || [];
    const campoModelo = modelos.length
      ? h('div', { class: 'opcoes', role: 'radiogroup', 'aria-label': 'Modelo do Claude' }, modelos.map((m) => {
        const r = h('input', { type: 'radio', name: 'aj-modelo', value: m.id, checked: a.modelo === m.id });
        r.addEventListener('change', () => { a.modelo = m.id; refletir(); });
        return h('label', { class: 'opcao' }, r, h('div', null, h('strong', { text: m.nome || m.id }), h('span', { text: m.dica || '' })));
      }))
      : campo('Modelo', entrada(a, 'modelo', { autocomplete: 'off' }, refletir));

    tela.temas = [['light', 'sol', 'Claro'], ['dark', 'lua', 'Escuro']].map(([tema, figura, rotulo]) => h('button', {
      type: 'button', 'data-tema': tema, 'aria-pressed': 'false',
      onclick: () => { trocarTema(tema); atualizar(); },
    }, icone(figura), rotulo));

    tela.situacao = h('p', { class: 'aj-situacao', role: 'status' });
    tela.descartar = botao('Descartar', {
      classe: 'btn-fantasma',
      aoClicar: () => { S.ajustes = salvos(); S.ajustesSujos = false; pintar(); },
    });
    tela.salvar = botao('Salvar ajustes', { classe: 'btn-primario', icone: 'check', aoClicar: salvarTudo });
    tela.barra = h('div', { class: 'aj-salvar' }, tela.situacao, h('div', { class: 'acoes' }, tela.salvar, tela.descartar));

    trocar(tela.main, casca(
      h('div', { class: 'aj-colunas' },
        h('div', { class: 'aj-coluna' },
          bloco(icone('celular'), 'WhatsApp',
            'O atendente responde pelas conversas privadas do número conectado. Grupos, status e canais ficam de fora.',
            tela.whats ? tela.whats.el : null,
            tela.desconectar),
          bloco(desenho(SINO), 'Avisos',
            'Quando um cliente esquenta, pede uma pessoa ou marca um horário, o assistente avisa estes números pelo WhatsApp.',
            tela.avisos.el),
          bloco(desenho(BLOQUEIO), 'Números ignorados',
            'O assistente fica quieto com estes números. Bom para família, amigos e fornecedores.',
            tela.nunca.el),
          bloco(icone('pessoa'), 'Sua conta',
            'Você entrou como ' + (SESSAO.usuario ? SESSAO.usuario.nome + ' (' + SESSAO.usuario.email + ')' : '') + '. O seu nome fica no histórico quando você assume uma conversa. O cliente não vê.',
            h('div', { class: 'acoes' },
              botao('Trocar senha', { icone: 'cadeado', aoClicar: () => ACESSO.trocarSenha() }),
              botao('Sair do painel', { classe: 'btn-fantasma', icone: 'sair', aoClicar: () => ACESSO.sair() })))),
        h('div', { class: 'aj-coluna' },
          bloco(icone('ia'), 'Cérebro',
            'Quem pensa pelo assistente: o Claude, com o login da sua conta, ou o ChatGPT, com uma chave da OpenAI. Modelos mais rápidos respondem em menos tempo. Os mais capazes entendem melhor conversas difíceis.',
            tela.conexao,
            (tela.campoModelo = campoModelo),
            tela.notaModelo,
            tela.modo),
          bloco(icone('relogio'), 'Ritmo das respostas',
            'O assistente espera um pouco antes de responder, mostrando "digitando". Fica mais natural.',
            h('div', { class: 'aj-dupla' },
              campo('Espera mínima, em segundos', segundos('atraso_min', { min: 0, max: 120 })),
              campo('Espera máxima, em segundos', segundos('atraso_max', { min: 0, max: 120 })),
              h('div', { class: 'largo' },
                campo('Limite de mensagens por contato a cada 10 minutos', segundos('limite_msgs', { min: 1, max: 60 }),
                  'Protege contra quem manda mensagem demais.')))),
          bloco(icone('sol'), 'Aparência',
            'O tema claro costuma ficar melhor em projetor e em salas iluminadas.',
            h('div', { class: 'segmentado aj-tema', role: 'group', 'aria-label': 'Tema do painel' }, tela.temas)),
          bloco(icone('desfazer'), 'Configuração inicial',
            'Passe de novo pelos passos do começo: IA, negócio, assistente, avisos, WhatsApp e ligar.',
            h('div', { class: 'acoes' }, botao('Refazer a configuração inicial', {
              icone: 'desfazer',
              aoClicar: () => { if (typeof abrirInicio === 'function') return abrirInicio(); return undefined; },
            }))))),
      // Equipe e registro de acessos: só o super administrador recebe estes blocos.
      h('div', { class: 'aj-coluna aj-acessos' }, typeof blocosDeAcesso === 'function' ? blocosDeAcesso() : null),
      tela.barra));
    refletir();
    atualizar();
  }

  TELAS.ajustes = { montar, atualizar, sair };
})();
