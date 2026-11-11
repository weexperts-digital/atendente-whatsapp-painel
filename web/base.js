'use strict';
// Peças básicas do painel: montar a tela, falar com o programa, avisos e formulários.
// Regra de ouro: nada que vem da API entra na página como HTML. Só como texto.

// O tema é aplicado o mais cedo possível, para a tela não piscar.
// O claro é o padrão. O escuro só entra se a pessoa escolheu (fica em localStorage.tema).
(function () {
  let tema = 'light';
  try { tema = localStorage.getItem('tema') || 'light'; } catch (e) { /* sem armazenamento */ }
  document.documentElement.dataset.theme = tema === 'dark' ? 'dark' : 'light';
})();

const $ = (seletor, raiz) => (raiz || document).querySelector(seletor);

const PROPRIEDADES = new Set([
  'value', 'checked', 'disabled', 'hidden', 'selected', 'htmlFor', 'type', 'name', 'id',
  'placeholder', 'rows', 'min', 'max', 'step', 'required', 'href', 'title', 'tabIndex',
  'open', 'multiple', 'accept', 'src', 'alt', 'download', 'readOnly', 'maxLength',
  'autocomplete', 'inputMode', 'width', 'height', 'target', 'rel',
]);

function por(el, filhos) {
  for (const f of filhos) {
    if (f == null || f === false) continue;
    if (Array.isArray(f)) por(el, f);
    else if (f instanceof Node) el.appendChild(f);
    else el.appendChild(document.createTextNode(String(f)));
  }
  return el;
}

// h cria um elemento. Textos entram sempre como texto puro.
function h(tag, props, ...filhos) {
  const el = document.createElement(tag);
  if (props) {
    for (const [k, v] of Object.entries(props)) {
      if (v == null || v === false) continue;
      if (k === 'class') el.className = v;
      else if (k === 'text') el.textContent = v;
      else if (k.startsWith('on') && typeof v === 'function') el.addEventListener(k.slice(2), v);
      else if (PROPRIEDADES.has(k)) el[k] = v;
      else el.setAttribute(k, v === true ? '' : v);
    }
  }
  return por(el, filhos);
}

function trocar(el, ...filhos) {
  el.replaceChildren();
  return por(el, filhos);
}

const SVG = 'http://www.w3.org/2000/svg';
function icone(nome, classe) {
  const svg = document.createElementNS(SVG, 'svg');
  svg.setAttribute('class', 'ic' + (classe ? ' ' + classe : ''));
  svg.setAttribute('aria-hidden', 'true');
  svg.setAttribute('focusable', 'false');
  const use = document.createElementNS(SVG, 'use');
  use.setAttribute('href', '#i-' + nome);
  svg.appendChild(use);
  return svg;
}

// ---------- Conversa com o programa ----------

async function pedir(url, opcoes) {
  let resp;
  try {
    resp = await fetch(url, opcoes);
  } catch (e) {
    throw new Error('Não consegui falar com o atendente. Confira se ele está ligado neste computador.');
  }
  let dados = null;
  try { dados = await resp.json(); } catch (e) { /* resposta sem JSON */ }
  if (!resp.ok || (dados && dados.ok === false)) {
    throw new Error((dados && dados.erro) || 'Algo deu errado (código ' + resp.status + '). Tente de novo.');
  }
  return dados || {};
}

const api = {
  get: (url) => pedir(url, { cache: 'no-store' }),
  post: (url, corpo) => pedir(url, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(corpo || {}),
  }),
  arquivo: (arq) => pedir('/api/conhecimento/enviar', {
    method: 'POST',
    headers: { 'Content-Type': 'application/octet-stream', 'X-Nome': encodeURIComponent(arq.name) },
    body: arq,
  }),
};

// ---------- Movimento ----------
// Todo movimento do painel passa por animar(). Ela usa a biblioteca Motion (web/vendor/motion.js).
// Se a biblioteca não carregou, usa a animação do próprio navegador. Se a pessoa pediu
// "reduzir movimento" no sistema, não anima nada.

const CONSULTA_MOVIMENTO = window.matchMedia ? window.matchMedia('(prefers-reduced-motion: reduce)') : null;

function semMovimento() {
  return !!(CONSULTA_MOVIMENTO && CONSULTA_MOVIMENTO.matches);
}

function animacaoParada() {
  const pronta = Promise.resolve();
  return { finished: pronta, then: (ok, falha) => pronta.then(ok, falha), cancel() {}, stop() {}, complete() {} };
}

