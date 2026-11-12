'use strict';
// Casca do painel: rotas, barra de situação, busca periódica e a configuração inicial.

const ABAS = ['conversas', 'contatos', 'agenda', 'relatorio', 'assistente', 'conhecimento', 'treinar', 'ajustes'];
const ARQUIVOS_TEXTO = ['personalidade', 'cerebro', 'regras', 'aprendizados'];
// Abas que só administradores veem. O programa também barra as rotas delas.
const ABAS_DE_ADMIN = ['assistente', 'conhecimento', 'treinar', 'ajustes'];

function abaPermitida(aba) {
  return !ABAS_DE_ADMIN.includes(aba) || ACESSO.pode('admin');
}

let telaAtual = null;
let batidas = 0;
let pulsando = null;

// ---------- Carregar e salvar ----------

async function carregarConfig(forcar) {
  if (S.config && !forcar) return S.config;
  const r = await api.get('/api/config');
  S.config = r.config || {};
  S.variaveis = r.variaveis || [];
  S.modelos = r.modelos || [];
  S.tons = r.tons || [];
  return S.config;
}

// salvarConfig manda só a parte que mudou e atualiza a cópia local.
async function salvarConfig(parte) {
  const r = await api.post('/api/config', parte);
  if (r.config) S.config = r.config;
  try { await carregarConfig(true); } catch (e) { /* fica com a cópia da resposta */ }
  // Rascunhos que ninguém mexeu voltam a nascer da configuração nova.
  if (!S.negocioSujo) S.negocio = null;
  if (!S.ajustesSujos) S.ajustes = null;
  if (!S.regrasSujas) S.regras = null;
  pulsar();
  return S.config;
}

async function carregarTextos() {
  const r = await api.get('/api/textos');
  S.textos = r;
  for (const arquivo of ARQUIVOS_TEXTO) {
    const t = r[arquivo] || {};
    const texto = t.texto || '';
    const ed = S.editores[arquivo];
    if (!ed) {
      S.editores[arquivo] = { salvo: texto, atual: texto, padrao: !!t.padrao, antesDaIA: null };
    } else {
      const sujo = ed.atual !== ed.salvo;
      ed.salvo = texto;
      ed.padrao = !!t.padrao;
      if (!sujo) ed.atual = texto;
    }
  }
  return S.editores;
}

async function carregarAgenda() {
  S.agenda = await api.get('/api/agenda');
  return S.agenda;
}

async function carregarMateriais() {
  const r = await api.get('/api/conhecimento');
  S.materiais = r.materiais || [];
  if (Array.isArray(r.formatos) && r.formatos.length) S.formatos = r.formatos.map((f) => String(f).toLowerCase());
  if (Number(r.limite_mb) > 0) S.limiteMb = Number(r.limite_mb);
  return S.materiais;
}

function temAlteracoes() {
  return S.negocioSujo || S.regrasSujas || S.ajustesSujos ||
    Object.values(S.editores).some((ed) => ed.atual !== ed.salvo);
}

// ---------- Tema ----------

// O tema claro é o padrão. O escuro só vale quando a pessoa escolhe.
function temaAtual() {
  return document.documentElement.dataset.theme === 'dark' ? 'dark' : 'light';
}

function pintarBotaoTema() {
  const claro = temaAtual() === 'light';
  $('#uso-tema').setAttribute('href', claro ? '#i-lua' : '#i-sol');
  $('#txt-tema').textContent = claro ? 'Tema escuro' : 'Tema claro';
}

function aplicarTema(tema) {
  document.documentElement.dataset.theme = tema === 'dark' ? 'dark' : 'light';
  try { localStorage.setItem('tema', temaAtual()); } catch (e) { /* sem armazenamento */ }
  pintarBotaoTema();
  // Quem desenha com cores próprias (o calendário, os gráficos) pode ouvir este aviso.
  document.dispatchEvent(new CustomEvent('tema', { detail: temaAtual() }));
}

// ---------- Rotas ----------

function telaExiste(aba) {
  return !!(TELAS[aba] && typeof TELAS[aba].montar === 'function');
}

