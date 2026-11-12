# API local do painel

O painel (pasta `web/`) conversa com o programa por estas rotas. Tudo roda só em `127.0.0.1`.

Regras gerais:

- `GET` devolve JSON. `POST` recebe JSON (`Content-Type: application/json`), menos o envio de arquivo.
- Toda resposta de `POST` tem `ok`. Se `ok` for falso, vem `erro` com um texto em português para mostrar na tela, e o status HTTP é 4xx ou 5xx.
- Datas vêm em ISO 8601 com fuso (`2026-11-12T14:00:00-03:00`).
- As rotas `/api/ia/*`, `/api/treinar` e o envio de material chamam o Claude e podem levar de 5 a 60 segundos.

## Estado geral

`GET /api/estado` (o painel chama a cada 3 segundos)

```json
{
  "versao": "2.0.0",
  "status": "iniciando | aguardando | conectando | conectado | desconectado",
  "mensagem": "Conectado como Maria",
  "erro": "",
  "tem_qr": false,
  "nome": "Maria",
  "numero": "+55 27 99999-1234",
  "ligado": true,
  "salvo": true,
  "modo": "claude | api | teste",
  "erro_cerebro": "",
  "aviso_cerebro": "",
  "modelo": "claude-haiku-5-5",
  "agenda_ativa": true,
  "contadores": {
    "conversas": 3, "aguardando": 1, "quentes": 1, "mornos": 1, "frios": 0,
    "agendamentos": 2, "materiais": 4
  },
  "leads": [
    {
      "id": "5527999990000@s.whatsapp.net",
      "exibir": "Carlos",
      "telefone": "5527999990000",
      "telefone_bonito": "+55 27 99999-0000",
      "nome": "Carlos",
      "nome_whatsapp": "Carlão",
      "necessidade": "", "orcamento": "", "decisor": "", "prazo": "",
      "pontuacao": 60,
      "classificacao": "quente | morno | frio | indefinido",
      "etapa": "", "resumo": "",
      "status": "qualificando | qualificado | aguardando humano",
      "motivo": "",
      "criado_em": "...", "ultima_msg": "...",
      "previa": "último texto da conversa, cortado",
      "previa_de": "cliente | atendente | humano",
      "total_msgs": 12,
      "proximo_agendamento": { "servico": "Corte", "inicio": "..." }
    }
  ]
}
```

`leads` vem do mais recente para o mais antigo. `proximo_agendamento` pode ser `null`. `erro_cerebro` é o texto técnico da última falha da IA. `aviso_cerebro` vem preenchido quando o programa reconhece a causa (hoje, o login do Claude expirado). `ia` é `{ "instalado": true, "conectado": false }` e falta nos primeiros segundos, enquanto o programa ainda não conferiu o login. Com `aviso_cerebro` preenchido ou `ia.instalado && !ia.conectado`, a página mostra a faixa "A IA está desconectada" com o botão Conectar a IA. As rotas de IA que falham por falta de login respondem 503 com `"conectar_ia": true`, e a página abre a janela de conexão. `tem_qr` verdadeiro quer dizer que `GET /qr.png` devolve a imagem do código (trocar a cada 2 segundos com `?r=` aleatório).

`POST /api/liga` `{ "ligar": true }` liga ou pausa o atendente. Erro 400 se os dados do negócio ainda não foram salvos.

`POST /api/whatsapp/desconectar` `{}` remove o aparelho e mostra um novo código QR.

## Conversas

`GET /api/lead?id=ID`

```json
{
  "ok": true,
  "lead": { "...os mesmos campos da lista...": "", "historico": [ { "de": "cliente | atendente | humano", "texto": "Oi", "hora": "..." } ] },
  "agendamentos": [ { "...ver Agenda...": "" } ]
}
```

- `POST /api/retomar` `{ "id": "..." }` devolve a conversa ao assistente.
- `POST /api/assumir` `{ "id": "..." }` pausa o assistente só nesta conversa (status vira `aguardando humano`).
- `POST /api/responder` `{ "id": "...", "texto": "..." }` envia uma mensagem escrita pela pessoa no painel. A conversa passa para `aguardando humano`. Erro se o WhatsApp não estiver conectado.
- `POST /api/lead/excluir` `{ "id": "..." }` apaga a conversa e o lead.
- `GET /leads.csv` baixa a planilha de leads.