const TRANSFORMACOES = { x: (v) => 'translateX(' + v + 'px)', y: (v) => 'translateY(' + v + 'px)', scale: (v) => 'scale(' + v + ')', rotate: (v) => 'rotate(' + v + 'deg)' };
const CURVAS = { linear: 'linear', easeIn: 'ease-in', easeOut: 'ease-out', easeInOut: 'ease-in-out' };

function alvosDe(alvo) {
  if (!alvo) return [];
  if (typeof alvo === 'string') return Array.from(document.querySelectorAll(alvo));
  if (alvo instanceof Element) return [alvo];
  return Array.from(alvo).filter((el) => el instanceof Element);
}

// Depois de uma entrada, o elemento volta a obedecer só ao CSS (sem estilo preso nele).
function limparEstilos(els, quadros) {
  for (const el of els) {
    for (const k of Object.keys(quadros)) el.style.removeProperty(TRANSFORMACOES[k] ? 'transform' : k.replace(/[A-Z]/g, (l) => '-' + l.toLowerCase()));
    el.style.removeProperty('will-change');
    if (!el.getAttribute('style')) el.removeAttribute('style');
  }
}

// Caminho reserva, sem a biblioteca: traduz os quadros para a animação do navegador.
function animarNoNavegador(els, quadros, opc) {
  const chaves = Object.keys(quadros);
  const passos = Math.max(2, ...chaves.map((k) => (Array.isArray(quadros[k]) ? quadros[k].length : 1)));
  const lista = [];
  for (let i = 0; i < passos; i++) {
    const quadro = {};
    const partes = [];
    for (const k of chaves) {
      const valores = Array.isArray(quadros[k]) ? quadros[k] : [quadros[k]];
      const v = valores[Math.min(i, valores.length - 1)];
      if (TRANSFORMACOES[k]) partes.push(TRANSFORMACOES[k](v));
      else quadro[k] = v;
    }
    if (partes.length) quadro.transform = partes.join(' ');
    lista.push(quadro);
  }
  const curva = Array.isArray(opc.ease) ? 'cubic-bezier(' + opc.ease.join(',') + ')' : (CURVAS[opc.ease] || 'ease-out');
  const todas = els.map((el, i) => {
    const atraso = typeof opc.delay === 'function' ? opc.delay(i, els.length) : (opc.delay || 0);
    return el.animate(lista, { duration: (opc.duration == null ? 0.2 : opc.duration) * 1000, delay: atraso * 1000, easing: curva, fill: 'backwards' });
  });
  const fim = Promise.all(todas.map((a) => a.finished.catch(() => {})));
  return {
    finished: fim, then: (ok, falha) => fim.then(ok, falha),
    cancel: () => todas.forEach((a) => a.cancel()), stop: () => todas.forEach((a) => a.cancel()),
    complete: () => todas.forEach((a) => a.finish()),
  };
}

// animar(el, quadros, opcoes) move um elemento, uma lista de elementos ou um seletor.
//   quadros: { opacity: [0, 1], y: [8, 0] }  (x, y, scale, rotate ou qualquer propriedade de CSS)
//   opcoes:  { duration: 0.18, delay: 0, ease: 'easeOut', manter: false }  (tempos em segundos)
// Devolve o controle da animação (dá para usar .then() ou await). Ao terminar, tira do elemento
// os estilos que a animação deixou; passe manter: true quando o fim da animação é o estado final.
function animar(alvo, quadros, opcoes) {
  const els = alvosDe(alvo);
  const opc = Object.assign({ duration: 0.2, ease: 'easeOut' }, opcoes || {});
  const manter = opc.manter;
  delete opc.manter;
  if (!els.length || !quadros) return animacaoParada();
  if (semMovimento()) {
    if (manter) {
      for (const el of els) {
        for (const [k, v] of Object.entries(quadros)) {
          if (!TRANSFORMACOES[k]) el.style[k] = Array.isArray(v) ? v[v.length - 1] : v;
        }
      }
    }
    return animacaoParada();
  }
  let controle = null;
  try {
    if (window.Motion && typeof window.Motion.animate === 'function') controle = window.Motion.animate(els, quadros, opc);
    else if (typeof els[0].animate === 'function') controle = animarNoNavegador(els, quadros, opc);
  } catch (e) {
    console.error(e);
  }
  if (!controle) return animacaoParada();
  let terminou = false;
  const limpar = () => { if (!manter) limparEstilos(els, quadros); };
  // A biblioteca ainda escreve o estado final no elemento logo depois de terminar.
  // Por isso a limpeza passa de novo um pouco mais tarde.
  const depois = () => { terminou = true; limpar(); setTimeout(limpar, 80); setTimeout(limpar, 400); };
  Promise.resolve(controle).then(depois, depois);
  // Rede de segurança: se o navegador parou de desenhar (aba em segundo plano, por exemplo),
  // a animação é levada ao fim na marra. Nada fica invisível à espera de um quadro.
  const atraso = typeof opc.delay === 'function'
    ? Math.max(0, ...els.map((el, i) => Number(opc.delay(i, els.length)) || 0))
    : (Number(opc.delay) || 0);
  setTimeout(() => {
    if (terminou) return;
    try { controle.complete(); } catch (e) { /* segue para a limpeza */ }
    depois();
  }, ((Number(opc.duration) || 0.2) + atraso) * 1000 + 350);
  return controle;
}

