'use strict';
// Entrada no painel: criar a conta do super administrador, entrar, criar a própria
// senha, recuperar o acesso e sair. Enquanto ninguém entra, o painel fica escondido.

const ACESSO = (function () {
  const PAPEIS = { superadmin: 3, admin: 2, atendente: 1 };
  const FORCAS = ['', 'Fraca', 'Razoável', 'Boa', 'Forte'];

  let raiz = null;        // a tela de entrada, quando está aberta
  let aoEntrar = null;    // avisa a partida do painel que alguém entrou
  let jaEntrou = false;   // o painel já foi montado nesta página
  let minSenha = 8;

  // chamar fala direto com o programa, sem passar pelo aviso de sessão caída.
  async function chamar(url, corpo) {
    let resp;
    try {
      resp = await fetch(url, corpo === undefined ? { cache: 'no-store' } : {
        method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(corpo),
      });
    } catch (e) {
      throw new Error('Não consegui falar com o atendente. Confira se ele está ligado neste computador.');
    }
    let dados = null;
    try { dados = await resp.json(); } catch (e) { /* resposta sem JSON */ }
    return { codigo: resp.status, dados: dados || {} };
  }

  // forca dá uma nota de 1 a 4 para a senha. É só um guia: quem decide é o programa.
  function forca(s) {
    if (!s) return 0;
    const tipos = [/[a-z]/, /[A-Z]/, /\d/, /[^A-Za-z0-9]/].filter((r) => r.test(s)).length;
    let nota = 1;
    if (s.length >= minSenha && tipos >= 2) nota = 2;
    if (s.length >= 10 && tipos >= 3) nota = 3;
    if (s.length >= 14 && tipos >= 3) nota = 4;
    if (s.length >= 18 && tipos >= 2) nota = 4;
    if (/^(.)\1+$/.test(s) || /^(1234|abcd|qwer|senha|password)/i.test(s)) nota = 1;
    return nota;
  }

  // ---------- Peças do formulário ----------

  let contador = 0;

  // campoDe monta rótulo, campo e lugar do erro. Devolve { el, input, erro(texto) }.
  function campoDe(opc) {
    const id = 'ac-' + (++contador);
    const input = h('input', {
      id, type: opc.tipo || 'text', name: opc.nome, autocomplete: opc.completar || 'off',
      required: true, maxLength: opc.max || 200, spellcheck: false,
      autocapitalize: opc.tipo === 'text' ? 'words' : 'none', inputMode: opc.tipo === 'email' ? 'email' : null,
    });
    if (opc.valor) input.value = opc.valor;
    const erro = h('p', { class: 'ac-erro', id: id + '-erro', role: 'alert', hidden: true });
    const linha = h('div', { class: 'ac-entrada' }, input);
    const el = h('div', { class: 'campo ac-campo' }, h('label', { htmlFor: id, text: opc.rotulo }), linha);
    let barra = null;
    if (opc.tipo === 'password') {
      const olho = h('button', { type: 'button', class: 'ac-olho', 'aria-pressed': 'false', 'aria-label': 'Mostrar a senha', title: 'Mostrar a senha' }, icone('olho'));
      olho.addEventListener('click', () => {
        const mostrar = input.type === 'password';
        input.type = mostrar ? 'text' : 'password';
        olho.setAttribute('aria-pressed', mostrar ? 'true' : 'false');
        olho.setAttribute('aria-label', mostrar ? 'Esconder a senha' : 'Mostrar a senha');
        olho.title = mostrar ? 'Esconder a senha' : 'Mostrar a senha';
        trocar(olho, icone(mostrar ? 'olho-fechado' : 'olho'));
      });
      linha.appendChild(olho);
      if (opc.medir) {
        const nivel = h('span', { class: 'ac-forca-texto' });
        barra = h('div', { class: 'ac-forca', 'data-nivel': '0' },
          h('span', { class: 'ac-forca-barras', 'aria-hidden': 'true' }, h('i'), h('i'), h('i'), h('i')), nivel);
        const medir = () => {
          const n = forca(input.value);
          barra.dataset.nivel = String(n);
          nivel.textContent = n ? 'Senha: ' + FORCAS[n].toLowerCase() : '';
        };
        input.addEventListener('input', medir);
        el.appendChild(barra);
      }
    }
    if (opc.ajuda) {
      const ajuda = h('p', { class: 'ajuda', id: id + '-ajuda', text: opc.ajuda });
      input.setAttribute('aria-describedby', id + '-ajuda ' + id + '-erro');
      el.appendChild(ajuda);
    } else {
      input.setAttribute('aria-describedby', id + '-erro');
    }
    el.appendChild(erro);
    const mostrarErro = (texto) => {
      erro.hidden = !texto;
      erro.textContent = texto || '';
      if (texto) input.setAttribute('aria-invalid', 'true'); else input.removeAttribute('aria-invalid');
    };
    input.addEventListener('input', () => mostrarErro(''));
    return { el, input, erro: mostrarErro };
  }

  // formulario junta os campos, o aviso geral e o botão. enviar(valores) devolve
  // { codigo, dados } do programa; em caso de recusa, o erro vai para o campo certo.
  function formulario(opc) {
    const campos = opc.campos;
    const aviso = h('p', { class: 'ac-aviso', role: 'alert', hidden: true });
    const btn = h('button', { type: 'submit', class: 'btn btn-primario btn-grande ac-enviar' }, h('span', { text: opc.botao }));
    const form = h('form', { class: 'ac-form', novalidate: true },
      Object.keys(campos).map((k) => campos[k].el), aviso, btn, opc.depois || null);
    form.addEventListener('submit', async (ev) => {
      ev.preventDefault();
      if (btn.disabled) return;
      aviso.hidden = true;
      let primeiro = null;
      for (const k of Object.keys(campos)) {
        const c = campos[k];
        const vazio = !c.input.value || (c.input.type !== 'password' && !c.input.value.trim());
        c.erro(vazio ? 'Preencha este campo.' : '');
        if (vazio && !primeiro) primeiro = c;
      }
      if (!primeiro && opc.conferir) {
        const recusa = opc.conferir();
        if (recusa) { campos[recusa.campo].erro(recusa.erro); primeiro = campos[recusa.campo]; }
      }
      if (primeiro) { primeiro.input.focus(); return; }
      btn.disabled = true;
      btn.classList.add('ocupado');
      btn.setAttribute('aria-busy', 'true');
      try {
        const valores = {};
        for (const k of Object.keys(campos)) valores[k] = campos[k].input.value;
        const r = await opc.enviar(valores);
        if (r.codigo >= 200 && r.codigo < 300 && r.dados.ok !== false) {
          await opc.deuCerto(r.dados);
          return;
        }
        const texto = r.dados.erro || 'Algo deu errado (código ' + r.codigo + '). Tente de novo.';
        const alvo = r.dados.campo && campos[r.dados.campo];
        if (alvo) { alvo.erro(texto); alvo.input.focus(); }
        else {
          aviso.textContent = texto;
          aviso.hidden = false;
          if (opc.aoRecusar) opc.aoRecusar(r);
        }
      } catch (e) {
        aviso.textContent = e.message || 'Algo deu errado. Tente de novo.';
        aviso.hidden = false;
      } finally {
        btn.disabled = false;
        btn.classList.remove('ocupado');
        btn.removeAttribute('aria-busy');
      }
    });
    return form;
  }

  // ---------- A tela de entrada ----------

  function vitrine() {
    const bolha = (lado, texto) => h('p', { class: 'ac-bolha ac-bolha-' + lado, text: texto });
    return h('aside', { class: 'ac-vitrine', 'aria-hidden': 'true' },
      h('div', { class: 'ac-conversa' },
        bolha('cliente', 'Oi! Vocês entregam hoje?'),
        bolha('ia', 'Entregamos sim. Qual é o seu bairro?'),
        bolha('cliente', 'Centro. Quero fechar o pedido.'),
        h('p', { class: 'ac-passagem' }, icone('assumir'), h('span', { text: 'Lead quente. A equipe assume daqui.' }))),
      h('p', { class: 'ac-frase', text: 'Conversas, contatos e agenda do WhatsApp do seu negócio em um só lugar.' }));
  }

  function mostrar(titulo, descricao, form, opc) {
    opc = opc || {};
    const app = $('.app');
    if (app) app.hidden = true;
    document.querySelectorAll('dialog[open]').forEach((d) => { try { d.close(); } catch (e) { /* tanto faz */ } });
    const h1 = h('h1', { tabIndex: -1, text: titulo });
    const caixa = h('main', { class: 'ac-caixa' },
      h('div', { class: 'marca' },
        h('span', { class: 'marca-sinal', 'aria-hidden': 'true' }),
        h('span', { class: 'marca-nome', text: 'Atendente IA' })),
      h('div', { class: 'ac-miolo' }, h1, h('p', { class: 'descricao', text: descricao }), form),
      h('p', { class: 'ac-pe' }, icone('cadeado'), h('span', { text: 'Este painel só abre neste computador. A senha fica guardada aqui, protegida.' })));
    const nova = h('div', { class: 'ac', id: 'acesso' }, caixa, vitrine());
    const primeiraVez = !raiz;
    if (raiz) raiz.replaceWith(nova); else document.body.appendChild(nova);
    raiz = nova;
    document.title = titulo + ' - Atendente IA';
    const alvo = opc.foco || form.querySelector('input');
    if (alvo) alvo.focus();
    animar(caixa.querySelector('.ac-miolo'), { opacity: [0, 1], y: [8, 0] }, { duration: 0.22, ease: 'easeOut' });
    if (primeiraVez) {
      // Um movimento só, na chegada: a conversa de exemplo aparece fala por fala.
      const falas = nova.querySelectorAll('.ac-conversa > *');
      falas.forEach((f, i) => animar(f, { opacity: [0, 1], y: [10, 0] }, { duration: 0.3, delay: 0.25 + i * 0.16, ease: 'easeOut' }));
    }
  }

  function fecharTela() {
    if (raiz) { raiz.remove(); raiz = null; }
    const app = $('.app');
    if (app) app.hidden = false;
    document.title = 'Atendente IA';
  }

  // entrou guarda quem entrou e segue: cria a senha, se for o caso, ou abre o painel.
  function entrou(usuario) {
    SESSAO.usuario = usuario;
    if (usuario.trocar_senha) { telaCriarSenha(usuario); return; }
    if (jaEntrou) { location.reload(); return; } // a sessão tinha caído: recomeça com tudo fresco
    jaEntrou = true;
    fecharTela();
    if (aoEntrar) { const avisar = aoEntrar; aoEntrar = null; avisar(usuario); }
  }

  function senhasIguais(campos, a, b) {
    return () => (campos[a].input.value !== campos[b].input.value
      ? { campo: b, erro: 'As duas senhas não são iguais. Digite de novo.' } : null);
  }

  function telaPrimeiroAcesso() {
    const campos = {
      nome: campoDe({ rotulo: 'Seu nome', nome: 'name', completar: 'name', max: 60 }),
      email: campoDe({ rotulo: 'E-mail', nome: 'email', tipo: 'email', completar: 'username', max: 254, ajuda: 'É com ele que você vai entrar no painel.' }),
      senha: campoDe({ rotulo: 'Crie uma senha', nome: 'new-password', tipo: 'password', completar: 'new-password', max: 128, medir: true, ajuda: 'Pelo menos ' + minSenha + ' caracteres. Uma frase que só você conhece funciona bem.' }),
      senha2: campoDe({ rotulo: 'Repita a senha', nome: 'new-password-2', tipo: 'password', completar: 'new-password', max: 128 }),
    };
    const form = formulario({
      campos, botao: 'Criar conta e continuar',
      conferir: senhasIguais(campos, 'senha', 'senha2'),
      enviar: (v) => chamar('/api/primeiro-acesso', v),
      deuCerto: (d) => entrou(d.usuario),
      aoRecusar: (r) => { if (r.codigo === 409) conferir(); },
      depois: h('p', { class: 'ac-nota', text: 'Guarde bem essa senha. O painel ainda não manda e-mail de recuperação.' }),
    });
    mostrar('Crie a conta do super administrador',
      'É a conta principal do painel: ela configura o atendente e cria o acesso das outras pessoas. Você escolhe a senha agora.', form);
  }

  function telaEntrar(aviso) {
    const campos = {
      email: campoDe({ rotulo: 'E-mail', nome: 'email', tipo: 'email', completar: 'username', max: 254 }),
      senha: campoDe({ rotulo: 'Senha', nome: 'password', tipo: 'password', completar: 'current-password', max: 128 }),
    };
    const ajuda = h('div', { class: 'ac-ajuda', hidden: true },
      h('p', { text: 'Sua conta foi criada por outra pessoa? Peça uma senha provisória nova ao super administrador, em Ajustes, Equipe e acessos.' }),
      h('p', { text: 'Você é o super administrador? Neste computador, abra o Claude Code na pasta do atendente e peça: "esqueci a senha do painel". Depois recarregue esta página e crie uma senha nova.' }));
    const esqueci = h('button', { type: 'button', class: 'ac-link', 'aria-expanded': 'false', text: 'Esqueci minha senha' });
    esqueci.addEventListener('click', () => {
      ajuda.hidden = !ajuda.hidden;
      esqueci.setAttribute('aria-expanded', ajuda.hidden ? 'false' : 'true');
      if (!ajuda.hidden) animar(ajuda, { opacity: [0, 1], y: [-4, 0] }, { duration: 0.18 });
    });
    const form = formulario({
      campos, botao: 'Entrar',
      enviar: (v) => chamar('/api/entrar', v),
      deuCerto: (d) => entrou(d.usuario),
      aoRecusar: () => { campos.senha.input.value = ''; campos.senha.input.focus(); },
      depois: h('div', { class: 'ac-depois' }, esqueci, ajuda),
    });
    mostrar('Entrar no painel', aviso || 'Use o e-mail e a senha da sua conta.', form);
  }

  function telaCriarSenha(usuario) {
    const campos = {
      atual: campoDe({ rotulo: 'Senha provisória', nome: 'password', tipo: 'password', completar: 'current-password', max: 128, ajuda: 'A que você recebeu de quem criou a sua conta.' }),
      nova: campoDe({ rotulo: 'Crie a sua senha', nome: 'new-password', tipo: 'password', completar: 'new-password', max: 128, medir: true, ajuda: 'Pelo menos ' + minSenha + ' caracteres. Só você vai saber.' }),
      nova2: campoDe({ rotulo: 'Repita a senha', nome: 'new-password-2', tipo: 'password', completar: 'new-password', max: 128 }),
    };
    const sair = h('button', { type: 'button', class: 'ac-link', text: 'Entrar com outra conta' });
    sair.addEventListener('click', () => encerrar());
    const form = formulario({
      campos, botao: 'Salvar e entrar',
      conferir: senhasIguais(campos, 'nova', 'nova2'),
      enviar: (v) => chamar('/api/senha', v),
      deuCerto: (d) => entrou(d.usuario),
      depois: h('div', { class: 'ac-depois' }, sair),
    });
    mostrar('Crie a sua senha', 'Olá, ' + (usuario.nome || '').split(' ')[0] + '. Antes de continuar, troque a senha provisória por uma que só você conhece.', form);
  }

  function telaRedefinir(nome) {
    const campos = {
      senha: campoDe({ rotulo: 'Senha nova', nome: 'new-password', tipo: 'password', completar: 'new-password', max: 128, medir: true, ajuda: 'Pelo menos ' + minSenha + ' caracteres.' }),
      senha2: campoDe({ rotulo: 'Repita a senha', nome: 'new-password-2', tipo: 'password', completar: 'new-password', max: 128 }),
    };
    const voltar = h('button', { type: 'button', class: 'ac-link', text: 'Lembrei a senha, quero entrar' });
    voltar.addEventListener('click', () => telaEntrar());
    const form = formulario({
      campos, botao: 'Salvar senha nova e entrar',
      conferir: senhasIguais(campos, 'senha', 'senha2'),
      enviar: (v) => chamar('/api/redefinir-senha', v),
      deuCerto: (d) => entrou(d.usuario),
      aoRecusar: (r) => { if (r.codigo === 403) conferir(); },
      depois: h('div', { class: 'ac-depois' }, voltar),
    });
    mostrar('Crie uma senha nova', 'A recuperação foi aberta neste computador. Escolha a senha nova do super administrador' + (nome ? ', ' + nome : '') + '.', form);
  }

  function telaSemResposta(erro) {
    const tentar = h('button', { type: 'submit', class: 'btn btn-primario btn-grande ac-enviar' }, h('span', { text: 'Tentar de novo' }));
    const form = h('form', { class: 'ac-form' }, tentar);
    form.addEventListener('submit', (ev) => { ev.preventDefault(); conferir(); });
    mostrar('O atendente não respondeu', erro, form, { foco: tentar });
  }

  // conferir pergunta ao programa como está o acesso e mostra a tela certa.
  async function conferir() {
    let r;
    try {
      r = await chamar('/api/sessao');
    } catch (e) {
      telaSemResposta(e.message);
      return;
    }
    const d = r.dados;
    if (d.min_senha) minSenha = d.min_senha;
    if (d.usuario) entrou(d.usuario);
    else if (d.precisa_criar) telaPrimeiroAcesso();
    else if (d.redefinir) telaRedefinir(d.redefinir_nome);
    else telaEntrar();
  }

  // entrar é chamado na partida do painel. Resolve quando alguém está dentro.
  function entrar() {
    return new Promise((resolver) => {
      aoEntrar = resolver;
      conferir();
    });
  }

  // caiu é chamado quando o programa recusa um pedido por falta de sessão.
  function caiu() {
    if (raiz || !SESSAO.usuario) return;
    SESSAO.usuario = null;
    conferir();
  }

  async function encerrar() {
    try { await chamar('/api/sair', {}); } catch (e) { /* sai mesmo sem resposta */ }
    SESSAO.usuario = null;
    location.reload();
  }

  async function sair() {
    if (typeof temAlteracoes === 'function' && temAlteracoes()) {
      const ok = await confirmar({ titulo: 'Sair sem salvar?', texto: 'Há alterações que ainda não foram salvas. Se sair agora, elas se perdem.', botao: 'Sair mesmo assim', perigo: true });
      if (!ok) return;
      S.negocioSujo = false; S.ajustesSujos = false;
    }
    await encerrar();
  }

  // trocarSenha abre a janela para quem está logado trocar a própria senha.
  function trocarSenha() {
    const campos = {
      atual: campoDe({ rotulo: 'Senha atual', nome: 'password', tipo: 'password', completar: 'current-password', max: 128 }),
      nova: campoDe({ rotulo: 'Senha nova', nome: 'new-password', tipo: 'password', completar: 'new-password', max: 128, medir: true, ajuda: 'Pelo menos ' + minSenha + ' caracteres.' }),
      nova2: campoDe({ rotulo: 'Repita a senha nova', nome: 'new-password-2', tipo: 'password', completar: 'new-password', max: 128 }),
    };
    let j = null;
    const form = formulario({
      campos, botao: 'Trocar senha',
      conferir: senhasIguais(campos, 'nova', 'nova2'),
      enviar: (v) => chamar('/api/senha', v),
      deuCerto: (d) => {
        SESSAO.usuario = d.usuario;
        j.fechar(true);
        toast('Senha trocada. As outras sessões desta conta foram encerradas.');
      },
    });
    j = janela({ titulo: 'Trocar senha', descricao: 'Vale já na próxima vez que você entrar.', corpo: form, classe: 'ac-janela' });
    campos.atual.input.focus();
  }

  function pode(nivel) {
    const u = SESSAO.usuario;
    return !!u && (PAPEIS[u.papel] || 0) >= (PAPEIS[nivel] || 99);
  }

  function iniciais(nome) {
    const partes = String(nome || '').trim().split(/\s+/).filter(Boolean);
    if (!partes.length) return '?';
    return (partes[0][0] + (partes.length > 1 ? partes[partes.length - 1][0] : '')).toUpperCase();
  }

  // pintarConta mostra, no pé do menu lateral, quem está logado e as ações da conta.
  function pintarConta(el) {
    const u = SESSAO.usuario;
    if (!el || !u) return;
    trocar(el,
      h('div', { class: 'conta-quem' },
        h('span', { class: 'conta-avatar', 'aria-hidden': 'true', text: iniciais(u.nome) }),
        h('div', { class: 'conta-textos' },
          h('span', { class: 'conta-nome', text: u.nome, title: u.email }),
          h('span', { class: 'conta-papel', text: u.papel_nome || '' }))),
      h('div', { class: 'conta-acoes' },
        h('button', { type: 'button', class: 'btn btn-fantasma', onclick: trocarSenha }, icone('cadeado'), h('span', { text: 'Trocar senha' })),
        h('button', { type: 'button', class: 'btn btn-fantasma', onclick: sair }, icone('sair'), h('span', { text: 'Sair' }))));
    el.hidden = false;
  }

  return { entrar, caiu, sair, trocarSenha, pode, pintarConta, campoDe, formulario, chamar, forca, iniciais };
})();
