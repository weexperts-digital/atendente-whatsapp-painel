'use strict';
// Campo de etiquetas: o que a pessoa digita vira uma etiqueta ao apertar Espaço,
// Vírgula, Tab ou Enter (ou ao sair do campo). Backspace com o campo vazio apaga a última.
//
// chips(['a', 'b'], {
//   rotulo: 'Etiquetas',            // nome para leitor de tela
//   placeholder: 'Digite e aperte Enter',
//   teclas: [' ', ',', 'Tab', 'Enter'],  // quais teclas fecham uma etiqueta
//   limpar: (texto) => texto ou '',  // normaliza; devolva '' para recusar
//   sugestoes: ['vip', 'orçamento'], // opcional, vira uma lista de sugestões
//   max: 12,
//   aoMudar: (lista) => {},
// })
// Devolve o elemento. el.valores() devolve a lista atual; el.definir(lista) troca tudo.
let contadorChips = 0;

function chips(inicial, opc) {
  opc = opc || {};
  const teclas = opc.teclas || [' ', ',', ';', 'Tab', 'Enter'];
  const limpar = opc.limpar || ((t) => t.trim());
  let valores = [];
  const caixa = h('div', { class: 'chips' });
  const entrada = h('input', {
    class: 'chips-entrada', type: 'text', autocomplete: 'off',
    placeholder: opc.placeholder || '', 'aria-label': opc.rotulo || 'Adicionar',
    inputMode: opc.inputMode,
  });
  if (opc.sugestoes && opc.sugestoes.length) {
    const idLista = 'chips-lista-' + (++contadorChips);
    entrada.setAttribute('list', idLista);
    caixa.appendChild(h('datalist', { id: idLista }, opc.sugestoes.map((s) => h('option', { value: s }))));
  }

  function desenhar() {
    caixa.querySelectorAll('.chip').forEach((c) => c.remove());
    valores.forEach((v, i) => {
      caixa.insertBefore(h('span', { class: 'chip' },
        h('span', { text: v }),
        h('button', {
          type: 'button', class: 'chip-x', 'aria-label': 'Remover ' + v,
          onclick: () => { valores.splice(i, 1); desenhar(); avisar(); entrada.focus(); },
        }, '×'),
      ), entrada);
    });
  }
  function avisar() { if (opc.aoMudar) opc.aoMudar(valores.slice()); }
  function fechar() {
    const partes = entrada.value.split(/[,;\n]/).map(limpar).filter(Boolean);
    entrada.value = '';
    let mudou = false;
    for (const p of partes) {
      if (valores.includes(p) || (opc.max && valores.length >= opc.max)) continue;
      valores.push(p);
      mudou = true;
    }
    if (mudou) { desenhar(); avisar(); }
    return mudou;
  }

  entrada.addEventListener('keydown', (e) => {
    if (teclas.includes(e.key) && entrada.value.trim()) {
      // Tab só segura o foco quando havia algo para virar etiqueta.
      e.preventDefault();
      fechar();
    } else if (e.key === 'Backspace' && !entrada.value && valores.length) {
      valores.pop();
      desenhar();
      avisar();
    }
  });
  entrada.addEventListener('blur', fechar);
  entrada.addEventListener('change', fechar);
  entrada.addEventListener('paste', () => setTimeout(() => { if (/[,;\n ]/.test(entrada.value)) fechar(); }, 0));
  caixa.addEventListener('click', (e) => { if (e.target === caixa) entrada.focus(); });

  caixa.appendChild(entrada);
  caixa.valores = () => { fechar(); return valores.slice(); };
  caixa.definir = (lista) => { valores = (lista || []).map(limpar).filter(Boolean); desenhar(); };
  caixa.definir(inicial);
  return caixa;
}