// emSequencia dá o atraso de cada item de uma lista (em segundos), para usar em opcoes.delay.
function emSequencia(passo, maximo) {
  const p = passo == null ? 0.03 : passo;
  const teto = maximo == null ? 12 : maximo;
  return (i) => Math.min(i, teto) * p;
}

// entrarEmSequencia faz os itens de uma lista aparecerem um depois do outro.
// Use só no primeiro desenho da lista, nunca a cada atualização.
function entrarEmSequencia(alvo, opcoes) {
  return animar(alvo, { opacity: [0, 1], y: [8, 0] }, Object.assign({ duration: 0.22, delay: emSequencia() }, opcoes || {}));
}

// abrirDialogo abre um <dialog> por cima da tela, com uma entrada suave.
function abrirDialogo(dlg, opc) {
  if (!dlg.open) dlg.showModal();
  // Aviso que já estava na tela continua por cima da janela nova.
  const avisos = $('#toasts');
  if (avisos && avisos.children.length) mostrarCaixaDeAvisos(avisos);
  const cheio = opc && opc.telaCheia;
  return animar(dlg, cheio ? { opacity: [0, 1] } : { opacity: [0, 1], y: [10, 0], scale: [0.98, 1] }, { duration: 0.18 });
}

let contadorJanelas = 0;

// janela monta e abre uma caixa por cima da tela. Devolve { el, corpo, fechar }.
//   opc: { titulo, descricao, corpo (nó ou lista), pe (botões do rodapé), classe, fecharFora, aoFechar, podeFechar }
//   classe 'janela-larga' abre uma janela mais larga.
//   podeFechar: função (pode ser assíncrona) que devolve falso para segurar a janela aberta.
//   fechar(true) fecha sem perguntar.
function janela(opc) {
  opc = opc || {};
  const idTitulo = 'janela-titulo-' + (++contadorJanelas);
  const corpo = h('div', { class: 'janela-corpo' });
  const dlg = h('dialog', { class: 'janela' + (opc.classe ? ' ' + opc.classe : ''), 'aria-labelledby': idTitulo });
  let fechando = false;
  const fechar = async (forcar) => {
    if (!dlg.open || fechando) return;
    if (!forcar && opc.podeFechar) {
      fechando = true;
      let pode = true;
      try { pode = await opc.podeFechar(); } catch (e) { console.error(e); }
      fechando = false;
      if (pode === false) return;
    }
    if (dlg.open) dlg.close();
  };
  por(dlg, [h('div', { class: 'janela-caixa' },
    h('div', { class: 'janela-topo' },
      h('div', { class: 'janela-titulos' },
        h('h2', { id: idTitulo, text: opc.titulo || '' }),
        opc.descricao ? h('p', { class: 'descricao', text: opc.descricao }) : null),
      h('button', { type: 'button', class: 'btn-icone', 'aria-label': 'Fechar', onclick: () => fechar() }, icone('fechar'))),
    por(corpo, [opc.corpo]),
    opc.pe ? h('div', { class: 'janela-pe' }, opc.pe) : null)]);
  dlg.addEventListener('cancel', (ev) => { ev.preventDefault(); fechar(); });
  dlg.addEventListener('close', () => {
    dlg.remove();
    if (opc.aoFechar) opc.aoFechar();
  });
  if (opc.fecharFora) {
    // Só o fundo escuro fecha: o clique precisa começar e terminar fora da caixa.
    let fora = false;
    dlg.addEventListener('pointerdown', (ev) => { fora = ev.target === dlg; });
    dlg.addEventListener('click', (ev) => { if (fora && ev.target === dlg) fechar(); fora = false; });
  }
  document.body.appendChild(dlg);
  abrirDialogo(dlg);
  return { el: dlg, corpo, fechar };
}