## Dados do negócio e ajustes

`GET /api/config`

```json
{
  "config": {
    "negocio": "", "segmento": "", "cidade": "", "endereco": "", "telefone": "",
    "site": "", "instagram": "",
    "produtos": "", "horario": "", "pagamento": "", "diferenciais": "",
    "promocoes": "", "politicas": "",
    "extras": [ { "nome": "ESTACIONAMENTO", "valor": "Gratuito na rua ao lado" } ],
    "assistente": "Ana",
    "tom": "simpático e profissional",
    "numero_humano": "",
    "nunca_responder": ["5527988887777"],
    "modelo": "claude-haiku-5-5",
    "atraso_min": 3, "atraso_max": 8, "limite_msgs": 6,
    "agenda": {
      "ativa": false,
      "servicos": [ { "nome": "Corte", "duracao_min": 30, "preco": "R$ 40" } ],
      "dias": [ { "dia": 0, "aberto": false, "inicio": "09:00", "fim": "18:00", "pausa_inicio": "", "pausa_fim": "" } ],
      "intervalo_min": 30, "vagas": 1, "antecedencia_min": 60, "dias_a_frente": 7,
      "avisar_humano": true
    },
    "salvo": false, "ligado": false
  },
  "variaveis": [ { "chave": "{{NEGOCIO}}", "rotulo": "Nome do negócio", "valor": "Barbearia Exemplo" } ],
  "modelos": [ { "id": "claude-haiku-5-5", "nome": "Haiku 5.5", "dica": "O mais rápido. Bom para o dia a dia." } ],
  "tons": [ "simpático e profissional", "..." ]
}
```

`agenda.dias` tem sempre 7 itens, `dia` de 0 (domingo) a 6 (sábado). `extras` são variáveis criadas pela pessoa: `nome` vira `{{NOME}}` nos textos.

`POST /api/config` recebe qualquer parte do objeto `config` (só as chaves que mudaram, ou todas). O que não vier fica como estava. `salvo` e `ligado` são ignorados aqui. Resposta: `{ "ok": true, "mensagem": "Salvo", "config": { ... } }`.

## Textos do assistente

Quatro textos, relidos do disco a cada mensagem: `personalidade`, `cerebro`, `regras`, `aprendizados`. Eles aceitam variáveis como `{{NEGOCIO}}`, trocadas pelo valor na hora de responder.

- `GET /api/textos` devolve `{ "personalidade": { "texto": "...", "padrao": true }, "cerebro": {...}, "regras": {...}, "aprendizados": {...} }`. `padrao` diz se o texto é igual ao modelo de fábrica.
- `POST /api/textos` `{ "arquivo": "personalidade", "texto": "..." }` salva.
- `POST /api/textos/restaurar` `{ "arquivo": "personalidade" }` volta ao modelo de fábrica e devolve `{ "ok": true, "texto": "..." }`.

## Ajuda da IA para escrever

Nenhuma destas rotas salva nada. Elas devolvem um rascunho para a pessoa revisar.

- `POST /api/ia/negocio` `{ "descricao": "texto livre sobre o negócio" }` devolve `{ "ok": true, "campos": { "negocio": "", "segmento": "", "cidade": "", "endereco": "", "telefone": "", "site": "", "instagram": "", "produtos": "", "horario": "", "pagamento": "", "diferenciais": "", "promocoes": "", "politicas": "" } }`. Campo que a IA não souber vem vazio.
- `POST /api/ia/melhorar` `{ "arquivo": "personalidade | cerebro | regras", "texto": "texto atual da tela", "pedido": "o que mudar" }` devolve `{ "ok": true, "texto": "novo texto", "mudancas": "o que mudou, em uma frase" }`.
- `POST /api/ia/cerebro` `{}` monta um cérebro novo com os dados do negócio e os materiais da base de conhecimento. Devolve `{ "ok": true, "texto": "..." }`.

## Treinar

`POST /api/treinar`

```json
{
  "mensagens": [ { "de": "cliente", "texto": "Oi" }, { "de": "atendente", "texto": "Olá!" }, { "de": "cliente", "texto": "Quanto custa?" } ],
  "lead": { "nome": "", "necessidade": "", "orcamento": "", "decisor": "", "prazo": "", "pontuacao": 0, "classificacao": "indefinido", "etapa": "", "resumo": "" }
}
```

