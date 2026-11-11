# Bibliotecas de terceiros

Estes arquivos vêm prontos de outros projetos. Eles ficam aqui dentro para o painel funcionar sem internet: nada é carregado de fora na hora de usar.

Não edite estes arquivos à mão. Para atualizar, baixe a versão nova no endereço indicado, troque o arquivo e ajuste a versão nesta página. A única mudança feita aqui está descrita em "Mudança no FullCalendar", mais abaixo.

| Arquivo | Biblioteca | Versão | Licença | De onde veio |
| --- | --- | --- | --- | --- |
| `motion.js` | Motion (pacote npm `motion`), cria o global `Motion` com `animate`, `stagger` e `inView` | 14.12.0 | MIT | https://cdn.jsdelivr.net/npm/motion@14.12.0/dist/motion.js |
| `fullcalendar.js` | FullCalendar, pacote padrão (pacote npm `fullcalendar`), cria o global `FullCalendar` | 6.1.22 | MIT | https://cdn.jsdelivr.net/npm/fullcalendar@6.1.22/index.global.min.js |
| `fullcalendar-pt-br.js` | FullCalendar, idioma português do Brasil (pacote npm `@fullcalendar/core`) | 6.1.22 | MIT | https://cdn.jsdelivr.net/npm/@fullcalendar/core@6.1.22/locales/pt-br.global.min.js |

Os textos das licenças estão em `LICENCA-motion.md` e `LICENCA-fullcalendar.md`.

Páginas dos projetos:

- Motion: https://motion.dev (código em https://github.com/motiondivision/motion)
- FullCalendar: https://fullcalendar.io (código em https://github.com/fullcalendar/fullcalendar)

## Onde são usadas

- `motion.js`: o ajudante `animar()` de `base.js` (troca de tela, avisos, janelas, listas e barra de situação). Se o arquivo faltar, o painel usa a animação do próprio navegador.
- `fullcalendar.js` e `fullcalendar-pt-br.js`: o calendário da aba Agenda (`agenda.js`). Se faltarem, a Agenda mostra só as listas de horários.

## Mudança no FullCalendar

O `fullcalendar.js` é igual ao arquivo original, menos um trecho: a regra `@font-face{font-family:fcicons;...}`, que traz uma fonte de ícones embutida no próprio arquivo (3.245 caracteres).

Motivo: a regra de segurança da página (`default-src 'self'`) não deixa carregar fonte embutida, e o navegador registrava um erro no console a cada abertura. O painel não usa essa fonte: os botões do calendário são do próprio painel e usam os ícones de `index.html`.

Para refazer depois de baixar uma versão nova:

```
python3 - <<'FIM'
import re
s = open('fullcalendar.js', encoding='utf-8').read()
regra = re.findall(r'@font-face\{font-family:fcicons;[^}]*\}', s)
assert len(regra) == 1
open('fullcalendar.js', 'w', encoding='utf-8').write(s.replace(regra[0], ''))
FIM
```

## Conferência dos arquivos (SHA-256)

```
a18fc08eb11c67658b34270bb71bc4ba806c87dbc1b720d7acd38c437844eee7  motion.js
98cc72a5134cb9869965ee8356295cf286cc1151c4ee33fe475c7819ed1bfab0  fullcalendar.js (sem a fonte de ícones)
65f927f1802cb35428b9c5e2a8d2276633880a89ee5ea5f9a7727f79892786ea  fullcalendar.js (original, como veio do endereço acima)
e1099f21b6a8ad7d9c96200d19dbc7f0536baba15fc945a81932df0e404a90fc  fullcalendar-pt-br.js
```

## Observações

- O FullCalendar 6 coloca o próprio CSS na página ao iniciar. As cores vêm dos tokens do painel, em `agenda.css` (variáveis `--fc-*`).
- Nenhum dos arquivos faz pedidos para a internet.