// ---------- Avisos ----------

// Os avisos ficam na camada de cima do navegador, para aparecerem também sobre as janelas.
function mostrarCaixaDeAvisos(caixa) {
  if (typeof caixa.showPopover !== 'function') return;
  try {
    if (caixa.matches(':popover-open')) {
      // Se uma janela abriu depois, a caixa volta para cima dela.
      if (!document.querySelector('dialog[open]')) return;
      caixa.hidePopover();
    }
    caixa.showPopover();
  } catch (e) {
    // Se a camada de cima falhar, os avisos voltam a aparecer do jeito comum.
    caixa.removeAttribute('popover');
  }
}

function tirarToast(el) {
  if (el.dataset.saindo) return;
  el.dataset.saindo = '1';
  const fim = () => {
    const caixa = el.parentNode;
    el.remove();
    if (caixa && !caixa.children.length && typeof caixa.hidePopover === 'function') {
      try { if (caixa.matches(':popover-open')) caixa.hidePopover(); } catch (e) { /* tanto faz */ }
    }
  };
  animar(el, { opacity: [1, 0], y: [0, -8] }, { duration: 0.15, ease: 'easeIn', manter: true }).then(fim, fim);
}

function toast(texto, tipo) {
  const caixa = $('#toasts');
  if (!caixa || !texto) return;
  const erro = tipo === 'erro';
  const el = h('div', { class: 'toast' + (erro ? ' toast-erro' : ''), role: erro ? 'alert' : 'status' },
    icone(erro ? 'alerta' : 'check'),
    h('span', { class: 'toast-texto', text: texto }),
    h('button', { type: 'button', class: 'btn-icone', 'aria-label': 'Fechar aviso', onclick: () => tirarToast(el) }, icone('fechar')),
  );
  caixa.appendChild(el);
  mostrarCaixaDeAvisos(caixa);
  animar(el, { opacity: [0, 1], y: [-12, 0] }, { duration: 0.2 });
  setTimeout(() => tirarToast(el), erro ? 7000 : 4000);
}

// confirmar abre uma janela de confirmação e devolve verdadeiro ou falso.
//   opc: { titulo, texto, botao (o que confirma), cancelar (o que desiste, padrão "Cancelar"), perigo }
function confirmar(opc) {
  return new Promise((resolver) => {
    const dlg = $('#dlg-confirmar');
    $('#dlg-titulo').textContent = opc.titulo || 'Tem certeza?';
    $('#dlg-texto').textContent = opc.texto || '';
    const sim = $('#dlg-sim');
    sim.textContent = opc.botao || 'Confirmar';
    sim.className = 'btn ' + (opc.perigo ? 'btn-perigo' : 'btn-primario');
    const nao = $('#dlg-nao');
    if (nao) nao.textContent = opc.cancelar || 'Cancelar';
    const fechar = () => {
      dlg.removeEventListener('close', fechar);
      resolver(dlg.returnValue === 'sim');
    };
    dlg.returnValue = '';
    dlg.addEventListener('close', fechar);
    abrirDialogo(dlg);
  });
}

// ocupar trava o botão enquanto a ação roda e mostra o erro, se houver.
async function ocupar(btn, acao) {
  if (btn.disabled) return undefined;
  btn.disabled = true;
  btn.classList.add('ocupado');
  btn.setAttribute('aria-busy', 'true');
  try {
    return await acao();
  } catch (e) {
    toast(e.message || 'Algo deu errado. Tente de novo.', 'erro');
    return undefined;
  } finally {
    btn.disabled = false;
    btn.classList.remove('ocupado');
    btn.removeAttribute('aria-busy');
  }
}

function botao(texto, opc) {
  opc = opc || {};
  const b = h('button', {
    type: opc.tipo || 'button',
    class: 'btn ' + (opc.classe || 'btn-suave'),
    title: opc.dica,
    'aria-label': opc.rotulo,
    disabled: opc.desligado,
  }, opc.icone ? icone(opc.icone) : null, texto ? h('span', { text: texto }) : null);
  if (opc.aoClicar) b.addEventListener('click', () => ocupar(b, opc.aoClicar));
  return b;
}

// ---------- Formulários ----------

let contadorCampos = 0;