A última mensagem tem que ser do cliente. `lead` é o que a resposta anterior devolveu (ou vazio na primeira vez). Nada vai para o WhatsApp e nada é marcado na agenda. Resposta:

```json
{
  "ok": true,
  "resposta": "texto que o assistente mandaria",
  "transbordo": false,
  "motivo": "",
  "lead": { "nome": "", "necessidade": "", "orcamento": "", "decisor": "", "prazo": "", "pontuacao": 30, "classificacao": "frio", "etapa": "", "resumo": "" },
  "agendamento": { "acao": "nenhuma | marcar | cancelar", "servico": "", "data": "", "hora": "" },
  "trechos": [ { "titulo": "Tabela de preços.pdf", "texto": "trecho usado" } ],
  "modo": "claude",
  "ms": 5400
}
```

`POST /api/aprendizado` `{ "situacao": "o que o cliente disse", "resposta": "como o assistente deve responder" }` acrescenta uma linha em `aprendizados` e devolve `{ "ok": true, "texto": "texto completo de aprendizados" }`.

## Base de conhecimento

`GET /api/conhecimento`

```json
{
  "materiais": [
    {
      "id": "m1a2b3", "nome": "tabela.pdf", "tipo": "pdf", "tamanho": 182331,
      "titulo": "Tabela de preços 2026", "resumo": "...", "perguntas": 8,
      "trechos": 12, "caracteres": 9400, "ativo": true,
      "estado": "organizando | pronto | erro", "erro": "", "criado_em": "..."
    }
  ],
  "formatos": ["pdf", "docx", "pptx", "txt", "md", "csv"],
  "limite_mb": 25
}
```

- `POST /api/conhecimento/enviar`: o corpo é o arquivo puro. Cabeçalhos: `Content-Type: application/octet-stream` e `X-Nome` com `encodeURIComponent(nome do arquivo)`. Devolve `{ "ok": true, "material": { ... } }` já com `estado` `organizando`. O resumo aparece depois, quando a IA terminar (acompanhe pelo `GET`).
- `POST /api/conhecimento/texto` `{ "titulo": "...", "texto": "..." }` cria um material a partir de texto colado.
- `POST /api/conhecimento/alterar` `{ "id": "...", "ativo": false }` liga ou desliga um material.
- `POST /api/conhecimento/excluir` `{ "id": "..." }` apaga.

## Agenda

`GET /api/agenda`

```json
{
  "ativa": true,
  "servicos": [ { "nome": "Corte", "duracao_min": 30, "preco": "R$ 40" } ],
  "agendamentos": [
    {
      "id": "a1b2c3", "lead_id": "", "nome": "Carlos", "telefone": "5527999990000",
      "telefone_bonito": "+55 27 99999-0000",
      "servico": "Corte", "inicio": "2026-11-12T14:00:00-03:00", "duracao_min": 30,
      "origem": "assistente | painel", "status": "marcado | cancelado", "obs": "", "criado_em": "..."
    }
  ],
  "livres": [ { "data": "2026-11-12", "rotulo": "qua 12/11", "horas": ["09:00", "09:30"] } ]
}
```

`agendamentos` traz os futuros e os dos últimos 30 dias, do mais próximo para o mais distante. `livres` cobre os próximos dias configurados.

- `POST /api/agenda/marcar` `{ "nome": "", "telefone": "", "servico": "", "data": "2026-11-12", "hora": "14:00", "obs": "" }` devolve `{ "ok": true, "agendamento": { ... } }`.
- `POST /api/agenda/cancelar` `{ "id": "..." }`.
- `GET /agenda.csv` baixa a planilha da agenda.

As regras da agenda (serviços, dias, horários) ficam em `config.agenda` e são salvas por `POST /api/config`.

---

# Versão 2.1: novidades

Tudo o que está acima continua valendo. O que segue é novo ou mudou.

## Situação da conversa (`status`)

| status | quer dizer |
| --- | --- |
| `qualificando`, `qualificado` | o assistente (IA) está respondendo |
| `aguardando humano` | o assistente transferiu e a conversa está na fila, esperando uma pessoa |
| `em atendimento` | uma pessoa da equipe assumiu e está atendendo |

