'use strict';

// Gerenciar etiquetas: a lista única de etiquetas do painel.
// Aqui a pessoa renomeia, junta duas em uma (dando às duas o mesmo nome) e apaga.
const ETIQUETAS = (() => {
  let aberta = null;

  async function abrir() {
    if (aberta) return;
    const corpo = h('div', { class: 'etq' }, carregandoEl('Buscando as etiquetas...'));
    aberta = janela({
      titulo: 'Etiquetas',
      descricao: 'Cada etiqueta existe uma vez só e vale para todos os contatos. A IA usa esta lista durante o atendimento e só cria uma nova quando nenhuma serve.',
      corpo, classe: 'etq-janela', fecharFora: true,
      aoFechar: () => { aberta = null; if (typeof pulsar === 'function') pulsar(); },
    });
    try {
      const r = await api.get('/api/etiquetas');
      pintar(corpo, r.etiquetas || []);
    } catch (e) {
      trocar(corpo, h('p', { class: 'ac-aviso', role: 'alert', text: e.message }));
    }
  }

  function pintar(corpo, lista) {
    if (!lista.length) {
      trocar(corpo, vazio('lista', 'Ainda não há etiquetas',
        'A IA cria as primeiras durante os atendimentos. Você também pode criar na ficha de um contato ou de uma conversa.'));
      return;
    }
    const admin = typeof ACESSO === 'undefined' || ACESSO.pode('admin');
    trocar(corpo,
      h('ul', { class: 'etq-lista' }, lista.map((e) => linha(corpo, e, lista, admin))),
      h('p', { class: 'ajuda', text: admin
        ? 'Para juntar duas etiquetas parecidas, renomeie uma com o nome da outra. Os contatos ficam com uma só.'
        : 'Só administradores renomeiam e apagam etiquetas.' }));
  }

  function linha(corpo, e, lista, admin) {
    const quantos = e.contatos === 1 ? '1 contato' : e.contatos + ' contatos';
    const el = h('li', { class: 'etq-linha' });
    const normal = () => {
      trocar(el,
        h('span', { class: 'etq-nome' }, h('span', { class: 'cv-tag', text: e.nome }),
          e.da_ia ? h('span', { class: 'etq-ia', title: 'Posta pela IA em ' + e.da_ia + (e.da_ia === 1 ? ' contato' : ' contatos') }, icone('ia'), 'IA') : null),
        h('span', { class: 'etq-conta', text: quantos }),
        admin ? h('span', { class: 'etq-acoes' },
          botao('', { icone: 'lapis', classe: 'btn-icone', rotulo: 'Renomear a etiqueta ' + e.nome, aoClicar: editar }),
          botao('', { icone: 'lixo', classe: 'btn-icone etq-apagar', rotulo: 'Apagar a etiqueta ' + e.nome, aoClicar: apagar })) : null);
    };
    const editar = () => {
      const rascunho = { nome: e.nome };
      const campoNome = entrada(rascunho, 'nome', { maxLength: 30, autocomplete: 'off', 'aria-label': 'Nome novo da etiqueta ' + e.nome });
      const salvar = botao('Salvar', {
        classe: 'btn-primario',
        aoClicar: async () => {
          const novo = rascunho.nome.trim().replace(/\s+/g, ' ');
          if (!novo) throw new Error('Escreva o nome da etiqueta.');
          if (novo === e.nome) { normal(); return; }
          const outra = lista.find((x) => x !== e && dobrarTexto(x.nome) === dobrarTexto(novo));
          if (outra) {
            const sim = await confirmar({
              titulo: 'Juntar as duas etiquetas?',
              texto: 'Já existe a etiqueta "' + outra.nome + '". Quem tem "' + e.nome + '" passa a ter "' + outra.nome + '", e "' + e.nome + '" deixa de existir.',
              botao: 'Juntar',
            });
            if (!sim) return;
          }
          const r = await api.post('/api/etiquetas/renomear', { de: e.nome, para: novo });
          toast(outra ? 'Etiquetas juntadas em "' + r.nome + '".' : 'Etiqueta renomeada para "' + r.nome + '".');
          pintar(corpo, r.etiquetas || []);
        },
      });
      trocar(el,
        h('form', { class: 'etq-editar', novalidate: true, onsubmit: (ev) => { ev.preventDefault(); salvar.click(); } },
          campoNome, salvar, botao('Cancelar', { classe: 'btn-fantasma', aoClicar: normal })));
      campoNome.focus();
      campoNome.select();
    };
    const apagar = async () => {
      const sim = await confirmar({
        titulo: 'Apagar a etiqueta "' + e.nome + '"?',
        texto: 'Ela sai de ' + quantos + '. As conversas e os contatos continuam como estão. A IA não põe essa etiqueta de volta nesses contatos.',
        botao: 'Apagar a etiqueta', perigo: true,
      });
      if (!sim) return;
      const r = await api.post('/api/etiquetas/excluir', { nome: e.nome });
      toast('Etiqueta apagada.');
      pintar(corpo, r.etiquetas || []);
    };
    normal();
    return el;
  }

  // dobrarTexto compara nomes sem contar maiúsculas e acentos, como o programa faz.
  function dobrarTexto(s) {
    return String(s || '').normalize('NFD').replace(/[̀-ͯ]/g, '').toLowerCase().trim().replace(/\s+/g, ' ');
  }

  return { abrir };
})();
