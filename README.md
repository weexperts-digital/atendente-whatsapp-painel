# Atendente de WhatsApp com IA

Um atendente de inteligência artificial para o WhatsApp do seu negócio. Ele conversa com quem escreve, entende o que a pessoa precisa, separa os leads em quente, morno e frio, marca horários na agenda e chama você quando é hora de uma pessoa assumir.

Roda no seu computador. O cérebro é o Claude, pela sua conta do Claude Code. Não tem servidor, não tem login e não tem mensalidade deste projeto.

Esta é a versão com painel completo. A versão mais simples, usada na palestra do SM Summit, fica em [weexperts-digital/atendente-whatsapp](https://github.com/weexperts-digital/atendente-whatsapp).

## O que ele faz

- **Conversas**: uma tela com cara de WhatsApp, com foto do contato, áudio, imagem, vídeo e PDF dentro da conversa. Quem foi transferido entra em uma fila; a pessoa da equipe clica em Iniciar atendimento, responde por texto, arquivo ou áudio, e depois devolve ao assistente. O cliente não vê quem está atendendo.
- **Etiquetas e relatório**: marque as conversas com etiquetas e veja um relatório simples de conversas, transferências, agendamentos e respostas.
- **Qualificação de leads**: pontua cada pessoa de 0 a 100 e avisa no seu WhatsApp quando o lead esquenta ou pede uma pessoa.
- **Agenda**: um calendário de semana, dia e mês. Você cadastra serviços, dias e horários; o assistente oferece os horários livres e marca sozinho.
- **Base de conhecimento**: envie PDF, Word, PowerPoint ou texto. A IA lê, resume e passa a responder com base nos seus materiais.
- **Assistente editável**: personalidade, cérebro e regras de transbordo em abas separadas, com variáveis como `{{NEGOCIO}}` e um botão para pedir mudanças à IA.
- **Treinar**: converse com o assistente dentro do painel, veja o que ele entendeu e corrija as respostas. As correções viram aprendizados.

## Do que você precisa

1. Um computador com Mac, Linux ou Windows.
2. O [Claude Code](https://claude.com/claude-code) instalado e com a sua conta conectada. É ele que pensa as respostas.
3. Um número de WhatsApp para o negócio. De preferência um número só para isso.

## Como ligar

### Jeito mais fácil: pedindo ao Claude Code

Abra o Claude Code e cole:

```
Baixe o repositório https://github.com/weexperts-digital/atendente-whatsapp-painel em uma pasta nova, entre nela e ligue o atendente seguindo o CLAUDE.md.
```

Ele baixa, prepara tudo e abre o painel no navegador.

### Pelo terminal (Mac e Linux)

```bash
git clone https://github.com/weexperts-digital/atendente-whatsapp-painel.git
```

```bash
cd atendente-whatsapp-painel && ./instalar.sh
```

O instalador baixa o Go para dentro da própria pasta, se você não tiver, monta o programa e abre o painel. Não pede senha de administrador.

### No Windows (PowerShell)

```powershell
git clone https://github.com/weexperts-digital/atendente-whatsapp-painel.git
cd atendente-whatsapp-painel
powershell -ExecutionPolicy Bypass -File .\instalar.ps1
```

Os scripts do Windows ainda não foram testados em uma máquina de verdade. Se algo falhar, abra o Claude Code na pasta e peça: ligue o atendente.

## Primeiros passos no painel

O painel abre em `http://localhost:8787` e guia você:

1. Conte sobre o seu negócio. Dá para escrever em poucas palavras e deixar a IA preencher os campos.
2. Escolha o nome e o tom do assistente.
3. Informe o WhatsApp que recebe os avisos de lead quente.
4. Leia o código QR com o WhatsApp do negócio (WhatsApp, Configurações, Aparelhos conectados, Conectar aparelho).
5. Ligue o atendente e mande um "Oi" de outro celular.

Depois disso:

| Para | Faça |
| --- | --- |
| Ligar de novo depois de reiniciar o computador | `./ligar.sh` (ou peça ao Claude Code: ligue o atendente) |
| Parar | `./parar.sh` |
| Ver o que aconteceu | abra o arquivo `atendente.log` |
| Atualizar depois de mudar o código | `./instalar.sh` |

Se o computador dormir ou desligar, o atendente para. Ele volta quando você ligar de novo.

## Como funciona por dentro

```
WhatsApp  ->  atendente (Go)  ->  claude -p (sem ferramentas)  ->  resposta
                   |
                   +-- painel local em 127.0.0.1 (pasta web/)
                   +-- dados/ (seus dados, só no seu computador)
```

- **Um programa só**, em Go, com a página embutida. A conexão com o WhatsApp usa a biblioteca [whatsmeow](https://github.com/tulir/whatsmeow).
- **A cada mensagem** o programa monta as instruções com a personalidade, o cérebro, as regras de transbordo, os aprendizados, os trechos dos seus materiais que combinam com a conversa e os horários livres da agenda. Depois chama o `claude` e recebe a resposta em um formato fixo (resposta, dados do lead, transbordo, agendamento).
- **Base de conhecimento**: o texto dos arquivos é extraído no próprio programa e dividido em trechos. Bases pequenas vão inteiras nas instruções. Bases grandes passam por uma busca por palavras (BM25) e só os trechos mais ligados à conversa entram.
- **Seus dados** ficam na pasta `dados/`, que nunca vai para o repositório.

A descrição das rotas do painel está em [docs/API.md](docs/API.md).

## Segurança

- O assistente só responde conversas privadas, e só a quem escreveu primeiro. Ignora grupos, status e canais. As únicas mensagens que ele inicia são os avisos para o número que você cadastrou.
- Nenhuma mensagem de WhatsApp consegue executar comandos no seu computador. O Claude é chamado sem nenhuma ferramenta (`--tools ""`), em uma pasta vazia, e só lê texto.
- Fotos, áudios, vídeos e documentos que os clientes mandam são baixados somente dos servidores do WhatsApp e guardados em `dados/midia`, para a equipe ver no painel. O programa não abre nem executa esses arquivos, e a IA não os lê. As fotos de perfil vêm do mesmo lugar.
- O painel escuta só em `127.0.0.1`. Ninguém de fora do computador consegue abrir.
- Nenhuma chave de API é gravada em arquivo.
- O texto das conversas e dos materiais é enviado ao Claude para gerar as respostas. Fora isso, nada sai do computador.

## Avisos importantes

- Este projeto não é do WhatsApp nem da Meta e não usa a API oficial. Ele conecta como um aparelho a mais, igual ao WhatsApp Web. Contas que mandam muita mensagem ou são denunciadas podem ser bloqueadas pelo WhatsApp. Use um número próprio para isso, responda só a quem procurou você e não use para disparos.
- É um projeto de demonstração: serve para mostrar que dá para ligar um agente de IA no seu negócio em poucos minutos. Antes de depender dele no dia a dia, teste bastante na aba Treinar.
- As respostas usam a sua conta do Claude e contam no limite do seu plano.

## Ajustes para quem mexe no código

| Variável de ambiente | Para que serve |
| --- | --- |
| `CLAUDE_MODELO` | Força o modelo das respostas, no lugar do escolhido no painel |
| `CLAUDE_MODELO_AJUDANTE` | Modelo que escreve rascunhos e resume materiais (padrão `claude-sonnet-5-5`) |
| `CLAUDE_BIN` | Caminho do programa `claude`, se ele não estiver no PATH |
| `ANTHROPIC_API_KEY` | Se existir, vira reserva quando o `claude` falha duas vezes seguidas |
| `ATENDENTE_WEB` | Pasta de onde servir a página, para mexer no visual sem compilar de novo |
| `ATENDENTE_PORTA` | Primeira porta tentada pelo painel (padrão 8787), para rodar uma cópia de teste ao lado |

```bash
CGO_ENABLED=0 go build -o atendente .
```

```bash
go test .
```

```bash
./atendente -simular "Oi, queria um orçamento"
```

Mapa dos arquivos:

| Arquivo | O que tem |
| --- | --- |
| `main.go` | início do programa e o modo `-simular` |
| `estado.go` | dados do negócio, leads e gravação em disco |
| `modelos.go` | textos de fábrica, variáveis e montagem das instruções |
| `cerebro.go` | chamada ao `claude`, reserva pela API e modo teste |
| `ia.go` | ajudas da IA: preencher cadastro, melhorar texto, montar cérebro, resumir material |
| `conhecimento.go` | leitura de PDF, DOCX e PPTX, trechos e busca |
| `agenda.go` | horários livres, marcação e cancelamento |
| `midia.go` | mídias recebidas e enviadas, fotos de perfil, etiquetas e relatório |
| `whats.go` | conexão, QR, mensagens, limites e avisos |
| `painel.go` | servidor local e rotas |
| `web/` | a página: HTML, CSS e JavaScript puros, sem etapa de build. As bibliotecas de terceiros ficam em `web/vendor/`, com as licenças em `web/vendor/LEIA-ME.md` |

## Licença

MIT. Veja o arquivo [LICENSE](LICENSE).