O assistente só responde nos dois primeiros. A pessoa só escreve e envia arquivos em `em atendimento`.

## Lead: campos novos (em `GET /api/estado` e `GET /api/lead`)

```json
{
  "etiquetas": ["cliente antigo", "orçamento"],
  "tem_foto": true,
  "foto_versao": "a1b2",
  "atendente": "Oliveira",
  "nao_lidas": 2,
  "previa_tipo": "texto | imagem | audio | video | documento"
}
```

- Foto do contato: `GET /api/foto?id=<id do lead>&v=<foto_versao>` devolve a imagem. Só existe se `tem_foto` for verdadeiro. Sem foto, a tela mostra as iniciais.
- `atendente` é quem assumiu (vazio se ninguém). O cliente nunca vê esse nome: ele fica só no histórico interno.
- `nao_lidas` conta as mensagens do cliente depois da última resposta.
- `previa_tipo` é o tipo da última mensagem (a de `previa`), para a lista mostrar um ícone quando for mídia. Em mídia sem legenda, `previa` vem como `[imagem]`, `[áudio]`, `[vídeo]` ou `[arquivo] nome`.

`GET /api/estado` ganhou `contadores.em_atendimento` e `etiquetas` (lista, em ordem alfabética, de todas as etiquetas em uso, para sugerir e filtrar).

## Histórico: tipos de mensagem

Cada item de `lead.historico`:

```json
{
  "de": "cliente | atendente | humano | sistema",
  "texto": "texto ou legenda (pode ser vazio em mídia)",
  "hora": "...",
  "tipo": "texto | imagem | audio | video | documento | evento",
  "midia": "f3a9c1d2e4.jpg",
  "nome": "tabela.pdf",
  "mime": "application/pdf",
  "tamanho": 182331,
  "autor": "Oliveira",
  "evento": "transbordo | assumiu | devolveu | agendou | cancelou"
}
```

- `tipo` ausente quer dizer `texto`.
- Mídia: `GET /api/midia?id=<midia>` devolve o arquivo. Imagem, áudio, vídeo e PDF abrem no navegador; o resto baixa. Se `midia` vier vazio em uma mensagem de mídia, o arquivo não pôde ser baixado (grande demais ou expirado): mostre só o `texto`.
- `de: "humano"` é a pessoa da equipe (pelo painel ou pelo celular). `autor` diz quem foi, quando se sabe.
- `de: "sistema"` com `tipo: "evento"` é uma linha de histórico interno (por exemplo "Oliveira iniciou o atendimento"). Mostre no meio da conversa, discreta, sem bolha. O cliente não recebe isso.

## Atendimento por uma pessoa

- `POST /api/assumir` `{ "id": "...", "atendente": "Oliveira" }` inicia o atendimento: `status` vira `em atendimento`.
- `POST /api/retomar` `{ "id": "...", "atendente": "Oliveira" }` encerra e devolve ao assistente.
- `POST /api/responder` `{ "id": "...", "texto": "...", "atendente": "Oliveira" }` envia texto. Se a conversa ainda não estava em atendimento, passa a estar.
- `POST /api/enviar-arquivo` envia imagem, áudio, vídeo ou documento. O corpo é o arquivo puro (`Content-Type: application/octet-stream`, até 25 MB). Cabeçalhos, todos com `encodeURIComponent`:
  - `X-Lead`: id da conversa
  - `X-Nome`: nome do arquivo
  - `X-Mime`: tipo do arquivo (`file.type` ou o tipo da gravação, por exemplo `audio/mp4`)
  - `X-Tipo`: `arquivo` ou `voz` (gravação feita no painel)
  - `X-Legenda`: texto junto com a imagem, o vídeo ou o documento (opcional)
  - `X-Atendente`: nome de quem envia
  Resposta: `{ "ok": true }`. A mensagem aparece no histórico na próxima busca.

  Gravação de voz: use `MediaRecorder` com o primeiro tipo que o navegador aceitar nesta ordem: `audio/ogg;codecs=opus`, `audio/mp4;codecs=mp4a.40.2`, `audio/mp4`, `audio/webm;codecs=opus`. Só o ogg sai como mensagem de voz; os outros saem como arquivo de áudio.