// campo junta rótulo, controle e texto de ajuda.
function campo(rotulo, controle, ajuda) {
  if (!controle.id) controle.id = 'c' + (++contadorCampos);
  const el = h('div', { class: 'campo' }, h('label', { htmlFor: controle.id, text: rotulo }), controle);
  if (ajuda) {
    const idAjuda = controle.id + '-ajuda';
    controle.setAttribute('aria-describedby', idAjuda);
    el.appendChild(h('p', { class: 'ajuda', id: idAjuda, text: ajuda }));
  }
  return el;
}

// entrada cria um campo de texto ligado a obj[chave].
function entrada(obj, chave, attrs, aoMudar) {
  const numero = attrs && attrs.type === 'number';
  const el = h('input', Object.assign({ type: 'text', value: obj[chave] == null ? '' : String(obj[chave]) }, attrs || {}));
  el.addEventListener('input', () => {
    obj[chave] = numero ? (el.value === '' ? 0 : Number(el.value)) : el.value;
    if (aoMudar) aoMudar();
  });
  return el;
}

function areaTexto(obj, chave, attrs, aoMudar) {
  const el = h('textarea', Object.assign({ rows: 4 }, attrs || {}));
  el.value = obj[chave] == null ? '' : String(obj[chave]);
  el.addEventListener('input', () => {
    obj[chave] = el.value;
    if (aoMudar) aoMudar();
  });
  return el;
}

// seletor cria um <select>. opcoes: [{ valor, texto }].
function seletor(obj, chave, opcoes, aoMudar, numero) {
  const el = h('select', null, opcoes.map((o) => h('option', { value: String(o.valor), text: o.texto })));
  el.value = String(obj[chave] == null ? '' : obj[chave]);
  el.addEventListener('change', () => {
    obj[chave] = numero ? Number(el.value) : el.value;
    if (aoMudar) aoMudar();
  });
  return el;
}

// chave cria um interruptor de ligar e desligar.
function chave(rotulo, ligado, aoMudar, opc) {
  opc = opc || {};
  const b = h('button', {
    type: 'button', class: 'chave' + (opc.classe ? ' ' + opc.classe : ''), role: 'switch',
    'aria-checked': ligado ? 'true' : 'false', 'aria-label': opc.escondido ? rotulo : null,
  },
    h('span', { class: 'chave-trilho' }, h('span', { class: 'chave-bola' })),
    opc.escondido ? null : h('span', { class: 'chave-texto', text: rotulo }),
  );
  b.addEventListener('click', () => {
    const novo = b.getAttribute('aria-checked') !== 'true';
    if (opc.manual) { aoMudar(novo, b); return; }
    b.setAttribute('aria-checked', novo ? 'true' : 'false');
    aoMudar(novo, b);
  });
  return b;
}

// ---------- Datas, números e rótulos ----------

const DIAS_CURTOS = ['dom', 'seg', 'ter', 'qua', 'qui', 'sex', 'sáb'];
const DIAS_LONGOS = ['Domingo', 'Segunda', 'Terça', 'Quarta', 'Quinta', 'Sexta', 'Sábado'];

function doisDigitos(n) { return String(n).padStart(2, '0'); }

function horaCurta(iso) {
  const d = new Date(iso);
  if (isNaN(d)) return '';
  return doisDigitos(d.getHours()) + ':' + doisDigitos(d.getMinutes());
}

function mesmoDia(a, b) {
  return a.getFullYear() === b.getFullYear() && a.getMonth() === b.getMonth() && a.getDate() === b.getDate();
}

// diaRotulo: Hoje, Ontem, Amanhã ou "qua, 12/11".
function diaRotulo(iso) {
  const d = new Date(iso);
  if (isNaN(d)) return '';
  const hoje = new Date();
  const outro = new Date(hoje);
  if (mesmoDia(d, hoje)) return 'Hoje';
  outro.setDate(hoje.getDate() - 1);
  if (mesmoDia(d, outro)) return 'Ontem';
  outro.setDate(hoje.getDate() + 1);
  if (mesmoDia(d, outro)) return 'Amanhã';
  return DIAS_CURTOS[d.getDay()] + ', ' + doisDigitos(d.getDate()) + '/' + doisDigitos(d.getMonth() + 1);
}

