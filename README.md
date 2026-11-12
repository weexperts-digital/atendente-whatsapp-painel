# Atendente de WhatsApp com IA

Um atendente de inteligência artificial para o WhatsApp do seu negócio. Ele conversa com quem escreve, entende o que a pessoa precisa, separa os leads em quente, morno e frio, marca horários na agenda e chama você quando é hora de uma pessoa assumir.

Roda no seu computador. O cérebro é o Claude, pela sua conta do Claude Code. Não tem servidor para contratar e não tem mensalidade deste projeto. O painel tem login e senha, e tudo fica guardado em um banco de dados no próprio computador.

Esta é a versão com painel completo. A versão mais simples, usada na palestra do SM Summit, fica em [weexperts-digital/atendente-whatsapp](https://github.com/weexperts-digital/atendente-whatsapp).

## O que ele faz

- **Início**: a primeira tela do painel. Mostra se está tudo funcionando (atendente, WhatsApp, IA e agenda), os números de hoje e da semana, quem está esperando uma pessoa, os próximos horários e as últimas conversas.
- **Conversas**: uma tela com cara de WhatsApp, com foto do contato, áudio, imagem, vídeo e PDF dentro da conversa. Quem foi transferido entra em uma fila; a pessoa da equipe clica em Iniciar atendimento, responde por texto, arquivo ou áudio, e depois devolve ao assistente. O cliente não vê quem está atendendo.
- **Áudios**: quando o cliente manda um áudio, o assistente ouve, transcreve no seu próprio computador e responde por texto. A transcrição aparece na conversa, embaixo do áudio. Sem chave e sem enviar o áudio para fora.
- **Contatos**: todo mundo que já escreveu para o negócio em uma tabela, com busca, filtros e planilha para baixar. Clicar em um contato abre a ficha dele: dados, o que a IA já entendeu, etiquetas, anotações da equipe, agendamentos e o histórico, para fazer o acompanhamento.
- **Login e equipe**: o primeiro acesso cria a conta do super administrador. Ele cria as contas da equipe (administrador ou atendente) e vê o registro de quem entrou e do que cada pessoa fez.
- **Banco de dados**: contatos, todas as mensagens, agenda e contas ficam em um banco SQLite dentro da pasta do programa, com uma cópia de segurança por dia. Não precisa instalar nem configurar nada.
- **Etiquetas e relatório**: cada etiqueta existe uma vez só. A IA usa as que já existem, cria uma nova só quando nenhuma serve e troca conforme o cliente avança (de "quer agendar" para "agendado", por exemplo). Você renomeia, junta e apaga em um lugar só. Veja também um relatório simples de conversas, transferências, agendamentos e respostas.
- **Equipe e IA na mesma conversa**: tudo o que a equipe conversa com o cliente fica guardado. Quando a conversa volta para o assistente, ele lê o atendimento inteiro e continua de onde a equipe parou.
- **Claude ou ChatGPT**: conecte o Claude com o login da sua conta, por um botão, ou use o ChatGPT com uma chave da OpenAI.
- **Qualificação de leads**: pontua cada pessoa de 0 a 100 e avisa no seu WhatsApp quando o lead esquenta ou pede uma pessoa.
- **Agenda**: um calendário de semana, dia e mês. Os dias, horários, pausas de almoço e serviços que você informa na configuração inicial já valem no calendário. O assistente oferece os horários livres e marca, remarca e desmarca sozinho, sem chamar uma pessoa.
- **Base de conhecimento**: envie PDF, Word, PowerPoint ou texto. A IA lê, resume e passa a responder com base nos seus materiais.
- **Assistente editável**: personalidade, cérebro e regras de transbordo em abas separadas, com variáveis como `{{NEGOCIO}}` e um botão para pedir mudanças à IA.
- **Treinar**: converse com o assistente dentro do painel, veja o que ele entendeu e corrija as respostas. As correções viram aprendizados.

## Do que você precisa

1. Um computador com Mac, Linux ou Windows.
2. O [Claude Code](https://claude.com/claude-code) instalado. É ele que pensa as respostas, com a sua conta do Claude (plano pago). Você conecta a conta por um botão do painel, sem terminal. Se preferir, use o ChatGPT com uma chave da OpenAI.
3. Um número de WhatsApp para o negócio. De preferência um número só para isso.
4. Para o assistente ouvir áudios: o Google Chrome, o Microsoft Edge ou o Brave instalado. É quase certo que você já tem um deles. Sem isso tudo funciona, mas o assistente pede para o cliente escrever.

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

1. Crie a conta do super administrador: seu nome, e-mail e uma senha, digitada duas vezes. Guarde bem essa senha.
2. Conecte a IA: clique em Conectar o Claude, entre na sua conta do Claude no navegador e autorize. Não precisa de terminal. Se preferir o ChatGPT, escolha a aba ChatGPT e cole uma chave da OpenAI.
3. Conte sobre o seu negócio. Dá para escrever em poucas palavras, ou falar no microfone, e deixar a IA preencher os campos. A voz é entendida no seu próprio computador, sem chave e sem enviar o áudio para fora.
4. Confira os horários: dias, hora de abrir e fechar, pausa do almoço e serviços. Diga se o negócio atende com hora marcada.
5. Escolha o nome e o tom do assistente.
6. Informe o WhatsApp que recebe os avisos de lead quente.
7. Leia o código QR com o WhatsApp do negócio (WhatsApp, Configurações, Aparelhos conectados, Conectar aparelho).
8. Ligue o atendente e mande um "Oi" de outro celular.

Para dar acesso a outras pessoas, vá em Ajustes, Equipe e acessos.

Depois disso:

| Para | Faça |
| --- | --- |
| Ligar de novo depois de reiniciar o computador | `./ligar.sh` (ou peça ao Claude Code: ligue o atendente) |
| Parar | `./parar.sh` |
| Ver o que aconteceu | abra o arquivo `atendente.log` |
| Atualizar para a versão mais nova | `./atualizar.sh` (ou peça ao Claude Code: atualize o atendente). Seus dados e a conexão do WhatsApp continuam |
| Montar de novo depois de mudar o código | `./instalar.sh` |
| O assistente respondeu que só lê texto quando mandaram áudio | Veja em Ajustes, Cérebro, a linha Áudios do WhatsApp. Ela diz o que falta |
| Aparece a faixa vermelha "A IA está desconectada" | Clique em Conectar a IA, entre na sua conta do Claude no navegador e autorize. A tela avisa quando terminar. Enquanto isso, as conversas novas vão para a fila da equipe. Plano B: `claude auth login` no terminal |
| Esqueci a senha do super administrador | `./atendente -esqueci-senha` e recarregue a página (ou peça ao Claude Code: esqueci a senha do painel) |

Se o computador dormir ou desligar, o atendente para. Ele volta quando você ligar de novo.

## Como funciona por dentro

```
WhatsApp  ->  atendente (Go)  ->  claude -p (sem ferramentas)  ->  resposta
                   |
                   +-- painel local em 127.0.0.1 (pasta web/)
                   +-- dados/atendente.db (banco de dados, só no seu computador)
```

- **Um programa só**, em Go, com a página embutida. A conexão com o WhatsApp usa a biblioteca [whatsmeow](https://github.com/tulir/whatsmeow).
- **A cada mensagem** o programa monta as instruções com a personalidade, o cérebro, as regras de transbordo, os aprendizados, os trechos dos seus materiais que combinam com a conversa e os horários livres da agenda. Depois chama o `claude` e recebe a resposta em um formato fixo (resposta, dados do lead, transbordo, agendamento).
- **Base de conhecimento**: o texto dos arquivos é extraído no próprio programa e dividido em trechos. Bases pequenas vão inteiras nas instruções. Bases grandes passam por uma busca por palavras (BM25) e só os trechos mais ligados à conversa entram.
- **Banco de dados**: um arquivo SQLite, `dados/atendente.db`, criado pelo próprio programa. Guarda os dados do negócio, os contatos, todas as mensagens, a agenda, as contas do painel e o registro de acessos. Todo dia uma cópia vai para `dados/copias/`.
- **Seus dados** ficam na pasta `dados/`, que nunca vai para o repositório.

A descrição das rotas do painel está em [docs/API.md](docs/API.md).

## Segurança

- O assistente só responde conversas privadas, e só a quem escreveu primeiro. Ignora grupos, status e canais. As únicas mensagens que ele inicia são os avisos para o número que você cadastrou.
- Nenhuma mensagem de WhatsApp consegue executar comandos no seu computador. O Claude é chamado sem nenhuma ferramenta (`--tools ""`), em uma pasta vazia, e só lê texto.
- Fotos, áudios, vídeos e documentos que os clientes mandam são baixados somente dos servidores do WhatsApp e guardados em `dados/midia`, para a equipe ver no painel. O programa não executa esses arquivos nem os entrega a outros programas. As fotos de perfil vêm do mesmo lugar.
- Os áudios dos clientes são transcritos no seu computador: o programa decodifica o som e o Whisper transforma em texto dentro de um navegador escondido, que o programa abre e fecha sozinho. Não há chave e o áudio não sai do computador. O texto transcrito é tratado como qualquer mensagem: um dado, nunca uma ordem.
- O painel escuta só em `127.0.0.1`. Ninguém de fora do computador consegue abrir.
- O painel pede login. As senhas ficam guardadas só como hash (Argon2id, com sal), nunca em texto. A sessão vai em um cookie que o JavaScript não lê e que não acompanha pedidos de outros sites. Cinco senhas erradas seguidas seguram o e-mail por 15 minutos.
- Cada tipo de acesso alcança só as suas rotas, e isso é conferido no programa, não só na tela. Entradas, senhas erradas, atendimentos, ajustes e planilhas baixadas ficam no registro de acessos.
- Não existe recuperação de senha por e-mail. Quem perde a senha do super administrador precisa estar no computador para criar outra.
- Nenhuma chave de API é gravada em arquivo. A chave da OpenAI, quando usada, fica no cofre de senhas do sistema (Keychain no Mac, Gerenciador de Credenciais no Windows) e a página só mostra as 4 últimas letras.
- Com o ChatGPT, vale a mesma regra do Claude: o pedido leva só texto e não declara nenhuma ferramenta.
- A voz do microfone é entendida no seu próprio navegador (Whisper), sem chave. O áudio não é gravado em disco nem enviado para fora. Na primeira vez, o programa baixa 267 MB com o modelo de voz e confere cada arquivo antes de usar.
- O texto das conversas e dos materiais é enviado ao Claude, ou à OpenAI se você escolher o ChatGPT, para gerar as respostas. Fora isso, nada sai do computador.

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
| `ATENDENTE_NAVEGADOR` | Navegador da família do Chrome usado, escondido, para transcrever os áudios (o padrão é achar o Chrome, Edge, Brave ou Chromium instalado) |

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
| `banco.go` | banco de dados SQLite, gravação do que mudou e cópia diária |
| `acesso.go` | contas, senhas, sessões, permissões por rota e registro de acessos |
| `contatos.go` | lista de contatos, filtros e planilha |
| `painel.go` | servidor local e rotas |
| `web/` | a página: HTML, CSS e JavaScript puros, sem etapa de build. As bibliotecas de terceiros ficam em `web/vendor/`, com as licenças em `web/vendor/LEIA-ME.md` |

## Licença

MIT. Veja o arquivo [LICENSE](LICENSE).