- Desde a versão 2.2 o nome de quem atende vem do login. Os campos `atendente` e `X-Atendente` são ignorados.

## Etiquetas

`POST /api/lead/etiquetas` `{ "id": "...", "etiquetas": ["cliente antigo", "vip"] }` troca a lista inteira. Devolve `{ "ok": true, "etiquetas": [...] }` já limpa (sem repetidas, até 12 por conversa, 30 letras cada).

## Relatório

`GET /api/relatorio?dias=7` (aceita 1, 7, 30 ou 0 para tudo)

```json
{
  "dias": 7,
  "periodo": {
    "conversas": 12, "novas": 9,
    "mensagens_clientes": 80, "respostas_assistente": 61, "respostas_equipe": 14,
    "transbordos": 5, "agendamentos": 4, "cancelamentos": 1
  },
  "agora": {
    "total": 30, "com_assistente": 22, "aguardando": 3, "em_atendimento": 5,
    "quentes": 4, "mornos": 9, "frios": 6, "indefinidos": 11
  },
  "por_etiqueta": [ { "nome": "orçamento", "total": 7 } ],
  "por_dia": [ { "data": "2026-11-05", "rotulo": "qui 05/11", "novas": 2, "mensagens": 14, "transbordos": 1, "agendamentos": 0 } ],
  "atendentes": [ { "nome": "Oliveira", "respostas": 9 } ]
}
```

`periodo` conta só o que aconteceu dentro dos dias pedidos. `agora` é a foto de todas as conversas neste momento. `por_dia` traz um item por dia, do mais antigo para o mais novo (14 dias quando `dias` é 0 ou 1).

## Números que recebem os avisos

`config.numeros_humanos` é uma lista (`["5527999990000", "5527988880000"]`). Todos recebem os avisos e todos podem mandar `/lista` e `/retomar`. `config.numero_humano` continua existindo só para leitura (é o primeiro da lista). Salve por `POST /api/config` `{ "numeros_humanos": [...] }`.

# Versão 2.2: login, banco de dados e contatos

## Acesso (login)

Tudo em `/api/...` e as planilhas (`.csv`) exigem sessão, menos as rotas desta lista marcadas como livres. A sessão vai em um cookie `HttpOnly`, `SameSite=Strict`, chamado `atendente_PORTA`. Sem sessão a resposta é `401` com `{ "ok": false, "erro": "...", "entrar": true }` (ou `"precisa_criar": true` quando ainda não existe conta). Com a senha provisória ainda não trocada, tudo responde `403` com `"trocar_senha": true`, menos `/api/senha`. Sem permissão para a rota: `403` com `"sem_permissao": true`.

Tipos de acesso (`papel`): `superadmin` (tudo), `admin` (tudo menos equipe e registro) e `atendente` (conversas, contatos, agenda e relatório). O mapa de quem alcança cada rota está em `nivelDaRota`, em `acesso.go`.

| Rota | Quem | O que faz |
| --- | --- | --- |
| `GET /api/sessao` | livre | `{ "versao", "precisa_criar", "redefinir", "redefinir_nome", "usuario", "min_senha" }`. `usuario` é `null` ou `{ id, nome, email, papel, papel_nome, ativo, trocar_senha, criado_em, ultimo_acesso }` |
| `POST /api/primeiro-acesso` | livre, só sem nenhuma conta | `{ nome, email, senha, senha2 }` cria o super administrador e já abre a sessão. `409` se já existir conta |
| `POST /api/entrar` | livre | `{ email, senha }`. `401` com "E-mail ou senha incorretos."; `429` com `espera_min` depois de 5 erros seguidos para o mesmo e-mail em 15 minutos |
| `POST /api/sair` | livre | Encerra a sessão e apaga o cookie |
| `POST /api/senha` | logado | `{ atual, nova, nova2 }` troca a própria senha e encerra as outras sessões da conta |
| `POST /api/redefinir-senha` | livre, só com a recuperação aberta | `{ senha, senha2 }` cria a senha nova do super administrador. A recuperação abre por 30 minutos com `./atendente -esqueci-senha` |
| `GET /api/usuarios` | superadmin | `{ "usuarios": [ { ..., "voce": true } ], "papeis": [ { id, nome, dica } ] }` |
| `POST /api/usuarios` | superadmin | `{ nome, email, papel, senha }`. A senha é provisória: a pessoa cria a própria na primeira entrada |
| `POST /api/usuarios/alterar` | superadmin | `{ id, nome?, papel?, ativo?, senha? }`. `senha` vira provisória e encerra as sessões da pessoa |
| `POST /api/usuarios/excluir` | superadmin | `{ id }`. O super administrador não pode ser removido, desativado nem ter o acesso trocado |
| `GET /api/registro?limite=50&antes=N` | superadmin | `{ "registros": [ { n, quando, usuario, acao, texto, detalhe } ], "tem_mais" }`, do mais novo para o mais antigo |