// lerRota entende "#agenda" e "#assistente/cerebro". Aba desconhecida, ou tela que
// ainda não foi carregada, cai em Conversas.
function lerRota() {
  const partes = location.hash.replace(/^#\/?/, '').split('/');
  let aba = ABAS.includes(partes[0]) ? partes[0] : 'conversas';
  let sub = partes[1] || '';
  if (!telaExiste(aba) || !abaPermitida(aba)) { aba = 'conversas'; sub = ''; }
  return { aba, sub };
}

let animacaoDeTela = null;

function navegar(foco) {
  const rota = lerRota();
  const abaAnterior = S.rota ? S.rota.aba : '';
  if (telaAtual && telaAtual.sair) {
    try { telaAtual.sair(); } catch (e) { console.error(e); }
  }
  S.rota = rota;
  document.querySelectorAll('.nav a').forEach((a) => {
    const ativo = a.dataset.aba === rota.aba;
    a.classList.toggle('ativo', ativo);
    if (ativo) {
      a.setAttribute('aria-current', 'page');
      // No celular o menu rola de lado: o item aberto fica sempre à vista.
      if (foco && a.scrollIntoView) a.scrollIntoView({ block: 'nearest', inline: 'nearest' });
    } else {
      a.removeAttribute('aria-current');
    }
  });
  const main = $('#conteudo');
  main.replaceChildren();
  main.dataset.aba = rota.aba;
  telaAtual = TELAS[rota.aba] || null;
  try {
    if (!telaExiste(rota.aba)) throw new Error('Esta tela não carregou. Recarregue a página.');
    const montando = telaAtual.montar(main, rota.sub);
    if (montando && typeof montando.catch === 'function') {
      montando.catch((e) => {
        console.error(e);
        if (S.rota === rota) trocar(main, falhaCarga(e, () => navegar()));
      });
    }
  } catch (e) {
    console.error(e);
    trocar(main, falhaCarga(e, () => navegar()));
  }
  if (foco) {
    window.scrollTo(0, 0);
    main.focus({ preventScroll: true });
    // Entrada suave da tela nova. Em tela estreita só clareia, sem deslocar,
    // para não mexer com o que fica preso na tela (a conversa aberta no celular).
    if (animacaoDeTela) animacaoDeTela.cancel();
    const mesmaAba = abaAnterior === rota.aba;
    const estreita = window.matchMedia('(max-width: 900px)').matches;
    animacaoDeTela = animar(main, mesmaAba || estreita ? { opacity: [0, 1] } : { opacity: [0, 1], y: [6, 0] },
      { duration: mesmaAba ? 0.14 : 0.18, ease: 'easeOut' });
  }
}

// ---------- Barra de situação ----------

let chavePrincipal = null;
let chavePintada = false; // a primeira pintura não anima
let seloPintado = false;

function montarChave() {
  chavePrincipal = chave('Atendente pausado', false, async (novo, b) => {
    if (!ACESSO.pode('admin')) {
      toast('Só um administrador liga ou pausa o atendente.', 'erro');
      return;
    }
    b.disabled = true;
    try {
      await api.post('/api/liga', { ligar: novo });
      toast(novo ? 'Atendente ligado. Ele já responde as conversas privadas.' : 'Atendente pausado. As mensagens só ficam registradas.');
    } catch (e) {
      toast(e.message, 'erro');
    }
    b.disabled = false;
    await pulsar();
  }, { manual: true, classe: 'chave-principal' });
  por($('#est-chave'), [
    chavePrincipal,
    h('p', { class: 'estado-aviso', text: 'Ele responde todas as conversas privadas deste número' }),
  ]);
}

function pintarEstado() {
  const ponto = $('#est-ponto');
  const msg = $('#est-msg');
  if (S.semConexao) {
    ponto.dataset.status = 'sem-conexao';
    msg.textContent = 'Sem resposta do atendente. Confira se ele está ligado neste computador.';
    return;
  }
  const e = S.estado;
  if (!e) return;
  const statusNovo = e.status || 'iniciando';
  const mudou = ponto.dataset.status !== statusNovo && ponto.dataset.status !== 'iniciando';
  ponto.dataset.status = statusNovo;
  msg.textContent = e.mensagem || 'Iniciando...';
  if (mudou) {
    // A situação do WhatsApp mudou: o ponto pulsa uma vez e o texto entra de novo.
    animar(ponto, { scale: [1, 1.7, 1] }, { duration: 0.45 });
    animar(msg, { opacity: [0, 1], x: [-6, 0] }, { duration: 0.22 });
  }

  const cerebro = $('#est-cerebro');
  if (cerebro.dataset.modo !== e.modo) {
    cerebro.dataset.modo = e.modo || '';
    const nomes = { claude: 'Cérebro: Claude', openai: 'Cérebro: ChatGPT', api: 'Cérebro: API', teste: 'Modo teste, sem Claude' };
    trocar(cerebro, icone('ia'), nomes[e.modo] || '');
  }

  const ligadoAntes = chavePrincipal.getAttribute('aria-checked') === 'true';
  chavePrincipal.setAttribute('aria-checked', e.ligado ? 'true' : 'false');
  const textoChave = chavePrincipal.querySelector('.chave-texto');
  textoChave.textContent = e.ligado ? 'Atendente ligado' : 'Atendente pausado';
  if (chavePintada && ligadoAntes !== !!e.ligado) animar(textoChave, { opacity: [0, 1] }, { duration: 0.2 });
  chavePintada = true;

  const avisos = [];
  if (e.erro) avisos.push(e.erro);
  // IA sem login: explica em uma frase e oferece o botão que resolve.
  const iaFora = !!e.aviso_cerebro || (!!e.ia && e.ia.instalado && !e.ia.conectado);
  const podeConectar = iaFora && ACESSO.pode('admin');
  if (iaFora) {
    const causas = {
      openai_chave: 'A OpenAI não aceitou a chave guardada',
      openai_credito: 'A conta da OpenAI está sem crédito',
      openai_modelo: 'O modelo escolhido não existe na conta da OpenAI',
    };
    avisos.push((causas[(e.ia || {}).motivo] || 'A IA está desconectada')
      + ', por isso o assistente não está respondendo. As conversas novas vão para a fila da equipe.'
      + (podeConectar ? '' : ' Avise um administrador do painel.'));
  } else if (e.erro_cerebro) avisos.push('O cérebro falhou na última tentativa: ' + e.erro_cerebro);
  const faixa = $('#faixa');
  const marca = avisos.join('\n') + (podeConectar ? '\n[conectar]' : '');
  if (faixa.dataset.marca !== marca) {
    const estavaEscondida = faixa.hidden;
    faixa.dataset.marca = marca;
    faixa.hidden = !avisos.length;
    trocar(faixa, avisos.length ? [icone('alerta'), h('div', null, avisos.map((a) => h('p', { text: a }))),
      podeConectar ? botao('Conectar a IA', { classe: 'btn-primario faixa-acao', icone: 'ia', aoClicar: () => CONEXAO.abrir() }) : null] : []);
    if (avisos.length && estavaEscondida) animar(faixa, { opacity: [0, 1], y: [-8, 0] }, { duration: 0.2 });
  }

  const esperando = (e.contadores && e.contadores.aguardando) || 0;
  const selo = $('#selo-aguardando');
  selo.hidden = !esperando;
  selo.textContent = String(esperando);
  selo.title = esperando === 1 ? '1 conversa na fila, esperando uma pessoa' : esperando + ' conversas na fila, esperando uma pessoa';
  if (selo.dataset.conta !== String(esperando)) {
    const subiu = esperando > Number(selo.dataset.conta || 0);
    selo.dataset.conta = String(esperando);
    if (subiu && seloPintado) animar(selo, { scale: [1, 1.35, 1] }, { duration: 0.35 });
  }
  seloPintado = true;
  document.title = (esperando ? '(' + esperando + ') ' : '') + 'Atendente IA';
  $('#versao').textContent = e.versao ? 'Versão ' + e.versao : '';
}

// pulsar busca a situação e avisa a tela aberta. Devolve uma promessa.
function pulsar() {
  if (pulsando) return pulsando.then(() => pulsar());
  pulsando = (async () => {
    try {
      S.estado = await api.get('/api/estado');
      S.semConexao = false;
    } catch (e) {
      S.semConexao = true;
    }
    batidas++;
    try {
      pintarEstado();
      if (telaAtual && telaAtual.atualizar) telaAtual.atualizar(batidas);
      atualizarInicio();
    } catch (e) {
      console.error(e);
    }
  })();
  const esta = pulsando;
  esta.then(() => { if (pulsando === esta) pulsando = null; });
  return esta;
}

// ---------- Bloco do WhatsApp (QR ou conectado) ----------

function criarBlocoWhats() {
  const el = h('div', { class: 'bloco-whats' });
  let marca = null;
  let img = null;
  let jaEntrou = false;

  function atualizar() {
    const e = S.estado;
    if (!e) {
      if (marca !== 'nada') {
        marca = 'nada';
        trocar(el, h('div', { class: 'whats-espera' }, h('span', { class: 'rodando' }), 'Consultando o WhatsApp...'));
      }
      return;
    }
    const nova = [e.status, e.tem_qr, e.mensagem, e.nome, e.numero, e.erro].join('|');
    if (nova === marca) return;
    marca = nova;
    img = null;
    const partes = [];
    if (e.status === 'conectado') {
      partes.push(h('div', { class: 'whats-ok' }, icone('check'),
        h('div', null,
          h('strong', { text: 'Conectado' + (e.nome ? ' como ' + e.nome : '') }),
          h('span', { text: e.numero || '' }))));
    } else if (e.tem_qr) {
      img = h('img', { alt: 'Código QR para conectar o WhatsApp', width: 212, height: 212, src: '/qr.png?r=' + Math.random() });
      partes.push(h('div', { class: 'qr' }, img),
        h('ol', { class: 'passos-qr' },
          h('li', null, h('span', { text: 'Abra o WhatsApp do negócio no celular.' })),
          h('li', null, h('span', { text: 'Toque em Configurações e depois em Aparelhos conectados.' })),
          h('li', null, h('span', { text: 'Toque em Conectar aparelho e aponte a câmera para este código.' }))));
    } else {
      partes.push(h('div', { class: 'whats-espera' }, h('span', { class: 'rodando' }), e.mensagem || 'Preparando o código...'));
    }
    if (e.erro) partes.push(h('p', { class: 'whats-erro', role: 'alert' }, icone('alerta'), h('span', { text: e.erro })));
    trocar(el, partes);
  }

  // O código QR muda sozinho: a imagem é trocada a cada 2 segundos.
  const relogio = setInterval(() => {
    if (!el.isConnected) {
      if (jaEntrou) clearInterval(relogio);
      return;
    }
    jaEntrou = true;
    if (img && !document.hidden && S.estado && S.estado.tem_qr) img.src = '/qr.png?r=' + Math.random();
  }, 2000);

  atualizar();
  return { el, atualizar };
}

// ---------- Configuração inicial ----------

const PASSOS = [
  { id: 'ia', rotulo: 'IA' },
  { id: 'negocio', rotulo: 'Seu negócio' },
  { id: 'assistente', rotulo: 'Seu assistente' },
  { id: 'avisos', rotulo: 'Avisos' },
  { id: 'whats', rotulo: 'WhatsApp' },
  { id: 'pronto', rotulo: 'Pronto' },
];
const TONS_PADRAO = ['simpático e profissional', 'descontraído e próximo', 'formal e objetivo'];
const CAMPOS_INICIO = ['negocio', 'segmento', 'cidade', 'horario', 'produtos'];

let inicio = null; // peças da configuração inicial aberta

async function abrirInicio() {
  const dlg = $('#inicio');
  if (dlg.open) return;
  try {
    await carregarConfig();
  } catch (e) {
    toast(e.message, 'erro');
    return;
  }
  if (dlg.open) return;
  const c = S.config;
  // Quem já está com a IA conectada começa direto no negócio.
  S.inicio.passo = S.estado && S.estado.ia && S.estado.ia.conectado ? 1 : 0;
  S.inicio.dados = {
    negocio: c.negocio || '', segmento: c.segmento || '', cidade: c.cidade || '',
    horario: c.horario || '', produtos: c.produtos || '',
    assistente: c.assistente || 'Ana',
    tom: c.tom || (S.tons[0] || TONS_PADRAO[0]),
    numero_humano: c.numero_humano || '',
    outros: {},
  };
  const passos = h('ol', { class: 'passos', 'aria-label': 'Passos da configuração' });
  const corpo = h('div', { class: 'inicio-corpo' });
  const pe = h('div', { class: 'inicio-pe' });
  trocar(dlg, h('div', { class: 'inicio-caixa' },
    h('div', { class: 'inicio-topo' },
      h('div', { class: 'marca' },
        h('span', { class: 'marca-sinal', 'aria-hidden': 'true' }),
        h('span', { class: 'marca-nome', text: 'Atendente IA' })),
      botao('Sair da configuração', { classe: 'btn-fantasma', icone: 'fechar', aoClicar: fecharInicio })),
    passos, corpo, pe));
  inicio = { passos, corpo, pe, whats: null, conexao: null, textoContinuar: null, resumoWhats: null, passoPintado: null };
  abrirDialogo(dlg, { telaCheia: true });
  pintarPasso();
}

function fecharInicio() {
  if (inicio && inicio.conexao) { inicio.conexao.parar(); inicio.conexao = null; }
  S.inicio.dispensado = true;
  const dlg = $('#inicio');
  if (dlg.open) dlg.close();
}

function avancar() {
  S.inicio.passo = Math.min(PASSOS.length - 1, S.inicio.passo + 1);
  pintarPasso();
}

function whatsEmTexto() {
  const e = S.estado || {};
  if (e.status === 'conectado') return 'Conectado' + (e.nome ? ' como ' + e.nome : '') + (e.numero ? ' (' + e.numero + ')' : '');
  return 'Ainda não conectado. Dá para conectar depois, em Ajustes.';
}

function pintarPasso() {
  if (!inicio) return;
  const i = S.inicio.passo;
  const d = S.inicio.dados;
  const corpo = inicio.corpo;
  inicio.whats = null;
  if (inicio.conexao) { inicio.conexao.parar(); inicio.conexao = null; }
  inicio.textoContinuar = null;
  inicio.resumoWhats = null;

  trocar(inicio.passos, PASSOS.map((p, n) => h('li', {
    class: n < i ? 'feito' : null, 'aria-current': n === i ? 'step' : null,
  }, h('span', { text: (n + 1) + '. ' + p.rotulo }))));

  const titulo = (texto) => h('h1', { text: texto, tabindex: '-1' });
  let continuar = null;
  const extras = [];

  if (PASSOS[i].id === 'ia') {
    const lugar = h('div', { class: 'inicio-ia' });
    trocar(corpo,
      titulo('Conecte a IA do seu assistente'),
      h('p', { class: 'descricao', text: 'É a IA que lê as mensagens e responde os seus clientes. Ela usa a sua conta do Claude. Você entra uma vez e ela continua conectada.' }),
      lugar);
    // Um botão principal por tela: enquanto a IA não conecta, o principal é "Conectar o Claude".
    continuar = botao('Conectar depois', { classe: 'btn-suave', aoClicar: avancar });
    const botaoSeguir = continuar;
    const textoBotao = continuar.querySelector('span');
    inicio.conexao = CONEXAO.painel(lugar, {
      aoMudar: (r) => {
        textoBotao.textContent = r.conectado ? 'Continuar' : 'Conectar depois';
        botaoSeguir.classList.toggle('btn-primario', !!r.conectado);
        botaoSeguir.classList.toggle('btn-suave', !r.conectado);
      },
    });
  } else if (PASSOS[i].id === 'negocio') {
    trocar(corpo,
      titulo('Conte sobre o seu negócio'),
      h('p', { class: 'descricao', text: 'Em poucos minutos o seu assistente está respondendo no WhatsApp. Comece dizendo o que você vende.' }),
      h('div', { class: 'caixa-ia' },
        campo('Escreva do seu jeito', areaTexto(S.inicio, 'descricao', {
          rows: 3, placeholder: 'Ex.: Barbearia em Vitória. Corte a R$ 40 e plano mensal de R$ 150. Aberta de segunda a sábado, das 9h às 19h.',
        }), 'A IA lê o seu texto e preenche os campos abaixo. Depois você revisa. Se preferir, preencha direto.'),
        h('div', { class: 'acoes' },
          botao('Preencher com IA', {
            classe: 'btn-suave', icone: 'ia',
            aoClicar: async () => {
              if (!S.inicio.descricao.trim()) throw new Error('Escreva alguma coisa sobre o negócio primeiro.');
              const r = await api.post('/api/ia/negocio', { descricao: S.inicio.descricao.trim() });
              let quantos = 0;
              for (const [k, v] of Object.entries(r.campos || {})) {
                if (typeof v !== 'string' || !v.trim()) continue;
                if (CAMPOS_INICIO.includes(k)) d[k] = v.trim();
                else d.outros[k] = v.trim();
                quantos++;
              }
              if (!quantos) throw new Error('A IA não encontrou dados para preencher. Tente escrever com mais detalhes.');
              toast('Campos preenchidos. Revise antes de continuar.');
              pintarPasso();
            },
          }),
          h('span', { class: 'espera-ia', text: 'Pode levar até um minuto.' }))),
      h('div', { class: 'grade-campos' },
        campo('Nome do negócio', entrada(d, 'negocio', { autocomplete: 'off' })),
        campo('Segmento', entrada(d, 'segmento', { placeholder: 'Ex.: Barbearia', autocomplete: 'off' })),
        campo('Cidade', entrada(d, 'cidade', { placeholder: 'Ex.: São Mateus, ES', autocomplete: 'off' })),
        campo('Horário de atendimento', entrada(d, 'horario', { placeholder: 'Ex.: segunda a sábado, das 9h às 19h', autocomplete: 'off' })),
        h('div', { class: 'largo' }, campo('Produtos, serviços e preços', areaTexto(d, 'produtos', { rows: 4 }),
          'O assistente só informa preço que estiver escrito. Você completa o resto depois, na aba Assistente.'))));
    continuar = botao('Continuar', {
      classe: 'btn-primario',
      aoClicar: async () => {
        if (!d.negocio.trim()) throw new Error('Escreva o nome do negócio para continuar.');
        const envio = Object.assign({}, d.outros);
        for (const k of CAMPOS_INICIO) envio[k] = d[k].trim();
        await salvarConfig(envio);
        avancar();
      },
    });
  } else if (PASSOS[i].id === 'assistente') {
    const tons = (S.tons.length ? S.tons : TONS_PADRAO).slice(0, 8);
    if (d.tom && !tons.includes(d.tom)) tons.unshift(d.tom);
    trocar(corpo,
      titulo('Como é o seu assistente'),
      h('p', { class: 'descricao', text: 'É ele quem conversa com os clientes. Escolha o nome e o jeito de falar.' }),
      campo('Nome do assistente', entrada(d, 'assistente', { placeholder: 'Ex.: Ana', autocomplete: 'off' }), 'É como ele se apresenta no WhatsApp.'),
      h('fieldset', { class: 'campo' },
        h('legend', { class: 'rotulo', text: 'Tom de voz' }),
        h('div', { class: 'tons' }, tons.map((t) => {
          const r = h('input', { type: 'radio', name: 'tom-inicio', value: t, checked: d.tom === t });
          r.addEventListener('change', () => { d.tom = t; });
          return h('label', { class: 'tom' }, r, h('span', { text: t }));
        }))));
    continuar = botao('Continuar', {
      classe: 'btn-primario',
      aoClicar: async () => {
        await salvarConfig({ assistente: d.assistente.trim() || 'Ana', tom: d.tom });
        avancar();
      },
    });
  } else if (PASSOS[i].id === 'avisos') {
    trocar(corpo,
      titulo('Quem recebe os avisos'),
      h('p', { class: 'descricao', text: 'Quando aparecer um lead quente, ou quando alguém pedir para falar com uma pessoa, o assistente manda um aviso para este WhatsApp.' }),
      campo('Número que recebe os avisos', entrada(d, 'numero_humano', { type: 'tel', inputMode: 'tel', placeholder: '5527999990000', autocomplete: 'off' }),
        'Com DDI e DDD, só números. Use um número diferente do WhatsApp do negócio. Pode deixar vazio e preencher depois, em Ajustes.'));
    continuar = botao('Continuar', {
      classe: 'btn-primario',
      aoClicar: async () => {
        d.numero_humano = d.numero_humano.replace(/\D/g, '');
        await salvarConfig({ numero_humano: d.numero_humano });
        avancar();
      },
    });
  } else if (PASSOS[i].id === 'whats') {
    inicio.whats = criarBlocoWhats();
    trocar(corpo,
      titulo('Conecte o WhatsApp do negócio'),
      h('p', { class: 'descricao', text: 'O assistente responde as conversas privadas deste número. Use o WhatsApp do negócio, não o pessoal.' }),
      inicio.whats.el);
    continuar = botao('Continuar', { classe: 'btn-primario', aoClicar: avancar });
    inicio.textoContinuar = continuar.querySelector('span');
  } else {
    inicio.resumoWhats = h('dd');
    trocar(corpo,
      titulo('Tudo pronto'),
      h('p', { class: 'descricao', text: 'Confira e ligue. Depois, mande um "Oi" de outro celular para o número do negócio e veja a conversa aparecer no painel.' }),
      h('dl', { class: 'resumo-inicio' },
        h('dt', { text: 'Negócio' }), h('dd', { text: [d.negocio, d.segmento, d.cidade].filter(Boolean).join(', ') }),
        h('dt', { text: 'Assistente' }), h('dd', { text: (d.assistente || 'Ana') + ', com tom ' + d.tom }),
        h('dt', { text: 'Avisos' }), h('dd', { text: d.numero_humano ? 'Vão para o número ' + d.numero_humano : 'Sem número de avisos por enquanto' }),
        h('dt', { text: 'WhatsApp' }), inicio.resumoWhats));
    extras.push(botao('Entrar sem ligar', { classe: 'btn-suave', aoClicar: fecharInicio }));
    continuar = botao('Ligar o atendente', {
      classe: 'btn-primario btn-grande', icone: 'ligar',
      aoClicar: async () => {
        await api.post('/api/liga', { ligar: true });
        toast('Atendente ligado. Ele já responde as conversas privadas.');
        fecharInicio();
        location.hash = '#conversas';
        await pulsar();
      },
    });
  }

  trocar(inicio.pe,
    i > 0 ? botao('Voltar', { classe: 'btn-suave', icone: 'voltar', aoClicar: () => { S.inicio.passo--; pintarPasso(); } }) : null,
    h('div', { class: 'direita' }, extras, continuar));
  atualizarInicio();
  const h1 = corpo.querySelector('h1');
  if (h1) h1.focus({ preventScroll: true });
  $('#inicio').scrollTop = 0;
  if (inicio.passoPintado != null && inicio.passoPintado !== i) {
    // Troca de passo: o conteúdo entra do lado para onde a pessoa foi.
    animar(corpo, { opacity: [0, 1], x: [i > inicio.passoPintado ? 16 : -16, 0] }, { duration: 0.18 });
  }
  inicio.passoPintado = i;
}

function atualizarInicio() {
  if (!inicio) return;
  if (inicio.whats) inicio.whats.atualizar();
  const conectado = S.estado && S.estado.status === 'conectado';
  if (inicio.textoContinuar) inicio.textoContinuar.textContent = conectado ? 'Continuar' : 'Conectar depois';
  if (inicio.resumoWhats) inicio.resumoWhats.textContent = whatsEmTexto();
}

// ---------- Partida ----------

// aplicarAcesso mostra a conta de quem entrou e esconde o que o acesso dela não alcança.
function aplicarAcesso() {
  ACESSO.pintarConta($('#conta'));
  const admin = ACESSO.pode('admin');
  document.querySelectorAll('.nav a').forEach((a) => { a.hidden = !abaPermitida(a.dataset.aba); });
  const whats = $('#est-whats');
  if (!admin && whats) {
    whats.removeAttribute('href');
    whats.removeAttribute('title');
  }
  if (!admin) chavePrincipal.setAttribute('aria-disabled', 'true');
}

// iniciar prepara o tema e espera alguém entrar. O painel só é montado depois.
function iniciar() {
  pintarBotaoTema();
  $('#btn-tema').addEventListener('click', () => aplicarTema(temaAtual() === 'light' ? 'dark' : 'light'));
  ACESSO.entrar().then(partir);
}

function partir() {
  montarChave();
  aplicarAcesso();

  const dlg = $('#inicio');
  dlg.addEventListener('cancel', () => { S.inicio.dispensado = true; });
  dlg.addEventListener('close', () => {
    inicio = null;
    dlg.replaceChildren();
    navegar(); // a tela por baixo pode ter dados antigos
  });

  window.addEventListener('hashchange', () => navegar(true));
  window.addEventListener('beforeunload', (ev) => {
    if (temAlteracoes()) { ev.preventDefault(); ev.returnValue = ''; }
  });
  document.addEventListener('visibilitychange', () => { if (!document.hidden && SESSAO.usuario) pulsar(); });
  setInterval(() => { if (!document.hidden && !pulsando && SESSAO.usuario) pulsar(); }, 3000);

  navegar();
  pulsar().then(() => {
    if (ACESSO.pode('admin') && S.estado && !S.estado.salvo && !S.inicio.dispensado) abrirInicio();
  });
}

iniciar();
