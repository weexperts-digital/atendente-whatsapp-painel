'use strict';

// Conexão da IA: escolher quem responde (Claude ou ChatGPT) e conectar, sem terminal.
//   Claude: login por botão. A senha é digitada só na página do Claude.
//   ChatGPT: chave da OpenAI. A chave vai para o cofre de senhas do sistema e nunca
//   volta para esta página (só as 4 últimas letras, para a pessoa reconhecer).
const CONEXAO = (() => {
  const PLANOS = { pro: 'Pro', max: 'Max', team: 'Equipe', enterprise: 'Empresa' };
  const PASSOS_LOGIN = [
    'Clique em Conectar o Claude. O navegador abre a página do Claude.',
    'Entre na sua conta e clique em Autorizar.',
    'Volte para cá. Esta tela avisa quando terminar.',
  ];
  const PAGINA_DE_CHAVES = 'https://platform.openai.com/api-keys';
  const MOTIVOS = {
    openai_chave: 'A OpenAI não aceitou a chave guardada. Cole uma chave nova.',
    openai_credito: 'A conta da OpenAI está sem crédito. Coloque crédito no site da OpenAI ou volte a usar o Claude.',
    openai_modelo: 'O modelo escolhido não existe nesta conta da OpenAI. Escolha outro modelo.',
  };

  const passos = (lista) => h('ol', { class: 'passos-qr cx-passos' }, lista.map((t) => h('li', null, typeof t === 'string' ? h('span', { text: t }) : t)));
  const estadoEl = (classe, figura, titulo, texto) => h('div', { class: 'cx-estado ' + classe, role: 'status' }, figura,
    h('div', null, h('strong', { text: titulo }), texto ? h('p', { text: texto }) : null));

  // painel monta a conexão dentro de `onde` e fica conferindo.
  //   opc.aoMudar(estado): chamado a cada mudança (o início guiado usa para trocar o botão).
  // Devolve { parar, estado() }.
  function painel(onde, opc) {
    opc = opc || {};
    let vivo = true;
    let estado = null;
    let marca = '';
    let relogio = null;
    let aba = null; // 'claude' ou 'openai': o que a pessoa está olhando
    let trocandoChave = false;
    let modelos = null; // modelos da conta da OpenAI, buscados uma vez
    const rascunho = { chave: '', modelo: '' };
    const abas = h('div', { class: 'segmentado cx-abas', role: 'group', 'aria-label': 'Quem responde os clientes' });
    const corpo = h('div', { class: 'cx-corpo' }, carregandoEl('Conferindo a conexão da IA...'));
    onde.appendChild(h('div', { class: 'cx' }, abas, corpo));

    function agendar() {
      clearTimeout(relogio);
      if (!vivo) return;
      relogio = setTimeout(async () => { await conferir(false); agendar(); }, estado && estado.em_andamento ? 1500 : 6000);
    }

    async function conferir(agora) {
      try {
        const r = await api.get('/api/ia/conexao' + (agora ? '?agora=1' : ''));
        if (vivo) receber(r);
      } catch (e) {
        if (vivo && !estado) trocar(corpo, h('p', { class: 'ac-aviso', role: 'alert', text: e.message }));
      }
    }

    // receber guarda o estado novo e só redesenha quando algo mudou, para não apagar
    // o que a pessoa está digitando.
    function receber(r, forcar, jaAvisou) {
      const antes = estado;
      estado = r;
      if (Array.isArray(r.modelos)) modelos = r.modelos;
      if (!aba) aba = r.provedor === 'openai' ? 'openai' : 'claude';
      const o = r.openai || {};
      const nova = JSON.stringify([r.provedor, r.conectado, r.motivo, r.instalado, r.claude_conectado, r.conta, r.plano,
        r.em_andamento, r.endereco, r.erro, o.tem_chave, o.final, o.modelo, o.origem]);
      if (antes && !antes.conectado && r.conectado) {
        if (!jaAvisou) toast('IA conectada. O assistente já pode responder.');
        if (typeof pulsar === 'function') pulsar();
      }
      if (opc.aoMudar) opc.aoMudar(r);
      if (nova === marca && !forcar) return;
      marca = nova;
      pintar();
    }

    function mostrarAba(qual) {
      if (aba === qual) return;
      aba = qual;
      trocandoChave = false;
      pintar();
      if (qual === 'openai') buscarModelos();
    }

    function pintar() {
      const r = estado;
      trocar(abas, [['claude', 'Claude'], ['openai', 'ChatGPT']].map(([id, nome]) => h('button', {
        type: 'button', 'aria-pressed': aba === id ? 'true' : 'false', onclick: () => mostrarAba(id),
      }, nome, r.provedor === id ? h('span', { class: 'cx-em-uso', text: 'em uso' }) : null)));
      trocar(corpo, aba === 'openai' ? pintarOpenAI(r) : pintarClaude(r));
    }

    // ---------- Claude ----------

    async function conectar() {
      receber(await api.post('/api/ia/conectar'));
      agendar();
    }

    async function cancelar() {
      await api.post('/api/ia/conectar/cancelar');
      await conferir(false);
    }

    async function usar(provedor) {
      receber(await api.post('/api/ia/provedor', { provedor }), true, true);
      toast(provedor === 'openai' ? 'O assistente passou a usar o ChatGPT.' : 'O assistente voltou a usar o Claude.');
      if (typeof pulsar === 'function') pulsar();
    }

    function pintarClaude(r) {
      const emUso = r.provedor === 'claude';
      if (!r.instalado) {
        return [
          estadoEl('cx-falta', icone('alerta'), 'Falta o Claude Code neste computador', 'É ele que faz o assistente pensar pelo Claude.'),
          passos([
            'Abra o Claude Code, o mesmo aplicativo que instalou este atendente.',
            'Peça para ele: instale o Claude do terminal e ligue o atendente de novo.',
            'Volte aqui e clique em Conferir de novo.',
          ]),
          h('div', { class: 'acoes' }, botao('Conferir de novo', { classe: 'btn-primario', aoClicar: () => conferir(true) })),
        ];
      }

      if (r.em_andamento) {
        const codigo = { valor: '' };
        const campoCodigo = entrada(codigo, 'valor', { autocomplete: 'off', spellcheck: 'false', placeholder: 'Cole o código aqui' });
        const enviar = botao('Enviar código', {
          aoClicar: async () => {
            if (!codigo.valor.trim()) throw new Error('Cole o código que a página do Claude mostrou.');
            await api.post('/api/ia/conectar/codigo', { codigo: codigo.valor.trim() });
            toast('Código enviado. Espere um instante.');
          },
        });
        return [
          estadoEl('cx-espera', h('span', { class: 'rodando' }), 'Esperando você autorizar no navegador',
            'A página do Claude abriu em outra aba ou janela. Entre na sua conta e clique em Autorizar.'),
          h('details', { class: 'cx-reserva' },
            h('summary', { text: 'O navegador não abriu?' }),
            h('div', { class: 'pilha' },
              h('p', { class: 'ajuda', text: 'Abra a página por este botão e entre na sua conta. No final ela mostra um código. Copie e cole no campo abaixo.' }),
              r.endereco
                ? h('div', null, h('a', { class: 'btn btn-suave', href: r.endereco, target: '_blank', rel: 'noopener noreferrer' }, icone('ia'), h('span', { text: 'Abrir a página do Claude' })))
                : null,
              h('form', { class: 'cx-codigo', novalidate: true, onsubmit: (ev) => { ev.preventDefault(); enviar.click(); } },
                campo('Código que a página mostrou', campoCodigo), enviar))),
          h('div', { class: 'acoes' }, botao('Cancelar', { classe: 'btn-fantasma', aoClicar: cancelar })),
        ];
      }

      if (r.claude_conectado) {
        const plano = r.plano ? (PLANOS[String(r.plano).toLowerCase()] || r.plano) : '';
        const conta = r.conta ? 'Conta: ' + r.conta + (plano ? ', plano ' + plano : '') + '.' : '';
        return [
          emUso
            ? estadoEl('cx-ok', icone('check'), 'O Claude está respondendo os clientes', conta)
            : estadoEl('cx-parado', icone('info'), 'O Claude está conectado, mas quem responde agora é o ChatGPT', conta),
          h('div', { class: 'acoes' },
            emUso ? null : botao('Usar o Claude', { classe: 'btn-primario', icone: 'ia', aoClicar: () => usar('claude') }),
            botao('Entrar com outra conta', { classe: 'btn-fantasma', aoClicar: conectar })),
        ];
      }

      return [
        r.erro ? h('p', { class: 'ac-aviso', role: 'alert', text: r.erro }) : null,
        passos(PASSOS_LOGIN),
        h('div', { class: 'acoes' }, botao('Conectar o Claude', { classe: 'btn-primario btn-grande', icone: 'ia', aoClicar: conectar })),
        h('p', { class: 'ajuda', text: 'Use uma conta do Claude com plano pago (Pro, Max ou de equipe). A senha é digitada só na página do Claude. O painel nunca vê a sua senha.' }),
      ];
    }

    // ---------- ChatGPT ----------

    async function buscarModelos() {
      if (modelos || !estado || !(estado.openai || {}).tem_chave) return;
      try {
        const r = await api.get('/api/ia/openai/modelos');
        modelos = Array.isArray(r.modelos) ? r.modelos : [];
        if (vivo && aba === 'openai' && !trocandoChave) pintar();
      } catch (e) {
        modelos = [];
      }
    }

    async function salvarChave() {
      const chave = rascunho.chave.trim();
      if (!chave) throw new Error('Cole a chave da OpenAI.');
      const r = await api.post('/api/ia/openai', { chave });
      rascunho.chave = '';
      trocandoChave = false;
      toast('Chave guardada. O assistente passou a usar o ChatGPT.');
      receber(r, true, true);
      if (typeof pulsar === 'function') pulsar();
    }

    async function trocarModelo(modelo) {
      if (!modelo || modelo === (estado.openai || {}).modelo) return;
      try {
        receber(await api.post('/api/ia/openai', { modelo }), true, true);
        toast('Modelo do ChatGPT trocado.');
        if (typeof pulsar === 'function') pulsar();
      } catch (e) {
        toast(e.message, 'erro');
        pintar();
      }
    }

    async function removerChave() {
      const sim = typeof confirmar !== 'function' || await confirmar({
        titulo: 'Remover a chave da OpenAI?',
        texto: 'A chave sai do cofre deste computador e o assistente volta a usar o Claude. Para usar o ChatGPT de novo, você cola a chave outra vez.',
        botao: 'Remover a chave', perigo: true,
      });
      if (!sim) return;
      const r = await api.post('/api/ia/openai/remover');
      modelos = null;
      toast('Chave removida. O assistente voltou a usar o Claude.');
      receber(r, true, true);
      if (typeof pulsar === 'function') pulsar();
    }

    function formularioDaChave(primeira) {
      const campoChave = entrada(rascunho, 'chave', {
        type: 'password', autocomplete: 'off', spellcheck: 'false', placeholder: 'sk-...',
      });
      const salvar = botao(primeira ? 'Salvar e usar o ChatGPT' : 'Salvar a chave nova', { classe: 'btn-primario', aoClicar: salvarChave });
      return h('form', { class: 'cx-chave', novalidate: true, onsubmit: (ev) => { ev.preventDefault(); salvar.click(); } },
        campo('Chave da OpenAI', campoChave),
        h('div', { class: 'acoes' }, salvar,
          primeira ? null : botao('Cancelar', { classe: 'btn-fantasma', aoClicar: () => { trocandoChave = false; rascunho.chave = ''; pintar(); } })));
    }

    function pintarOpenAI(r) {
      const o = r.openai || {};
      const emUso = r.provedor === 'openai';
      const nota = h('p', { class: 'ajuda', text: 'A OpenAI cobra por uso, separado da assinatura do ChatGPT. A chave fica guardada no cofre de senhas deste computador, não em arquivo. Com o ChatGPT em uso, as conversas passam pela OpenAI para gerar as respostas.' });

      if (!o.tem_chave) {
        return [
          passos([
            h('span', null, 'Abra a ',
              h('a', { href: PAGINA_DE_CHAVES, target: '_blank', rel: 'noopener noreferrer', text: 'página de chaves da OpenAI' }),
              ' e entre na sua conta.'),
            'Crie uma chave nova e copie.',
            'Cole a chave aqui embaixo e clique em Salvar.',
          ]),
          formularioDaChave(true),
          nota,
        ];
      }

      const lista = (modelos || []).slice();
      const escolha = { modelo: o.modelo || '' };
      let campoModelo;
      if (lista.length) {
        if (o.modelo && !lista.includes(o.modelo)) lista.unshift(o.modelo);
        campoModelo = campo('Modelo do ChatGPT', seletor(escolha, 'modelo', lista.map((m) => ({ valor: m, texto: m })), () => trocarModelo(escolha.modelo)),
          'Os modelos com "mini" no nome respondem mais rápido e custam menos.');
      } else if (modelos) {
        // A conta não deixou listar os modelos (chave restrita): a pessoa escreve o nome.
        const trocar_ = botao('Trocar o modelo', { aoClicar: () => trocarModelo(escolha.modelo.trim()) });
        campoModelo = h('form', { class: 'cx-codigo', novalidate: true, onsubmit: (ev) => { ev.preventDefault(); trocar_.click(); } },
          campo('Modelo do ChatGPT', entrada(escolha, 'modelo', { autocomplete: 'off', spellcheck: 'false', placeholder: 'Ex.: gpt-5-mini' }),
            'Escreva o nome do modelo como aparece no site da OpenAI.'), trocar_);
      } else {
        campoModelo = h('p', { class: 'ajuda', text: 'Modelo em uso: ' + (o.modelo || 'padrão') + '.' });
      }
      const daChave = 'Chave terminada em ' + (o.final || '****') + (o.origem === 'ambiente' ? ', vinda da variável OPENAI_API_KEY.' : ', guardada no cofre deste computador.');

      return [
        emUso && r.motivo && MOTIVOS[r.motivo] ? h('p', { class: 'ac-aviso', role: 'alert', text: MOTIVOS[r.motivo] }) : null,
        emUso
          ? estadoEl('cx-ok', icone('check'), 'O ChatGPT está respondendo os clientes', daChave)
          : estadoEl('cx-parado', icone('info'), 'A chave está guardada, mas quem responde agora é o Claude', daChave),
        trocandoChave ? formularioDaChave(false) : campoModelo,
        trocandoChave ? null : h('div', { class: 'acoes' },
          emUso ? null : botao('Usar o ChatGPT', { classe: 'btn-primario', icone: 'ia', aoClicar: () => usar('openai') }),
          botao('Trocar a chave', { aoClicar: () => { trocandoChave = true; pintar(); } }),
          o.origem === 'ambiente' ? null : botao('Remover a chave', { classe: 'btn-fantasma', icone: 'lixo', aoClicar: removerChave })),
        nota,
      ];
    }

    conferir(true).then(() => { agendar(); if (aba === 'openai') buscarModelos(); });
    return {
      parar() { vivo = false; clearTimeout(relogio); },
      estado: () => estado,
    };
  }

  let aberta = null;

  // abrir mostra a janela "Conectar a IA". Só administradores conectam.
  function abrir() {
    if (aberta) return;
    if (typeof ACESSO !== 'undefined' && !ACESSO.pode('admin')) {
      toast('A IA está desconectada. Avise um administrador do painel.', 'erro');
      return;
    }
    const corpo = h('div');
    let p = null;
    aberta = janela({
      titulo: 'Conectar a IA',
      descricao: 'Escolha quem responde os seus clientes: o Claude, com o login da sua conta, ou o ChatGPT, com uma chave da OpenAI.',
      corpo, classe: 'cx-janela', fecharFora: true,
      aoFechar: () => { if (p) p.parar(); aberta = null; },
    });
    p = painel(corpo);
  }

  return { abrir, painel };
})();
