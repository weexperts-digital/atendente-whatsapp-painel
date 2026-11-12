'use strict';
// Lista suspensa do painel: substitui o <select> do navegador e serve de base para os menus.
//
//   suspenso(gatilho, montar, opc)  um botão que abre um painel flutuante
//   listaSuspensa(opc)              o substituto do <select>, de escolha única
//
// O painel abre na camada de cima do navegador (Popover API). Por isso ele aparece também
// por cima de uma janela aberta com showModal(). Dentro do painel, todo elemento com o
// atributo data-sus-item é um item: as setas, Home, End e as letras andam por eles, e
// Enter ou espaço clicam no item ativo. Esc fecha e devolve o foco ao botão. Tab fecha.
// Regra de ouro: nada entra como HTML. Todo texto entra como texto.

const SUS_MARGEM = 8;    // distância mínima entre o painel e a borda da tela
const SUS_VAO = 6;       // espaço entre o botão e o painel
const SUS_ALTURA = 420;  // altura máxima do painel
let susContador = 0;
let susAberto = null;    // só um painel fica aberto de cada vez

function susDobrar(s) {
  return String(s == null ? '' : s).toLowerCase().normalize('NFD').replace(/[̀-ͯ]/g, '');
}

// Peça de baixo nível: um botão que abre um painel flutuante.
//   gatilho: elemento <button>
//   montar(painel, fechar): preenche o painel a cada abertura
//   opc: {
//     alinhar: 'inicio' (padrão) ou 'fim': a borda do botão em que o painel encosta
//     largura: 'gatilho' (igual à do botão) ou um número em px. Sem isso, o painel tem
//              a largura do conteúdo e nunca fica mais estreito que o botão.
//     aoFechar: função chamada depois de fechar
//     papel: valor de aria-haspopup ('listbox' é o padrão; 'menu' ou 'dialog')
//     altura: altura máxima do painel em px (o padrão é 420). Passando disso, ele rola.
//     ancora: elemento que dá a posição e a largura, quando não é o próprio botão
//     classe: classe a mais para o painel
//     foco: 'painel' (padrão: o foco anda pelos itens) ou 'gatilho' (o foco fica no botão
//           e o item ativo é dito por aria-activedescendant, como em um <select>)
//   }
// Devolve { abrir(), fechar(), aberto(), refazer(), posicionar(), painel }.
function suspenso(gatilho, montar, opc) {
  opc = opc || {};
  const noGatilho = opc.foco === 'gatilho';
  const painel = h('div', {
    class: 'sus-painel' + (opc.classe ? ' ' + opc.classe : ''), id: 'sus-' + (++susContador), tabindex: '-1',
  });
  let naCamada = typeof painel.showPopover === 'function';
  if (naCamada) painel.setAttribute('popover', 'manual');

  let aberto = false;
  let ativo = null;
  let quadro = 0;
  let letras = '';
  let relogioLetras = 0;
  let observador = null;
  let engolirEspaco = false;
  let itensCriados = 0;

  gatilho.setAttribute('aria-haspopup', opc.papel || 'listbox');
  gatilho.setAttribute('aria-expanded', 'false');

  function itens() {
    return Array.from(painel.querySelectorAll('[data-sus-item]'))
      .filter((el) => el.getAttribute('aria-disabled') !== 'true' && !el.disabled);
  }

  // rolarAte mexe só na rolagem do painel. A página fica onde está.
  function rolarAte(item) {
    const caixa = item.closest('.sus-rolagem') || painel;
    const lista = itens();
    // Na ponta da lista, a rolagem vai até o fim: o título do grupo também aparece.
    if (item === lista[0]) { caixa.scrollTop = 0; return; }
    if (item === lista[lista.length - 1]) { caixa.scrollTop = caixa.scrollHeight; return; }
    const c = caixa.getBoundingClientRect();
    const i = item.getBoundingClientRect();
    if (i.top < c.top + 4) caixa.scrollTop -= c.top + 4 - i.top;
    else if (i.bottom > c.bottom - 4) caixa.scrollTop += i.bottom - (c.bottom - 4);
  }

  function ativar(item, semRolar) {
    if (ativo && ativo !== item) ativo.classList.remove('sus-ativo');
    ativo = item || null;
    if (!ativo) {
      gatilho.removeAttribute('aria-activedescendant');
      return;
    }
    ativo.classList.add('sus-ativo');
    if (!semRolar) rolarAte(ativo);
    if (noGatilho) {
      if (!ativo.id) ativo.id = painel.id + '-i' + (++itensCriados);
      gatilho.setAttribute('aria-activedescendant', ativo.id);
    } else if (document.activeElement !== ativo) {
      if (!ativo.hasAttribute('tabindex')) ativo.tabIndex = -1;
      ativo.focus({ preventScroll: true });
    }
  }

  function itemInicial() {
    const lista = itens();
    return lista.find((el) => el.hasAttribute('data-sus-inicial')) ||
      lista.find((el) => el.getAttribute('aria-selected') === 'true' || el.getAttribute('aria-checked') === 'true') ||
      lista[0] || null;
  }

  // posicionar põe o painel embaixo do botão. Se não couber, vira para cima. Nunca sai da tela.
  function posicionar() {
    if (!aberto) return;
    if (!gatilho.isConnected) { fechar(); return; }
    const r = (opc.ancora && opc.ancora.isConnected ? opc.ancora : gatilho).getBoundingClientRect();
    const larguraTela = document.documentElement.clientWidth;
    const alturaTela = window.innerHeight;
    // Botão escondido, ou que saiu da tela ao rolar: o painel fecha.
    if ((!r.width && !r.height) || r.bottom < 0 || r.top > alturaTela || r.right < 0 || r.left > larguraTela) { fechar(); return; }

    const s = painel.style;
    const maximo = Math.max(120, larguraTela - 2 * SUS_MARGEM);
    s.maxWidth = maximo + 'px';
    s.minWidth = Math.min(r.width, maximo) + 'px';
    if (opc.largura === 'gatilho') s.width = Math.min(r.width, maximo) + 'px';
    else if (typeof opc.largura === 'number') s.width = Math.min(opc.largura, maximo) + 'px';

    // Mede a altura natural sem perder a rolagem de quem já estava rolando.
    const caixas = [painel].concat(Array.from(painel.querySelectorAll('.sus-rolagem')));
    const rolagens = caixas.map((c) => c.scrollTop);
    s.maxHeight = 'none';
    const natural = Math.min(painel.offsetHeight, opc.altura || SUS_ALTURA);
    const abaixo = alturaTela - r.bottom - SUS_VAO - SUS_MARGEM;
    const acima = r.top - SUS_VAO - SUS_MARGEM;
    const paraCima = natural > abaixo && acima > abaixo;
    const altura = Math.max(Math.min(natural, paraCima ? acima : abaixo), Math.min(natural, 120));
    s.maxHeight = altura + 'px';
    caixas.forEach((c, i) => { if (c.scrollTop !== rolagens[i]) c.scrollTop = rolagens[i]; });

    const largura = painel.offsetWidth;
    let topo = paraCima ? r.top - SUS_VAO - altura : r.bottom + SUS_VAO;
    topo = Math.max(SUS_MARGEM, Math.min(topo, alturaTela - SUS_MARGEM - altura));
    let esquerda = opc.alinhar === 'fim' ? r.right - largura : r.left;
    esquerda = Math.max(SUS_MARGEM, Math.min(esquerda, larguraTela - SUS_MARGEM - largura));
    s.top = Math.round(topo) + 'px';
    s.left = Math.round(esquerda) + 'px';
    painel.dataset.lado = paraCima ? 'cima' : 'baixo';
  }

  function agendar() {
    if (quadro) return;
    quadro = requestAnimationFrame(() => { quadro = 0; posicionar(); });
  }

  // procurar pula para o item que começa com o que a pessoa digitou.
  function procurar(letra) {
    clearTimeout(relogioLetras);
    relogioLetras = setTimeout(() => { letras = ''; }, 500);
    letras += susDobrar(letra);
    const lista = itens();
    if (!lista.length) return;
    // A mesma letra repetida passa pelos itens que começam com ela.
    const repetida = letras.split('').every((c) => c === letras[0]);
    const termo = repetida ? letras[0] : letras;
    const de = Math.max(0, lista.indexOf(ativo) + (repetida ? 1 : 0));
    const ordem = lista.slice(de).concat(lista.slice(0, de));
    const achou = ordem.find((el) => susDobrar(el.dataset.susTexto || el.textContent).trim().startsWith(termo));
    if (achou) ativar(achou);
  }

  function aoTeclar(ev) {
    if (!aberto || ev.defaultPrevented) return;
    const alvo = ev.target instanceof Element ? ev.target : null;
    if (ev.key === 'Escape') {
      ev.preventDefault();
      ev.stopPropagation();
      fechar(true);
      return;
    }
    if (ev.key === 'Tab') { fechar(true); return; }
    // Campo de texto dentro do painel fica com as suas próprias teclas.
    if (alvo && alvo.matches('input, textarea, select, [contenteditable="true"]')) return;
    const lista = itens();
    const i = lista.indexOf(ativo);
    const ir = (n) => {
      ev.preventDefault();
      if (lista.length) ativar(lista[Math.max(0, Math.min(lista.length - 1, n))]);
    };
    switch (ev.key) {
      case 'ArrowDown': ir(i + 1); break;
      case 'ArrowUp': ir(i < 0 ? lista.length - 1 : i - 1); break;
      case 'Home': ir(0); break;
      case 'End': ir(lista.length - 1); break;
      case 'PageDown': ir(i + 8); break;
      case 'PageUp': ir(i < 0 ? 0 : i - 8); break;
      case 'Enter':
      case ' ':
        if (ev.key === ' ' && letras) { ev.preventDefault(); procurar(' '); break; }
        // Botão comum dentro do painel (que não é item) segue com o seu próprio clique.
        if (alvo && alvo !== gatilho && !alvo.closest('[data-sus-item]') && alvo.closest('button, a[href]')) return;
        // Link com o foco: o Enter do navegador já abre o link (ou baixa o arquivo).
        if (ev.key === 'Enter' && ativo && ativo === alvo && ativo.matches('a[href]')) return;
        ev.preventDefault();
        if (ev.key === ' ') engolirEspaco = true;
        if (ativo && ativo.isConnected) ativo.click();
        break;
      default:
        if (ev.key.length === 1 && !ev.ctrlKey && !ev.metaKey && !ev.altKey) {
          ev.preventDefault();
          procurar(ev.key);
        }
    }
  }

  function aoApontarFora(ev) {
    const alvo = ev.target;
    if (!(alvo instanceof Node)) return;
    if (painel.contains(alvo) || gatilho.contains(alvo)) return;
    // O rótulo do campo conta como o próprio botão.
    if (gatilho.labels && Array.from(gatilho.labels).some((rotulo) => rotulo.contains(alvo))) return;
    fechar();
  }

  function aoRolar(ev) {
    if (ev.target instanceof Node && painel.contains(ev.target)) return;
    agendar();
  }

  function aoSairDoFoco(ev) {
    const para = ev.relatedTarget;
    if (!aberto || !(para instanceof Node)) return;
    if (painel.contains(para) || gatilho.contains(para)) return;
    fechar();
  }

  function aoPassar(ev) {
    const item = ev.target instanceof Element ? ev.target.closest('[data-sus-item]') : null;
    if (item && item !== ativo && painel.contains(item) && itens().includes(item)) ativar(item, true);
  }

  // Com o foco no botão, clicar no painel não tira o foco de lá.
  function segurarFoco(ev) { ev.preventDefault(); }

  function ligar(sim) {
    const m = sim ? 'addEventListener' : 'removeEventListener';
    document[m]('pointerdown', aoApontarFora, true);
    window[m]('scroll', aoRolar, true);
    window[m]('resize', agendar);
    window[m]('hashchange', fechar);
    painel[m]('keydown', aoTeclar);
    painel[m]('pointermove', aoPassar);
    painel[m]('focusout', aoSairDoFoco);
    gatilho[m]('focusout', aoSairDoFoco);
    if (noGatilho) painel[m]('mousedown', segurarFoco);
    if (sim && window.ResizeObserver) {
      observador = new ResizeObserver(agendar);
      observador.observe(gatilho);
      observador.observe(painel);
      if (opc.ancora) observador.observe(opc.ancora);
    } else if (observador) {
      observador.disconnect();
      observador = null;
    }
  }

  function preencher() {
    painel.replaceChildren();
    ativo = null;
    montar(painel, fechar);
  }

  function abrir() {
    if (aberto || gatilho.disabled || !gatilho.isConnected) return;
    if (susAberto) susAberto.fechar();
    preencher();
    // Dentro de uma janela, o painel precisa morar nela: fora, ele não recebe cliques.
    (gatilho.closest('dialog') || document.body).appendChild(painel);
    if (naCamada) {
      try {
        painel.showPopover();
      } catch (e) {
        // Sem a camada de cima, o painel abre do jeito comum (posição fixa).
        naCamada = false;
        painel.removeAttribute('popover');
      }
    }
    aberto = true;
    susAberto = controle;
    gatilho.setAttribute('aria-expanded', 'true');
    gatilho.setAttribute('aria-controls', painel.id);
    posicionar();
    if (!aberto) return; // o botão não estava à vista
    ligar(true);
    // Há navegador que não põe o foco no botão clicado: as teclas precisam chegar aqui.
    if (noGatilho && document.activeElement !== gatilho) gatilho.focus({ preventScroll: true });
    const inicial = itemInicial();
    if (inicial) ativar(inicial);
    else if (!noGatilho) painel.focus({ preventScroll: true });
    animar(painel, { opacity: [0, 1], y: [painel.dataset.lado === 'cima' ? 4 : -4, 0] }, { duration: 0.14 });
    if (opc.aoAbrir) opc.aoAbrir();
  }

  // fechar(true) devolve o foco ao botão. Sem isso, o foco só volta se estava dentro do painel.
  function fechar(comFoco) {
    if (!aberto) return;
    aberto = false;
    ligar(false);
    if (quadro) { cancelAnimationFrame(quadro); quadro = 0; }
    clearTimeout(relogioLetras);
    letras = '';
    const focoDentro = painel.contains(document.activeElement);
    if ((comFoco === true || focoDentro) && gatilho.isConnected) gatilho.focus({ preventScroll: true });
    try {
      if (naCamada && painel.matches(':popover-open')) painel.hidePopover();
    } catch (e) { /* já estava fechado */ }
    painel.remove();
    ativo = null;
    gatilho.setAttribute('aria-expanded', 'false');
    gatilho.removeAttribute('aria-controls');
    gatilho.removeAttribute('aria-activedescendant');
    if (susAberto === controle) susAberto = null;
    if (opc.aoFechar) opc.aoFechar();
  }

  // refazer monta o conteúdo de novo com o painel aberto (as opções mudaram, por exemplo).
  function refazer() {
    if (!aberto) return;
    const texto = ativo ? (ativo.dataset.susTexto || ativo.textContent) : null;
    preencher();
    posicionar();
    if (!aberto) return;
    const lista = itens();
    const mesmo = texto == null ? null : lista.find((el) => (el.dataset.susTexto || el.textContent) === texto);
    const proximo = mesmo || itemInicial();
    if (proximo) ativar(proximo);
    else if (!noGatilho) painel.focus({ preventScroll: true });
  }

  gatilho.addEventListener('click', () => { if (aberto) fechar(); else abrir(); });
  gatilho.addEventListener('keydown', (ev) => {
    if (aberto) {
      // Com o foco no painel, a tecla já passou por lá.
      if (noGatilho) aoTeclar(ev);
      else if (ev.key === 'Escape') { ev.preventDefault(); ev.stopPropagation(); fechar(true); }
      return;
    }
    if (ev.ctrlKey || ev.metaKey || ev.altKey) return;
    if (ev.key === 'ArrowDown' || ev.key === 'ArrowUp') {
      ev.preventDefault();
      abrir();
    } else if (noGatilho && (ev.key === 'Home' || ev.key === 'End')) {
      ev.preventDefault();
      abrir();
      const lista = itens();
      if (aberto && lista.length) ativar(lista[ev.key === 'Home' ? 0 : lista.length - 1]);
    } else if (noGatilho && ev.key.length === 1 && ev.key !== ' ') {
      // Digitar com a lista fechada abre e já pula para a opção.
      ev.preventDefault();
      abrir();
      if (aberto) procurar(ev.key);
    }
    // Enter e espaço: o clique do próprio botão abre.
  });
  // O espaço que escolheu um item não pode virar um clique no botão ao soltar a tecla.
  gatilho.addEventListener('keyup', (ev) => {
    if (ev.key === ' ' && engolirEspaco) ev.preventDefault();
    engolirEspaco = false;
  });

  const controle = { abrir, fechar, aberto: () => aberto, refazer, posicionar, painel };
  return controle;
}