Erros de formulário trazem `campo` (por exemplo `"campo": "senha2"`), para a tela mostrar o aviso no lugar certo.

## Contatos

`GET /api/contatos?q=&situacao=&temperatura=&etiqueta=&ordem=`

- `q`: nome, telefone, etiqueta ou trecho do que a pessoa precisa. Só números procura no telefone.
- `situacao`: `assistente`, `fila` ou `atendimento`. `temperatura`: `quente`, `morno`, `frio` ou `indefinido`.
- `ordem`: `recentes` (padrão), `antigos`, `nome` ou `pontuacao`.

Resposta: `{ "contatos": [ ...mesmos campos do lead, sem o histórico... ], "total", "total_geral", "etiquetas" }`. `total_msgs` conta todas as mensagens trocadas desde o começo.

`GET /contatos.csv` (lista simples) e `GET /leads.csv` (com os dados de qualificação) aceitam os mesmos filtros.

## Conversas longas

A memória guarda as últimas 80 mensagens de cada conversa; o banco guarda todas. `GET /api/lead` devolve também `"tem_anteriores": true` quando há mensagens mais antigas. Cada mensagem tem `n`, o número dela no banco.

`GET /api/lead/historico?id=...&antes=N` devolve `{ "ok": true, "mensagens": [...], "tem_mais": true }` com até 60 mensagens anteriores à de número `N`, em ordem de tempo.

## Banco de dados

Um arquivo SQLite, `dados/atendente.db`, criado pelo próprio programa. Tabelas: `config` (dados do negócio e ajustes), `contatos`, `mensagens`, `agendamentos`, `usuarios`, `sessoes`, `registro` e `meta`. Uma cópia por dia fica em `dados/copias/` (as 7 mais novas). Os arquivos `config.json`, `leads.json` e `agenda.json` das versões anteriores são levados para o banco na primeira vez e ficam guardados com o final `.migrado`.


## Versão 2.3: conexão da IA

O login do Claude feito pelo painel. Todas as rotas são de administrador. O programa roda só o comando fixo `claude auth login --claudeai` e nunca vê a senha.

### `GET /api/ia/conexao`

`?agora=1` confere de novo na hora (sem isso, o programa reaproveita a última conferência por 20 segundos).

```json
{
  "instalado": true,
  "conectado": false,
  "conta": "",
  "plano": "",
  "em_andamento": false,
  "endereco": "",
  "erro": ""
}
```

- `instalado`: o `claude` existe neste computador.
- `conectado`: o login está valendo. `conta` e `plano` vêm preenchidos quando o `claude` informa.
- `em_andamento`: há um login aberto, esperando a pessoa autorizar no navegador (até 10 minutos).
- `endereco`: página de reserva do Claude, para quando o navegador não abre sozinho. Só aparece com o login em andamento e só aceita endereços `https` do Claude. Essa página mostra um código no final.
- `erro`: frase pronta quando o último login não terminou.

### `POST /api/ia/conectar`

Corpo `{}`. Inicia o login (ou não faz nada se já houver um em andamento) e devolve o mesmo objeto de `GET /api/ia/conexao`. Responde 503 se o `claude` não estiver instalado.

### `POST /api/ia/conectar/codigo`

Corpo `{ "codigo": "..." }`. Entrega ao login em andamento o código que a página de reserva mostrou. 400 para código vazio, com espaço ou quebra de linha. 409 se não houver login em andamento.

### `POST /api/ia/conectar/cancelar`

