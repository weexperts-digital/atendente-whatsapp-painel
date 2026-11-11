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

`leads` vem do mais recente para o mais antigo. `proximo_agendamento` pode ser `null`. `tem_qr` verdadeiro quer dizer que `GET /qr.png` devolve a imagem do código (trocar a cada 2 segundos com `?r=` aleatório).

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
- `atendente` é sempre opcional. O nome de quem atende fica no navegador, em `localStorage["atendente.nome"]`.

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