// susGrupos aceita uma lista de opções ou uma lista de grupos e devolve sempre grupos.
function susGrupos(lista) {
  const itens = Array.isArray(lista) ? lista.filter(Boolean) : [];
  if (itens.some((x) => Array.isArray(x.opcoes))) {
    return itens.map((g) => ({ titulo: g.titulo || '', opcoes: (g.opcoes || []).filter(Boolean) }));
  }
  return [{ titulo: '', opcoes: itens }];
}

// susItem monta uma linha de opção: marca de escolhida, ponto de cor, texto, dica e contagem.
//   o: { texto, dica, conta, cor }. Com guardarPonto, a opção sem cor deixa o lugar do ponto
//   vazio, para o texto ficar alinhado com o das vizinhas que têm cor.
//   Devolve { el, marcar(sim), contar(n) }.
function susItem(o, attrs, guardarPonto) {
  const marca = h('span', { class: 'sus-marca' });
  const conta = h('span', { class: 'sus-conta', hidden: true });
  let ponto = null;
  if (o.cor) {
    ponto = h('span', { class: 'sus-ponto' });
    ponto.style.backgroundColor = o.cor;
  } else if (guardarPonto) {
    ponto = h('span', { class: 'sus-ponto sus-ponto-vazio' });
  }
  const el = h('div', Object.assign({ class: 'sus-item', 'data-sus-item': '', 'data-sus-texto': o.texto }, attrs || {}),
    marca, ponto,
    h('span', { class: 'sus-textos' },
      h('span', { class: 'sus-texto', text: o.texto }),
      o.dica ? h('span', { class: 'sus-dica', text: o.dica }) : null),
    conta);
  const peca = {
    el,
    marcar(sim) {
      el.setAttribute('aria-selected', sim ? 'true' : 'false');
      if (sim && !marca.firstChild) marca.appendChild(icone('check'));
      else if (!sim && marca.firstChild) marca.replaceChildren();
    },
    contar(n) {
      const tem = n != null && n !== '';
      conta.hidden = !tem;
      const texto = tem ? String(n) : '';
      if (conta.textContent !== texto) conta.textContent = texto;
      el.classList.toggle('sus-zero', tem && Number(n) === 0);
    },
  };
  peca.contar(o.conta);
  return peca;
}

