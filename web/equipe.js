'use strict';
// Blocos de Ajustes que só o super administrador vê: a equipe (contas do painel) e
// o registro de acessos (quem entrou e quem fez o quê).

const blocosDeAcesso = (function () {
  const LETRAS = 'abcdefghjkmnpqrstuvwxyz23456789'; // sem letras e números que se confundem

  function senhaProvisoria() {
    const sorteio = new Uint32Array(12);
    crypto.getRandomValues(sorteio);
    let s = '';
    for (let i = 0; i < sorteio.length; i++) {
      if (i && i % 4 === 0) s += '-';
      s += LETRAS[sorteio[i] % LETRAS.length];
    }
    return s;
  }

  function quando(iso) {
    if (!iso || iso.startsWith('0001')) return 'Nunca entrou';
    const d = new Date(iso);
    return d.toLocaleDateString('pt-BR') + ' às ' + d.toLocaleTimeString('pt-BR', { hour: '2-digit', minute: '2-digit' });
  }

  async function copiarTexto(texto, oQue) {
    try {
      await navigator.clipboard.writeText(texto);
      toast(oQue + ' copiada.');
    } catch (e) {
      toast('Não consegui copiar. Selecione o texto e copie.', 'erro');
    }
  }

  function cabecalho(figura, titulo, explicacao, acao) {
    return h('div', { class: 'aj-topo' },
      h('span', { class: 'aj-figura' }, icone(figura)),
      h('div', { class: 'aj-titulo' }, h('h2', { text: titulo }), h('p', { class: 'descricao', text: explicacao })),
      acao ? h('div', { class: 'eq-acao-topo' }, acao) : null);
  }

  // campoProvisoria: senha provisória à vista, já sorteada, com sortear outra e copiar.
  function campoProvisoria() {
    const c = ACESSO.campoDe({
      rotulo: 'Senha provisória', nome: 'provisoria', tipo: 'text', max: 128,
      ajuda: 'Passe esta senha para a pessoa. Na primeira entrada ela cria a própria senha, que só ela vai saber.',
    });
    c.input.value = senhaProvisoria();
    c.input.classList.add('eq-senha');
    const linha = c.input.parentNode;
    linha.appendChild(h('button', { type: 'button', class: 'btn btn-suave', onclick: () => { c.input.value = senhaProvisoria(); c.erro(''); } }, h('span', { text: 'Sortear outra' })));
    linha.appendChild(h('button', { type: 'button', class: 'btn btn-suave', onclick: () => copiarTexto(c.input.value, 'Senha provisória') }, h('span', { text: 'Copiar' })));
    return c;
  }

  // ---------- Equipe ----------

  function blocoEquipe() {
    const lista = h('div', { class: 'eq-lista' }, carregandoEl('Carregando a equipe...'));
    let papeis = [];

    async function carregar() {
      try {
        const r = await api.get('/api/usuarios');
        papeis = r.papeis || [];
        pintar(r.usuarios || []);
      } catch (e) {
        trocar(lista, falhaCarga(e, carregar));
      }
    }

    async function alterar(u, parte, aviso) {
      await api.post('/api/usuarios/alterar', Object.assign({ id: u.id }, parte));
      if (aviso) toast(aviso);
      await carregar();
    }

    function novaSenha(u) {
      const campos = { senha: campoProvisoria() };
      let j = null;
      const form = ACESSO.formulario({
        campos, botao: 'Salvar senha provisória',
        enviar: (v) => ACESSO.chamar('/api/usuarios/alterar', { id: u.id, senha: v.senha }),
        deuCerto: async () => {
          j.fechar(true);
          toast('Senha provisória salva. As sessões abertas de ' + u.nome + ' foram encerradas.');
          await carregar();
        },
      });
      j = janela({
        titulo: 'Senha provisória para ' + u.nome,
        descricao: 'Use quando a pessoa esquecer a senha. Ela entra com esta e cria uma nova em seguida.',
        corpo: form, classe: 'ac-janela',
      });
    }

    function linha(u) {
      const fixo = u.papel === 'superadmin' || u.voce;
      const estado = { papel: u.papel };
      const acesso = fixo
        ? h('span', { class: 'eq-papel', text: u.papel_nome })
        : seletor(estado, 'papel', papeis.map((p) => ({ valor: p.id, texto: p.nome })), () => {
          alterar(u, { papel: estado.papel }, 'Acesso de ' + u.nome + ' alterado.').catch((e) => { toast(e.message, 'erro'); carregar(); });
        });
      if (!fixo) acesso.setAttribute('aria-label', 'Tipo de acesso de ' + u.nome);
      const acoes = fixo ? null : h('div', { class: 'eq-acoes' },
        botao('Nova senha', { classe: 'btn-fantasma', icone: 'cadeado', dica: 'Criar uma senha provisória nova', aoClicar: () => novaSenha(u) }),
        botao(u.ativo ? 'Desativar' : 'Reativar', {
          classe: 'btn-fantasma', icone: u.ativo ? 'fechar' : 'check',
          aoClicar: async () => {
            if (u.ativo) {
              const ok = await confirmar({ titulo: 'Desativar a conta de ' + u.nome + '?', texto: 'A pessoa sai do painel na hora e não consegue entrar até a conta ser reativada. O histórico dela continua guardado.', botao: 'Desativar', perigo: true });
              if (!ok) return;
            }
            await alterar(u, { ativo: !u.ativo }, u.ativo ? 'Conta desativada.' : 'Conta reativada.');
          },
        }),
        botao('Remover', {
          classe: 'btn-fantasma eq-remover', icone: 'lixo',
          aoClicar: async () => {
            const ok = await confirmar({ titulo: 'Remover a conta de ' + u.nome + '?', texto: 'A conta é apagada e não dá para desfazer. O que a pessoa fez continua no registro de acessos.', botao: 'Remover conta', perigo: true });
            if (!ok) return;
            await api.post('/api/usuarios/excluir', { id: u.id });
            toast('Conta removida.');
            await carregar();
          },
        }));
      const selos = [];
      if (u.voce) selos.push(h('span', { class: 'eq-selo', text: 'Você' }));
      if (!u.ativo) selos.push(h('span', { class: 'eq-selo eq-selo-off', text: 'Desativada' }));
      else if (u.trocar_senha) selos.push(h('span', { class: 'eq-selo eq-selo-espera', text: 'Ainda não criou a senha' }));
      return h('div', { class: 'eq-linha' + (u.ativo ? '' : ' eq-inativa') },
        h('span', { class: 'conta-avatar', 'aria-hidden': 'true', text: ACESSO.iniciais(u.nome) }),
        h('div', { class: 'eq-quem' },
          h('p', { class: 'eq-nome' }, h('span', { text: u.nome }), selos),
          h('p', { class: 'eq-email', text: u.email }),
          h('p', { class: 'eq-ultimo', text: 'Último acesso: ' + quando(u.ultimo_acesso) })),
        h('div', { class: 'eq-acesso' }, acesso),
        acoes);
    }

    function pintar(usuarios) {
      trocar(lista, usuarios.map(linha));
    }

    function adicionar() {
      const campos = {
        nome: ACESSO.campoDe({ rotulo: 'Nome da pessoa', nome: 'nome-novo', max: 60 }),
        email: ACESSO.campoDe({ rotulo: 'E-mail', nome: 'email-novo', tipo: 'email', max: 254, ajuda: 'É com ele que a pessoa entra no painel.' }),
        senha: campoProvisoria(),
      };
      const escolha = { papel: 'atendente' };
      const tipos = h('fieldset', { class: 'campo' }, h('legend', { class: 'rotulo', text: 'Tipo de acesso' }),
        h('div', { class: 'opcoes', role: 'radiogroup' }, papeis.slice().reverse().map((p) => {
          const r = h('input', { type: 'radio', name: 'eq-papel', value: p.id, checked: p.id === escolha.papel });
          r.addEventListener('change', () => { escolha.papel = p.id; });
          return h('label', { class: 'opcao' }, r, h('div', null, h('strong', { text: p.nome }), h('span', { text: p.dica || '' })));
        })));
      let j = null;
      const form = ACESSO.formulario({
        campos, botao: 'Criar conta',
        enviar: (v) => ACESSO.chamar('/api/usuarios', { nome: v.nome, email: v.email, senha: v.senha, papel: escolha.papel }),
        deuCerto: async (d) => {
          const provisoria = campos.senha.input.value;
          const dados = h('dl', { class: 'eq-criada' },
            h('dt', { text: 'E-mail' }), h('dd', { text: d.usuario.email }),
            h('dt', { text: 'Senha provisória' }), h('dd', null, h('code', { text: provisoria })));
          trocar(j.corpo,
            h('p', { class: 'eq-pronto' }, icone('check'), h('span', { text: 'Conta criada. Passe estes dados para ' + d.usuario.nome + '.' })),
            dados,
            h('p', { class: 'ajuda', text: 'Na primeira entrada a pessoa troca a senha provisória por uma que só ela conhece.' }),
            h('div', { class: 'acoes' },
              botao('Copiar e-mail e senha', { classe: 'btn-primario', aoClicar: () => copiarTexto('E-mail: ' + d.usuario.email + '\nSenha provisória: ' + provisoria, 'Combinação') }),
              botao('Fechar', { aoClicar: () => j.fechar(true) })));
          await carregar();
        },
      });
      form.insertBefore(tipos, campos.senha.el);
      j = janela({
        titulo: 'Adicionar pessoa', descricao: 'Cada pessoa entra com o próprio e-mail e senha. Assim o histórico mostra quem fez o quê.',
        corpo: form, classe: 'ac-janela',
      });
      campos.nome.input.focus();
    }

    carregar();
    return h('section', { class: 'cartao aj-bloco eq-bloco' },
      cabecalho('escudo', 'Equipe e acessos',
        'Quem pode entrar neste painel. O cliente nunca vê o nome de quem atende: ele fica só no histórico daqui.',
        botao('Adicionar pessoa', { classe: 'btn-primario', icone: 'mais', aoClicar: adicionar })),
      lista);
  }

  // ---------- Registro de acessos ----------

  function blocoRegistro() {
    const corpo = h('tbody');
    const tabela = h('table', { class: 'eq-tabela' },
      h('thead', null, h('tr', null,
        h('th', { scope: 'col', text: 'Quando' }), h('th', { scope: 'col', text: 'Quem' }),
        h('th', { scope: 'col', text: 'O que fez' }), h('th', { scope: 'col', text: 'Detalhe' }))),
      corpo);
    const caixa = h('div', { class: 'eq-registro' }, carregandoEl('Carregando o registro...'));
    let ultimo = 0;
    const mais = botao('Ver mais antigos', { aoClicar: () => carregar(false) });
    const pe = h('div', { class: 'acoes', hidden: true }, mais);

    async function carregar(doComeco) {
      try {
        const r = await api.get('/api/registro?limite=40' + (doComeco || !ultimo ? '' : '&antes=' + ultimo));
        const linhas = r.registros || [];
        if (doComeco) corpo.replaceChildren();
        for (const l of linhas) {
          corpo.appendChild(h('tr', { class: l.acao === 'senha_errada' || l.acao === 'entrada_bloqueada' ? 'eq-alerta' : null },
            h('td', { class: 'eq-quando', text: quando(l.quando) }),
            h('td', { text: l.usuario || 'Sem login' }),
            h('td', { text: l.texto }),
            h('td', { class: 'eq-detalhe', text: l.detalhe || '' })));
          ultimo = l.n;
        }
        if (doComeco) trocar(caixa, corpo.children.length ? tabela : h('p', { class: 'ajuda', text: 'Nada registrado ainda.' }));
        pe.hidden = !r.tem_mais;
      } catch (e) {
        if (doComeco) trocar(caixa, falhaCarga(e, () => carregar(true)));
        else throw e;
      }
    }

    carregar(true);
    return h('section', { class: 'cartao aj-bloco eq-bloco' },
      cabecalho('registro', 'Registro de acessos',
        'Quem entrou no painel e o que cada pessoa fez: entradas, senhas erradas, atendimentos, ajustes salvos e planilhas baixadas.',
        botao('Atualizar', { icone: 'desfazer', aoClicar: () => { ultimo = 0; return carregar(true); } })),
      caixa, pe);
  }

  return function () {
    if (!ACESSO.pode('superadmin')) return [];
    return [blocoEquipe(), blocoRegistro()];
  };
})();
