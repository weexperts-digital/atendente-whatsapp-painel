'use strict';

// Editores de dias, horários e serviços usados na configuração inicial.
// São os mesmos dados das Regras da agenda: o que a pessoa informa aqui é o que o
// calendário mostra e o que o assistente usa para oferecer e marcar horários.

const ORDEM_DA_SEMANA = [1, 2, 3, 4, 5, 6, 0]; // de segunda a domingo

// semanaDe devolve os 7 dias prontos para editar, a partir do que já está salvo.
function semanaDe(dias) {
  const semana = [];
  for (let d = 0; d < 7; d++) {
    const atual = (Array.isArray(dias) ? dias : []).find((x) => x && Number(x.dia) === d) || {};
    const util = d >= 1 && d <= 5;
    semana.push({
      dia: d,
      aberto: atual.aberto !== undefined ? !!atual.aberto : (util || d === 6),
      inicio: atual.inicio || '09:00',
      fim: atual.fim || (d === 6 ? '13:00' : '18:00'),
      pausa_inicio: atual.pausa_inicio || '',
      pausa_fim: atual.pausa_fim || '',
    });
  }
  return semana;
}

// editorDeDias monta as 7 linhas (dia, abre, fecha, pausa de, pausa até).
function editorDeDias(dias, aoMudar) {
  const mudou = () => { if (aoMudar) aoMudar(); };
  return h('div', { class: 'dias' },
    h('div', { class: 'linha-dia cabeca', 'aria-hidden': 'true' },
      h('span', { text: 'Dia' }), h('span', { text: 'Abre' }), h('span', { text: 'Fecha' }),
      h('span', { text: 'Pausa de' }), h('span', { text: 'Pausa até' })),
    ORDEM_DA_SEMANA.map((d) => {
      const dia = dias[d];
      const nome = DIAS_LONGOS[d];
      const linha = h('div', { class: 'linha-dia' + (dia.aberto ? '' : ' fechado') });
      const marca = h('input', { type: 'checkbox', checked: dia.aberto });
      marca.addEventListener('change', () => {
        dia.aberto = marca.checked;
        linha.classList.toggle('fechado', !dia.aberto);
        mudou();
      });
      const hora = (chaveDia, curto, longo) => h('label', { class: 'hora-campo' },
        h('span', { class: 'hora-rotulo', text: curto }),
        entrada(dia, chaveDia, { type: 'time', 'aria-label': nome + ', ' + longo }, mudou));
      return por(linha, [
        h('label', { class: 'marcar dia-rotulo' }, marca, h('span', { text: nome })),
        hora('inicio', 'Abre', 'abre às'), hora('fim', 'Fecha', 'fecha às'),
        hora('pausa_inicio', 'Pausa de', 'pausa a partir de'), hora('pausa_fim', 'Pausa até', 'pausa até'),
      ]);
    }));
}

// editorDeServicos monta a lista de serviços (nome, duração e preço) com o botão de adicionar.
function editorDeServicos(servicos, aoMudar) {
  const mudou = () => { if (aoMudar) aoMudar(); };
  const lista = h('div', { class: 'pilha' });
  const pintar = () => {
    trocar(lista,
      servicos.map((s, i) => h('div', { class: 'linha-servico' },
        campo('Serviço', entrada(s, 'nome', { placeholder: 'Ex.: Consulta', autocomplete: 'off', maxLength: 120 }, mudou)),
        campo('Duração (min)', entrada(s, 'duracao_min', { type: 'number', min: 5, max: 600, step: 5, inputMode: 'numeric' }, mudou)),
        h('div', { class: 'campo-preco' }, campo('Preço', entrada(s, 'preco', { placeholder: 'Ex.: R$ 80', autocomplete: 'off', maxLength: 80 }, mudou))),
        botao('', {
          icone: 'lixo', classe: 'btn-icone', rotulo: 'Remover o serviço ' + (s.nome || (i + 1)),
          aoClicar: () => { servicos.splice(i, 1); mudou(); pintar(); },
        }))),
      servicos.length ? null : h('p', { class: 'ajuda', text: 'Sem serviços na lista, cada horário dura 30 minutos e o assistente marca pelo nome que o cliente disser.' }));
  };
  pintar();
  return h('div', { class: 'pilha' }, lista,
    h('div', null, botao('Adicionar serviço', {
      icone: 'mais',
      aoClicar: () => { servicos.push({ nome: '', duracao_min: 30, preco: '' }); mudou(); pintar(); },
    })));
}

// conferirSemana devolve uma frase de erro se algum dia aberto tiver horário impossível.
function conferirSemana(dias) {
  const min = (s) => { const m = /^(\d{1,2}):(\d{2})$/.exec(String(s || '')); return m ? Number(m[1]) * 60 + Number(m[2]) : null; };
  if (!dias.some((d) => d.aberto)) return 'Marque pelo menos um dia em que o negócio abre.';
  for (const d of ORDEM_DA_SEMANA) {
    const dia = dias[d];
    if (!dia.aberto) continue;
    const nome = DIAS_LONGOS[d];
    const ini = min(dia.inicio), fim = min(dia.fim);
    if (ini === null || fim === null) return nome + ': preencha a hora de abrir e a de fechar.';
    if (fim <= ini) return nome + ': a hora de fechar precisa ser depois da hora de abrir.';
    const pi = min(dia.pausa_inicio), pf = min(dia.pausa_fim);
    if ((pi === null) !== (pf === null)) return nome + ': preencha o começo e o fim da pausa, ou deixe os dois vazios.';
    if (pi !== null && (pf <= pi || pi < ini || pf > fim)) return nome + ': a pausa precisa ficar dentro do horário de funcionamento.';
  }
  return '';
}