// Substituto do <select>, de escolha única.
//   opc: {
//     rotulo: nome para leitores de tela (aria-label). Dentro de campo() o <label> já serve.
//     opcoes: [{ valor, texto, dica?, conta?, cor? }]  ou  grupos: [{ titulo, opcoes }]
//     valor: valor escolhido
//     aoMudar(valor): chamada quando a pessoa escolhe outra opção
//     vazio: texto do botão quando nada está escolhido
//     compacto: botão mais baixo, para barras de filtro
//     alinhar, largura: iguais aos de suspenso()
//   }
// Devolve o botão, já montado, com: el.valor (ler e gravar), el.definirOpcoes(opcoesOuGrupos)
// e el.desativar(sim).
function listaSuspensa(opc) {
  opc = opc || {};
  let grupos = susGrupos(opc.grupos || opc.opcoes);
  let valor = opc.valor;

  const ponto = h('span', { class: 'sus-ponto', hidden: true });
  const texto = h('span', { class: 'sus-valor' });
  const gatilho = h('button', {
    type: 'button', class: 'sus-gatilho' + (opc.compacto ? ' sus-compacto' : ''),
    role: 'combobox', 'aria-label': opc.rotulo,
  }, ponto, texto, icone('seta-baixo', 'sus-seta'));

  const igual = (a, b) => String(a == null ? '' : a) === String(b == null ? '' : b);
  const todas = () => grupos.reduce((lista, g) => lista.concat(g.opcoes), []);
  const escolhida = () => todas().find((o) => igual(o.valor, valor)) || null;

  function pintarGatilho() {
    const o = escolhida();
    texto.textContent = o ? o.texto : (opc.vazio || 'Escolha');
    gatilho.classList.toggle('sus-sem-valor', !o);
    ponto.hidden = !(o && o.cor);
    if (o && o.cor) ponto.style.backgroundColor = o.cor;
  }

  function escolher(o) {
    if (igual(o.valor, valor)) return;
    valor = o.valor;
    pintarGatilho();
    if (opc.aoMudar) opc.aoMudar(valor);
  }

  function montar(painel, fechar) {
    painel.setAttribute('role', 'listbox');
    const nome = opc.rotulo || gatilho.getAttribute('aria-label') ||
      (gatilho.labels && gatilho.labels[0] ? gatilho.labels[0].textContent : '');
    if (nome) painel.setAttribute('aria-label', nome);
    else painel.removeAttribute('aria-label');
    for (const g of grupos) {
      const comCor = g.opcoes.some((o) => o.cor);
      const caixa = g.titulo
        ? h('div', { class: 'sus-grupo', role: 'group', 'aria-label': g.titulo },
          h('div', { class: 'sus-titulo', 'aria-hidden': 'true', text: g.titulo }))
        : painel;
      for (const o of g.opcoes) {
        const item = susItem(o, { role: 'option', 'aria-disabled': o.desativada ? 'true' : null }, comCor);
        item.marcar(igual(o.valor, valor));
        item.el.addEventListener('click', () => {
          if (o.desativada) return;
          fechar(true);
          escolher(o);
        });
        caixa.appendChild(item.el);
      }
      if (caixa !== painel) painel.appendChild(caixa);
    }
    if (!todas().length) painel.appendChild(h('p', { class: 'sus-nada', text: 'Nada para escolher.' }));
  }

  const controle = suspenso(gatilho, montar, { foco: 'gatilho', papel: 'listbox', alinhar: opc.alinhar, largura: opc.largura });

  Object.defineProperty(gatilho, 'valor', {
    get: () => valor,
    set: (novo) => { valor = novo; pintarGatilho(); controle.refazer(); },
  });
  gatilho.definirOpcoes = (lista) => {
    grupos = susGrupos(lista);
    pintarGatilho();
    controle.refazer();
  };
  gatilho.desativar = (sim) => {
    gatilho.disabled = !!sim;
    if (sim) controle.fechar();
  };
  gatilho.abrir = controle.abrir;
  gatilho.fechar = controle.fechar;

  pintarGatilho();
  return gatilho;
}