// tempoRelativo: agora, há 5 min, há 2 h, ontem, 12/11.
function tempoRelativo(iso) {
  const d = new Date(iso);
  if (isNaN(d) || d.getFullYear() < 2000) return '';
  const seg = Math.round((Date.now() - d.getTime()) / 1000);
  if (seg < 45) return 'agora';
  if (seg < 3600) return 'há ' + Math.max(1, Math.round(seg / 60)) + ' min';
  if (mesmoDia(d, new Date())) return 'há ' + Math.round(seg / 3600) + ' h';
  const rotulo = diaRotulo(iso);
  return rotulo === 'Ontem' ? 'ontem' : doisDigitos(d.getDate()) + '/' + doisDigitos(d.getMonth() + 1);
}

function tamanhoBonito(bytes) {
  const n = Number(bytes) || 0;
  if (n < 1024) return n + ' B';
  if (n < 1024 * 1024) return Math.round(n / 1024) + ' KB';
  return (n / 1024 / 1024).toFixed(1).replace('.', ',') + ' MB';
}

const TEMPERATURAS = { quente: 'Quente', morno: 'Morno', frio: 'Frio', indefinido: 'Sem classificação' };

function temperatura(classe) {
  return TEMPERATURAS[classe] ? classe : 'indefinido';
}

// etiquetaTemp mostra a temperatura do lead em cor e em texto.
function etiquetaTemp(classe) {
  const t = temperatura(classe);
  return h('span', { class: 'etiqueta temp-' + t }, icone('termometro'), TEMPERATURAS[t]);
}

function rotuloStatus(status) {
  if (status === 'em atendimento') return 'Em atendimento';
  if (status === 'aguardando humano') return 'Aguardando você';
  if (status === 'qualificado') return 'Qualificado';
  return 'Com o assistente';
}

function copiar(obj) {
  return JSON.parse(JSON.stringify(obj == null ? null : obj));
}

function cortarTexto(s, max) {
  s = String(s == null ? '' : s);
  return s.length > max ? s.slice(0, max - 1) + '…' : s;
}

// definicao monta um par "rótulo: valor". Valor vazio vira um aviso discreto.
function definicao(rotulo, valor, falta) {
  const tem = valor != null && String(valor).trim() !== '';
  return [
    h('dt', { text: rotulo }),
    h('dd', { class: tem ? null : 'falta', text: tem ? String(valor) : (falta || 'Ainda não disse') }),
  ];
}

function carregandoEl(texto) {
  return h('div', { class: 'carregando' }, h('span', { class: 'rodando' }), texto || 'Carregando...');
}

// falhaCarga mostra o erro de carregamento com um botão para tentar de novo.
function falhaCarga(erro, tentar) {
  return h('div', { class: 'cartao' }, vazio('alerta', 'Não consegui carregar esta parte',
    (erro && erro.message) || 'Tente de novo em alguns segundos.',
    botao('Tentar de novo', { classe: 'btn-primario', aoClicar: tentar })));
}

// ---------- Memória da página ----------
// S guarda o que já foi carregado e os rascunhos, para nada se perder ao trocar de aba.

const S = {
  estado: null,
  semConexao: false,
  rota: { aba: 'conversas', sub: '' },

  filtro: 'todas',
  aberto: null,
  lead: null,
  agendamentosLead: [],

  config: null,
  variaveis: [],
  modelos: [],
  tons: [],

  textos: null,
  editores: {},

  negocio: null,
  negocioSujo: false,

  agenda: null,
  regras: null,
  regrasSujas: false,
  regrasAbertas: false,
  marcar: null,
  agendaVisao: '',        // visão aberta no calendário (semana, dia, mês ou lista)
  agendaData: null,       // dia em que a pessoa parou no calendário
  agendaCancelados: false,

  materiais: null,
  formatos: ['pdf', 'docx', 'pptx', 'txt', 'md', 'csv'],
  limiteMb: 25,

  treino: { mensagens: [], lead: {}, ultimo: null, esperando: false, rodada: 0 },

  ajustes: null,
  ajustesSujos: false,

  inicio: { passo: 0, dados: null, descricao: '', dispensado: false },
};

// TELAS guarda cada aba: { montar(main, sub), atualizar(batidas), sair() }.
const TELAS = {};

// vazio monta a mensagem de quando ainda não há nada para mostrar.
function vazio(nomeIcone, titulo, texto, ...acoes) {
  return h('div', { class: 'vazio' },
    icone(nomeIcone, 'ic-grande'),
    h('h2', { text: titulo }),
    texto ? h('p', { text: texto }) : null,
    acoes.length ? h('div', { class: 'vazio-acoes' }, acoes) : null,
  );
}