Corpo `{}`. Encerra o login em andamento.

## Versão 2.4: ChatGPT por chave, etiquetas da IA e contexto da equipe

### Quem responde

`GET /api/estado` passa a trazer `"modo": "openai"` quando o ChatGPT está em uso, e `ia` vira:

```json
{ "provedor": "claude | openai", "instalado": true, "conectado": true, "motivo": "" }
```

`motivo` diz por que não está pronto: `claude_login`, `openai_chave`, `openai_credito` ou `openai_modelo`. A página usa o motivo para escolher o texto da faixa vermelha.

`GET /api/ia/conexao` ganha campos (os antigos continuam, e falam do login do Claude):

```json
{
  "provedor": "openai",
  "conectado": true,
  "motivo": "",
  "claude_conectado": false,
  "openai": { "tem_chave": true, "final": "AbC9", "modelo": "gpt-5-mini", "origem": "cofre | ambiente" }
}
```

`conectado` é do provedor em uso. A chave nunca volta: só `final`, as 4 últimas letras.

### `POST /api/ia/openai`

Corpo `{ "chave": "sk-...", "modelo": "" }`. Confere a chave com a OpenAI, guarda no cofre de senhas do sistema, escolhe o modelo (o enviado, o que já estava ou um padrão da conta) e passa a usar o ChatGPT. Sem `chave`, só troca o modelo. Devolve o objeto de `GET /api/ia/conexao` mais `"modelos": ["gpt-..."]`. 400 para chave em formato errado, chave recusada pela OpenAI ou nome de modelo inválido. 502 se a OpenAI não respondeu. 500 se o computador não tem cofre de senhas (a chave não é guardada).

### `GET /api/ia/openai/modelos`

`{ "modelos": ["gpt-..."] }`: os modelos de conversa da conta da chave guardada. Lista vazia sem chave.

### `POST /api/ia/provedor`

Corpo `{ "provedor": "claude" | "openai" }`. Troca quem responde. 400 ao escolher `openai` sem chave.

### `POST /api/ia/openai/remover`

Corpo `{}`. Apaga a chave do cofre e volta para o Claude. 409 se a chave vem da variável `OPENAI_API_KEY`.

`POST /api/config` ignora `provedor` e `modelo_openai`: quem responde só muda pelas rotas acima.

### Etiquetas criadas pela IA

- A saída da IA tem `lead.etiquetas` (lista de textos, até 3 por resposta).
- Cada lead em `GET /api/estado`, `GET /api/lead` e `GET /api/contatos` traz `etiquetas_ia`: as etiquetas de `etiquetas` que foram criadas pela IA.
- `POST /api/lead/etiquetas` devolve também `etiquetas_ia`. O que a pessoa tira por essa rota a IA não repõe.
- `POST /api/treinar` devolve `etiquetas`: as etiquetas da conversa simulada. A página manda de volta em `lead.etiquetas` no pedido seguinte.

## Versão 2.5: agenda pelo onboard, etiquetas únicas, ficha do contato e voz

### Horários e agenda

- Os dias da agenda (`config.agenda.dias`, um item por dia de 0 a 6, com `aberto`, `inicio`, `fim`, `pausa_inicio` e `pausa_fim`) são a fonte do horário de funcionamento. Quando `POST /api/config` recebe a chave `agenda`, o programa escreve `config.horario` a partir dos dias (por exemplo: "Segunda a sexta, das 9h às 19h, com pausa das 12h às 13h30. Sábado, das 8h às 20h, com pausa das 12h às 14h. Domingo, fechado."). A página não envia mais `horario`.
- `POST /api/ia/negocio` devolve, além de `campos`, a chave `agenda`: `null` quando a descrição não fala de horário nem de serviço com hora marcada, ou

```json
{
  "usa": true,
  "dias": [ { "dia": 0, "aberto": false, "inicio": "09:00", "fim": "18:00", "pausa_inicio": "", "pausa_fim": "" } ],
  "servicos": [ { "nome": "Corte", "duracao_min": 30, "preco": "R$ 70" } ]
}
```

  `dias` traz sempre os 7 dias, ou falta quando a descrição só fala de serviços. Quando há dias, `campos.horario` já vem escrito a partir deles. A configuração inicial usa isso no passo Horários e salva com `POST /api/config { "agenda": { "ativa", "dias", "servicos" } }`.
- A saída da IA aceita `agendamento.acao` igual a `remarcar`, com a data e a hora novas: o agendamento do cliente muda de horário de uma vez. No histórico vira o registro `remarcou`.

### Etiquetas únicas

- Cada etiqueta existe uma vez só no painel, sem contar maiúsculas e acentos. `POST /api/lead/etiquetas` troca a grafia enviada pela que já existe.
- `lead.etiquetas`, na saída da IA, passa a ser a lista completa que a conversa deve ter. O programa põe as que faltam e tira as que a própria IA tinha posto e saíram da lista. Lista vazia não muda nada.
- `GET /api/etiquetas` (quem está logado): `{ "etiquetas": [ { "nome": "corte", "contatos": 3, "da_ia": 2 } ] }`, da mais usada para a menos usada.
- `POST /api/etiquetas/renomear` (administrador) com `{ "de", "para" }`: troca o nome em todos os contatos. Se `para` já existir, as duas viram uma só. Devolve `{ "etiquetas", "nome", "contatos" }`. 404 se a etiqueta não existir.
- `POST /api/etiquetas/excluir` (administrador) com `{ "nome" }`: tira a etiqueta de todos os contatos. Devolve `{ "etiquetas", "contatos" }`.
- Toda mudança de etiqueta vira uma mensagem interna com `de` igual a `sistema` e `evento` igual a `etiqueta`. A tela de conversa não mostra esse registro entre as mensagens, só no histórico interno e na ficha do contato.

### Ficha do contato

`GET /api/contato?id=<id>` (quem está logado):

```json
{
  "ok": true,
  "contato": { "...": "os mesmos campos do lead de GET /api/contatos, sem historico", "anotacoes": "texto" },
  "agendamentos": [ { "id": "", "lead_id": "", "nome": "", "telefone": "", "telefone_bonito": "", "servico": "", "inicio": "", "duracao_min": 60, "origem": "assistente | painel", "status": "marcado | cancelado", "obs": "", "criado_em": "" } ],
  "historico": [ { "n": 143, "hora": "2026-11-12T12:15:27Z", "evento": "assumiu", "texto": "...", "autor": "" } ],
  "historico_tem_mais": false
}
```

- `agendamentos`: todos os do contato, do mais novo para o mais antigo. Entram os que têm `lead_id` igual ao contato e os marcados pelo painel com o mesmo telefone.
- `historico`: os registros internos do contato (mensagens com `de` igual a `sistema`), lidos do banco, do mais novo para o mais antigo, até 500. O último item é sempre `evento` igual a `primeiro_contato`, montado a partir de `criado_em`. Valores de `evento`: `transbordo`, `assumiu`, `devolveu`, `agendou`, `remarcou`, `cancelou`, `etiqueta`. A página mostra tipos desconhecidos pelo texto.
- 404 quando o contato não existe.

`POST /api/contato/anotacoes` (quem está logado) com `{ "id", "anotacoes" }`: grava as anotações da equipe e devolve `{ "ok": true, "anotacoes": "como ficou gravado" }`. 400 acima de 4000 letras, sem gravar.

Na aba Contatos, clicar em uma linha abre `#contatos/<id>` (a ficha). A conversa fica em `#conversas/<id>`.

### Voz (reconhecimento local)

- `GET /api/voz/estado` (quem está logado): `{ "ok", "pronto", "baixando", "progresso", "erro", "tamanho_mb", "arquivos": { "biblioteca", "wasm_mjs", "wasm", "modelos", "modelo" } }`. `progresso` vai de 0 a 100.
- `POST /api/voz/preparar` (quem está logado, corpo `{}`): começa ou tenta de novo o download. Mesma resposta.
- `GET /voz/...` (quem está logado): os arquivos da voz, baixados para `dados/voz/`. Só entrega o que está na lista fixa de `voz.go` e já teve o SHA-256 conferido.
- Na página, `VOZ.botao({ aoTexto(texto, final), aoEstado(estado, info) })` devolve o botão do microfone. `texto` é sempre o texto inteiro da gravação até ali. `microfoneDe(area, obj, chave, aoTerminar)`, em `base.js`, liga esse botão a uma caixa de texto.

