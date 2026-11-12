'use strict';

// Reconhecimento de voz local. Ver voz.go e voz-worker.js.
//
// A pessoa clica no microfone, fala, e o texto aparece. Quem entende a fala é o Whisper,
// rodando dentro desta página (WebAssembly), em um Web Worker. Não há chave, e o áudio
// nunca sai deste computador: nada aqui fala com a internet, só com o próprio painel.
//
//   VOZ.botao(opc)        devolve o botão pronto para ficar ao lado de um campo de texto.
//     opc.aoTexto(texto, final)  texto inteiro desta gravação até agora. Chega por trechos
//                                (final = false) e uma última vez com final = true.
//     opc.aoEstado(estado, info) 'preparando' (baixando, só na primeira vez) | 'pronto' |
//                                'abrindo' (pedindo o microfone) | 'gravando' | 'entendendo' | 'erro'.
//                                info: { progresso } ao preparar, { mensagem } no erro.
//     opc.limiteSeg              tempo máximo de gravação, em segundos (padrão 180).
//     opc.texto                  o que vai escrito no botão (padrão "Falar").
//     O elemento devolvido tem .voz = { parar(), cancelar(), estado() }.
//   VOZ.transcrever(audio, aoParcial)  transcreve um áudio pronto e devolve o texto.
//     audio: Float32Array (mono, 16 kHz) ou um arquivo de áudio (Blob ou ArrayBuffer).
const VOZ = (() => {
  const TAXA = 16000;            // o Whisper ouve em mono, 16 kHz
  const LIMITE_PADRAO = 180;     // 3 minutos por gravação
  const AVISO_ANTES = 30;        // avisa quando faltam 30 segundos
  const DESCANSO_MS = 4 * 60 * 1000; // sem uso por esse tempo, o Whisper sai da memória

  const AVISOS = {
    semSuporte: 'Este navegador não consegue usar a voz. Atualize o navegador ou use o Chrome.',
    bloqueado: 'O navegador bloqueou o microfone. Clique no cadeado ao lado do endereço da página, permita o microfone e tente de novo.',
    semMicrofone: 'Não encontrei um microfone neste computador. Ligue um microfone e tente de novo.',
    emUso: 'Outro programa está usando o microfone. Feche o outro programa e tente de novo.',
    microfone: 'Não consegui usar o microfone. Tente de novo.',
    nada: 'Não ouvi nada. Chegue mais perto do microfone e tente de novo.',
    motor: 'Não consegui ligar a voz neste computador. Feche outras abas do navegador e tente de novo.',
    entender: 'Não consegui entender o áudio. Tente de novo.',
    outra: 'Já tem uma gravação em andamento. Termine a outra primeiro.',
    preparo: 'Não consegui preparar a voz. Confira a internet e tente de novo.',
    semAudio: 'O navegador não ligou o áudio. Clique de novo para falar.',
    arquivo: 'Não consegui abrir este áudio. Tente outro arquivo.',
    aguarde: 'A voz ainda está sendo preparada. Espere um pouco e tente de novo.',
  };

  const erroDeVoz = (mensagem) => Object.assign(new Error(mensagem), { deVoz: true });
  const avisoDe = (e, padrao) => (e && e.deVoz ? e.message : padrao);

  function suportado() {
    return !!(navigator.mediaDevices && navigator.mediaDevices.getUserMedia
      && (window.AudioContext || window.webkitAudioContext)
      && window.Worker && window.WebAssembly);
  }

  // ---------- O que o programa já baixou (/api/voz/estado) ----------

  let servidor = null;            // última resposta
  let acompanhando = null;        // relógio enquanto baixa
  const interessados = new Set(); // botões na tela, avisados a cada mudança

  async function consultar() {
    try {
      servidor = await api.get('/api/voz/estado');
    } catch (e) {
      servidor = { pronto: false, baixando: false, progresso: 0, erro: e.message || AVISOS.preparo, semResposta: true };
    }
    for (const avisar of [...interessados]) {
      try { avisar(servidor); } catch (e) { console.error(e); }
    }
    clearTimeout(acompanhando);
    if (servidor.baixando && interessados.size) acompanhando = setTimeout(consultar, 1200);
    return servidor;
  }

  // preparar pede ao programa para baixar (ou tentar de novo) o que a voz precisa.
  async function preparar() {
    try {
      servidor = await api.post('/api/voz/preparar');
    } catch (e) {
      servidor = { pronto: false, baixando: false, progresso: 0, erro: e.message || AVISOS.preparo, semResposta: true };
    }
    return consultar();
  }

  // ---------- O Whisper, em um Web Worker ----------

  let worker = null;
  let carregado = null;   // promessa: o Whisper está na memória
  let numero = 0;
  let descanso = null;
  let emUso = 0;          // gravações e transcrições em andamento
  const pedidos = new Map();

  function desligarMotor(erro) {
    clearTimeout(descanso);
    if (worker) worker.terminate();
    worker = null;
    carregado = null;
    for (const p of pedidos.values()) p.falha(erro || erroDeVoz(AVISOS.entender));
    pedidos.clear();
  }

  function ligarMotor() {
    if (carregado) return carregado;
    let pronto;
    let falhou;
    carregado = new Promise((ok, falha) => { pronto = ok; falhou = falha; });
    carregado.catch(() => { /* quem pediu é avisado em transcreverTrecho */ });
    try {
      worker = new Worker('/voz-worker.js', { type: 'module' });
    } catch (e) {
      carregado = null;
      return Promise.reject(erroDeVoz(AVISOS.semSuporte));
    }
    const quebrou = (detalhe) => {
      console.error('Voz: ' + detalhe);
      const erro = erroDeVoz(AVISOS.motor);
      falhou(erro);
      desligarMotor(erro);
    };
    worker.onmessage = (ev) => {
      const m = ev.data || {};
      if (m.tipo === 'carregado') { pronto(); return; }
      if (m.tipo === 'falha' && m.id == null) { quebrou(m.erro); return; }
      const p = pedidos.get(m.id);
      if (!p) return;
      pedidos.delete(m.id);
      if (m.tipo === 'texto') p.ok(m);
      else { console.error('Voz: ' + m.erro); p.falha(erroDeVoz(AVISOS.entender)); }
    };
    worker.onerror = (ev) => { if (ev && ev.preventDefault) ev.preventDefault(); quebrou((ev && ev.message) || 'o worker parou'); };
    worker.postMessage({ tipo: 'carregar', arquivos: servidor && servidor.arquivos });
    return carregado;
  }

  function marcarUso(delta) {
    emUso += delta;
    clearTimeout(descanso);
    if (emUso <= 0) {
      emUso = 0;
      descanso = setTimeout(() => { if (!emUso && !pedidos.size) desligarMotor(); }, DESCANSO_MS);
    }
  }

  // transcreverTrecho manda até 30 s de áudio (16 kHz) para o Whisper e devolve o texto.
  async function transcreverTrecho(audio) {
    await ligarMotor();
    return new Promise((ok, falha) => {
      const id = ++numero;
      pedidos.set(id, { ok, falha });
      worker.postMessage({ tipo: 'transcrever', id, audio }, [audio.buffer]);
    });
  }

  // ---------- Limpeza do texto ----------

  // Sem fala nenhuma, o Whisper às vezes inventa créditos de legenda e marcas de música. Isso sai.
  const INVENTADO = /\s*legendas?\s+(pela|da)\s+comunidade(\s+(da\s+)?amara\.org)?[.!]?|\s*\bamara\.org\b[.!]?/gi;

  function limpar(texto) {
    return String(texto || '').replace(/\[[^\]]*\]/g, ' ').replace(/[♪♫]+/g, ' ').replace(INVENTADO, ' ').replace(/\s+/g, ' ').trim();
  }

  // ---------- Fila: os trechos entram em ordem e o texto vai saindo ----------
  // O Whisper acerta mais quando ouve frases inteiras, com contexto. Por isso cada trecho novo
  // é transcrito junto com os anteriores, em uma "janela" que cresce até 29 s, e o texto da
  // janela inteira é trocado pelo novo. Quando a janela enche, ela fecha e começa outra.
  // Se o computador for devagar, os trechos se acumulam e vão vários de uma vez (menos pedidos).

  function criarFila(aoParcial, aoFalhar) {
    const JANELA = 29 * TAXA;
    const espera = [];    // trechos que ainda não foram para o Whisper
    const janelas = [];   // { audio (só a última guarda), texto }
    let ocupada = false;
    let fechada = false;
    let cancelada = false;
    let erro = null;
    let acabar = null;

    const junto = () => janelas.map((j) => j.texto).filter(Boolean).join(' ');

    function conferirFim() {
      if (!acabar || ocupada) return;
      if (erro) acabar.falha(erro);
      else if (!espera.length) acabar.ok(junto());
    }

    async function andar() {
      if (ocupada || cancelada || erro || !espera.length) { conferirFim(); return; }
      // Continua a última janela só se tudo o que está esperando couber nela. Se não couber
      // (janela cheia, ou computador devagar com trechos acumulados), abre outra: repetir a
      // janela antiga para ganhar poucos segundos novos custaria um pedido inteiro.
      let janela = janelas[janelas.length - 1];
      const esperando = espera.reduce((soma, trecho) => soma + trecho.length, 0);
      if (!janela || !janela.audio || janela.audio.length + esperando > JANELA) {
        if (janela) janela.audio = null;
        janela = null;
      }
      let total = janela ? janela.audio.length : 0;
      let quantos = 0;
      while (quantos < espera.length && (total + espera[quantos].length <= JANELA || (!janela && quantos === 0))) {
        total += espera[quantos].length;
        quantos++;
      }
      const partes = espera.splice(0, quantos);
      const audio = new Float32Array(total);
      let pos = 0;
      if (janela) { audio.set(janela.audio); pos = janela.audio.length; }
      for (const parte of partes) { audio.set(parte, pos); pos += parte.length; }
      ocupada = true;
      try {
        const r = await transcreverTrecho(audio.slice()); // a cópia vai para o worker, o original fica
        if (cancelada) return;
        if (!janela) { janela = { audio: null, texto: '' }; janelas.push(janela); }
        janela.audio = audio;
        janela.texto = limpar(r.texto);
        if (aoParcial && junto()) aoParcial(junto());
      } catch (e) {
        erro = e;
        if (!cancelada && !fechada && aoFalhar) aoFalhar(e);
      } finally {
        ocupada = false;
      }
      andar();
    }

    return {
      por(...trechos) { if (!fechada && !cancelada) { espera.push(...trechos); andar(); } },
      // fechar: não entra mais nada. Devolve o texto inteiro quando o último trecho sair.
      fechar() {
        fechada = true;
        return new Promise((ok, falha) => { acabar = { ok, falha }; conferirFim(); });
      },
      cancelar() { cancelada = true; espera.length = 0; if (acabar) acabar.ok(junto()); },
      texto: junto,
    };
  }

  // ---------- Cortar a fala em trechos ----------
  // O Whisper lê no máximo 30 s por vez. O corte acontece nas pausas da fala, para não
  // partir uma palavra ao meio, e o silêncio fica de fora (sem fala, o Whisper inventa texto).
  //   aoVivo = true: corta em toda pausa boa, para o texto ir aparecendo enquanto a pessoa fala.
  //   aoVivo = false (áudio pronto): trechos do maior tamanho possível, que é mais rápido no total.

  function criarSegmentador(aoTrecho, aoVivo) {
    const QUADRO = 480;             // 30 ms
    const MAXIMO = 28 * TAXA;       // nenhum trecho passa de 28 s
    const MIN_FALA = 5;             // menos que 150 ms de som é estalo, não fala
    const buf = new Float32Array(MAXIMO + TAXA);
    let n = 0;                      // amostras guardadas
    let medido = 0;                 // até onde já foi medido
    let comVoz = [];                // um item por quadro: tem voz ou não
    let forca = [];                 // um item por quadro: volume
    let falas = 0;                  // quadros com voz no trecho
    let ultimaFala = -1;            // último quadro com voz
    let piso = -1;                  // ruído de fundo, medido o tempo todo (o primeiro quadro dá a partida)

    function recontar() {
      falas = 0;
      ultimaFala = -1;
      for (let i = 0; i < comVoz.length; i++) if (comVoz[i]) { falas++; ultimaFala = i; }
    }

    // tirar joga fora os primeiros quadros. Com entregar = true, eles viram um trecho.
    function tirar(quadros, entregar) {
      const fim = Math.min(quadros * QUADRO, n);
      if (fim <= 0) return;
      if (entregar) aoTrecho(buf.slice(0, fim));
      buf.copyWithin(0, fim, n);
      n -= fim;
      medido -= fim;
      comVoz = comVoz.slice(quadros);
      forca = forca.slice(quadros);
      recontar();
    }

    function decidir() {
      const q = comVoz.length;
      const silencio = q - 1 - ultimaFala; // quadros quietos no fim
      if (falas < MIN_FALA) {
        // Só silêncio: guarda 0,3 s, para não cortar o começo da fala. Um estalo solto sai depois de 1,5 s.
        if (ultimaFala < 0 ? q > 10 : silencio > 50) tirar(q - 10, false);
        return;
      }
      const segundos = q * QUADRO / TAXA;
      // Quanto mais longo o trecho, menor a pausa que já serve para cortar:
      // 0,9 s (trecho de 2 s), 0,6 s (4 s), 0,36 s (12 s) ou 0,21 s (20 s). Com 2 s de silêncio, sempre.
      const pausa = segundos >= 20 ? 7 : segundos >= 12 ? 12 : segundos >= 4 ? 20 : 30;
      if ((aoVivo && segundos >= 2 && silencio >= pausa) || silencio >= 66) {
        tirar(ultimaFala + 1 + Math.min(silencio, 8), true);
        return;
      }
      if (n >= MAXIMO) {
        // Chegou a 28 s: corta na maior pausa dos últimos 8 s (no meio dela).
        const desde = Math.max(1, q - 266);
        let melhor = -1;
        let maior = 2; // pausa de menos de 90 ms não conta: pode ser o meio de uma palavra
        for (let i = desde, corrida = 0; i < q; i++) {
          corrida = comVoz[i] ? 0 : corrida + 1;
          if (corrida >= maior) { maior = corrida; melhor = i - (corrida >> 1); }
        }
        if (melhor < 0) {
          // Falou sem parar: corta no ponto mais fraco.
          let menor = Infinity;
          for (let i = desde; i < q - 2; i++) {
            const v = forca[i - 1] + forca[i] + forca[i + 1];
            if (v <= menor) { menor = v; melhor = i; }
          }
        }
        tirar(melhor + 1, true);
      }
    }

    function medir() {
      while (medido + QUADRO <= n) {
        let soma = 0;
        for (let i = medido; i < medido + QUADRO; i++) soma += buf[i] * buf[i];
        const rms = Math.sqrt(soma / QUADRO);
        // O piso desce rápido e sobe devagar: acompanha o ruído da sala, não a fala.
        if (piso < 0) piso = Math.min(Math.max(rms, 0.0005), 0.02);
        else piso = rms < piso ? (piso + rms) / 2 : Math.min(piso * 1.003, 0.02);
        // Na dúvida, conta como voz: é melhor mandar ruído ao Whisper do que perder uma palavra.
        const limiar = Math.min(Math.max(piso * 2.5, 0.003), 0.03);
        const temVoz = rms > limiar;
        comVoz.push(temVoz);
        forca.push(rms);
        if (temVoz) { falas++; ultimaFala = comVoz.length - 1; }
        medido += QUADRO;
        decidir();
      }
    }

    return {
      alimentar(amostras) {
        let i = 0;
        while (i < amostras.length) {
          const cabe = Math.min(amostras.length - i, buf.length - n);
          if (cabe <= 0) { tirar(comVoz.length, falas >= MIN_FALA); n = 0; medido = 0; continue; } // não deve acontecer
          buf.set(amostras.subarray(i, i + cabe), n);
          n += cabe;
          i += cabe;
          medir();
        }
      },
      // fechar entrega o que sobrou, se tiver fala.
      fechar() {
        if (falas >= MIN_FALA) {
          const fim = Math.min(n, (ultimaFala + 11) * QUADRO);
          aoTrecho(buf.slice(0, fim));
        }
        n = 0; medido = 0; comVoz = []; forca = []; falas = 0; ultimaFala = -1;
      },
    };
  }

  // ---------- Converter o som do microfone para 16 kHz ----------
  // O microfone entrega 44,1 ou 48 kHz. Um filtro passa-baixas (sinc com janela) tira o que
  // o Whisper não usa e evita chiado na conversão.

  function criarConversor(taxaDeEntrada) {
    if (taxaDeEntrada === TAXA) return (bloco) => bloco;
    const passo = taxaDeEntrada / TAXA;                        // amostras de entrada por amostra de saída
    const corte = 0.45 * Math.min(1, TAXA / taxaDeEntrada);    // em ciclos por amostra de entrada
    const meia = Math.ceil(10 / (2 * corte));                  // metade do filtro: 10 cruzamentos por zero
    const FINO = 32;                                           // pontos da tabela por amostra
    const tabela = new Float32Array(meia * FINO + 2);
    for (let i = 0; i <= meia * FINO; i++) {
      const x = i / FINO;
      const a = 2 * Math.PI * corte * x;
      const sinc = a === 0 ? 1 : Math.sin(a) / a;
      tabela[i] = 2 * corte * sinc * (0.5 + 0.5 * Math.cos(Math.PI * x / meia));
    }
    let resto = new Float32Array(meia); // fim do bloco anterior (começa em silêncio)
    let p = meia;                       // posição da próxima amostra de saída, dentro de `junto`

    return (bloco) => {
      const junto = new Float32Array(resto.length + bloco.length);
      junto.set(resto);
      junto.set(bloco, resto.length);
      const saida = new Float32Array(Math.max(0, Math.ceil((junto.length - meia - p) / passo)) + 1);
      let m = 0;
      while (p + meia < junto.length) {
        const centro = Math.floor(p);
        let soma = 0;
        let peso = 0;
        for (let j = centro - meia + 1; j <= centro + meia; j++) {
          const x = Math.abs(p - j) * FINO;
          const k = x | 0;
          const w = tabela[k] + (tabela[k + 1] - tabela[k]) * (x - k);
          soma += w * junto[j];
          peso += w;
        }
        saida[m++] = soma / peso;
        p += passo;
      }
      const fora = Math.max(0, Math.floor(p) - meia);
      resto = junto.slice(fora);
      p -= fora;
      return saida.subarray(0, m);
    };
  }

  // ---------- Microfone ----------

  function avisoDoMicrofone(e) {
    const nome = e && e.name;
    if (nome === 'NotAllowedError' || nome === 'SecurityError') return AVISOS.bloqueado;
    if (nome === 'NotFoundError' || nome === 'OverconstrainedError') return AVISOS.semMicrofone;
    if (nome === 'NotReadableError' || nome === 'AbortError') return AVISOS.emUso;
    return AVISOS.microfone;
  }

  // abrirMicrofone começa a ouvir e entrega o som em blocos de 16 kHz para aoSom(bloco).
  // Devolve { fechar() }. Chame dentro do clique: o navegador exige isso para ligar o áudio.
  async function abrirMicrofone(aoSom) {
    const Contexto = window.AudioContext || window.webkitAudioContext;
    let contexto = null;
    let fluxo = null;
    const fechar = () => {
      if (fluxo) for (const faixa of fluxo.getTracks()) { try { faixa.stop(); } catch (e) { /* já parou */ } }
      fluxo = null;
      if (contexto && contexto.state !== 'closed') contexto.close().catch(() => {});
      contexto = null;
    };
    try {
      contexto = new Contexto(); // criado antes da permissão, ainda dentro do clique
      try {
        fluxo = await navigator.mediaDevices.getUserMedia({
          // Sem cancelamento de eco: aqui nada toca no alto-falante, e ele mexe no som do computador.
          audio: { channelCount: 1, echoCancellation: false, noiseSuppression: true, autoGainControl: true },
        });
      } catch (e) {
        throw erroDeVoz(avisoDoMicrofone(e));
      }
      if (contexto.state !== 'running') {
        await Promise.race([contexto.resume(), new Promise((ok) => setTimeout(ok, 1500))]);
        if (contexto.state !== 'running') throw erroDeVoz(AVISOS.semAudio);
      }
      const converter = criarConversor(contexto.sampleRate);
      const receber = (bloco) => { if (fluxo) aoSom(converter(bloco)); };
      const fonte = contexto.createMediaStreamSource(fluxo);
      const mudo = contexto.createGain(); // mantém o áudio andando sem tocar nada no alto-falante
      mudo.gain.value = 0;
      let no = null;
      try {
        if (!contexto.audioWorklet || !window.AudioWorkletNode) throw new Error('sem AudioWorklet');
        await contexto.audioWorklet.addModule('/voz-worker.js');
        no = new AudioWorkletNode(contexto, 'voz-captura', {
          numberOfInputs: 1, numberOfOutputs: 1, outputChannelCount: [1],
          channelCount: 1, channelCountMode: 'explicit', channelInterpretation: 'speakers',
        });
        no.port.onmessage = (ev) => receber(ev.data);
      } catch (e) {
        // Navegador antigo: o jeito de antes, que roda junto com a tela.
        if (!contexto.createScriptProcessor) throw erroDeVoz(AVISOS.semSuporte);
        no = contexto.createScriptProcessor(4096, 1, 1);
        no.onaudioprocess = (ev) => receber(new Float32Array(ev.inputBuffer.getChannelData(0)));
      }
      fonte.connect(no);
      no.connect(mudo);
      mudo.connect(contexto.destination);
      return { fechar };
    } catch (e) {
      fechar();
      throw e.deVoz ? e : erroDeVoz(AVISOS.microfone);
    }
  }

  // ---------- Uma gravação ----------

  let gravando = null; // só uma gravação por vez na página

  // gravar abre o microfone e já vai transcrevendo. Devolve { parar(), cancelar() }.
  //   parar() devolve o texto inteiro. aoParcial(texto) roda a cada trecho entendido.
  //   aoNivel(0 a 1) roda várias vezes por segundo, com o volume do que o microfone ouve.
  //   aoFalhar(erro) roda se o Whisper falhar no meio (a gravação deve ser cancelada).
  async function gravar(aoParcial, aoNivel, aoFalhar) {
    if (gravando) throw erroDeVoz(AVISOS.outra);
    const ficha = {};
    gravando = ficha;
    const fila = criarFila(aoParcial, aoFalhar);
    const cortador = criarSegmentador((trecho) => fila.por(trecho), true);
    let microfone = null;
    try {
      microfone = await abrirMicrofone((bloco) => {
        if (gravando !== ficha) return;
        cortador.alimentar(bloco);
        if (aoNivel && bloco.length) {
          let soma = 0;
          for (let i = 0; i < bloco.length; i++) soma += bloco[i] * bloco[i];
          aoNivel(Math.min(1, Math.sqrt(Math.sqrt(soma / bloco.length)) * 2.2));
        }
      });
    } catch (e) {
      if (gravando === ficha) gravando = null;
      throw e;
    }
    if (gravando !== ficha) { microfone.fechar(); throw erroDeVoz(AVISOS.microfone); } // cancelada enquanto abria
    marcarUso(1);
    let encerrada = false;
    // O Whisper carrega enquanto a pessoa fala.
    ligarMotor().catch((e) => { if (!encerrada && aoFalhar) aoFalhar(e); });
    const soltar = () => {
      if (encerrada) return false;
      encerrada = true;
      microfone.fechar();
      if (gravando === ficha) gravando = null;
      return true;
    };
    return {
      async parar() {
        if (!soltar()) return fila.texto();
        cortador.fechar();
        try {
          const texto = await fila.fechar();
          if (!texto) throw erroDeVoz(AVISOS.nada);
          return texto;
        } finally {
          marcarUso(-1);
        }
      },
      cancelar() {
        if (!soltar()) return;
        fila.cancelar();
        marcarUso(-1);
      },
      texto: fila.texto,
    };
  }

  // ---------- Áudio pronto (arquivo ou amostras) ----------

  async function decodificar(arquivo) {
    const dados = arquivo instanceof ArrayBuffer ? arquivo.slice(0) : await arquivo.arrayBuffer();
    const Offline = window.OfflineAudioContext || window.webkitOfflineAudioContext;
    // Um contexto de 16 kHz já entrega o áudio decodificado nessa taxa.
    const audio = await new Offline(1, 1, TAXA).decodeAudioData(dados);
    const mono = new Float32Array(audio.length);
    for (let c = 0; c < audio.numberOfChannels; c++) {
      const canal = audio.getChannelData(c);
      for (let i = 0; i < mono.length; i++) mono[i] += canal[i] / audio.numberOfChannels;
    }
    return mono;
  }

  async function transcrever(audio, aoParcial) {
    if (!servidor || !servidor.pronto) await consultar();
    if (!servidor.pronto) throw erroDeVoz(servidor.erro || AVISOS.aguarde);
    let amostras;
    try {
      amostras = audio instanceof Float32Array ? audio : await decodificar(audio);
    } catch (e) {
      throw erroDeVoz(AVISOS.arquivo);
    }
    const fila = criarFila(aoParcial);
    const trechos = [];
    const cortador = criarSegmentador((trecho) => trechos.push(trecho), false);
    marcarUso(1);
    try {
      for (let i = 0; i < amostras.length; i += TAXA) {
        cortador.alimentar(amostras.subarray(i, i + TAXA));
        if (i % (20 * TAXA) === 0) await new Promise((ok) => setTimeout(ok)); // deixa a tela respirar
      }
      cortador.fechar();
      fila.por(...trechos); // todos de uma vez: a fila junta os vizinhos em pedidos de até 29 s
      return await fila.fechar();
    } finally {
      marcarUso(-1);
    }
  }

  // ---------- O botão ----------

  const SVG_NS = 'http://www.w3.org/2000/svg';
  const DESENHOS = {
    microfone: [['path', { d: 'M12 2a3 3 0 0 0-3 3v7a3 3 0 0 0 6 0V5a3 3 0 0 0-3-3Z' }], ['path', { d: 'M19 10v2a7 7 0 0 1-14 0v-2M12 19v3' }]],
    parar: [['rect', { x: 6, y: 6, width: 12, height: 12, rx: 2 }]],
  };

  function desenho(nome) {
    const svg = document.createElementNS(SVG_NS, 'svg');
    svg.setAttribute('class', 'ic');
    svg.setAttribute('viewBox', '0 0 24 24');
    svg.setAttribute('aria-hidden', 'true');
    svg.setAttribute('focusable', 'false');
    for (const [tag, attrs] of DESENHOS[nome]) {
      const el = document.createElementNS(SVG_NS, tag);
      for (const [k, v] of Object.entries(attrs)) el.setAttribute(k, v);
      svg.appendChild(el);
    }
    return svg;
  }

  const relogio = (seg) => Math.floor(seg / 60) + ':' + String(Math.floor(seg % 60)).padStart(2, '0');
  const porExtenso = (seg) => (seg % 60 === 0 ? (seg / 60) + (seg === 60 ? ' minuto' : ' minutos') : seg + ' segundos');

  function botao(opc) {
    opc = opc || {};
    const limite = Math.max(10, Math.round(Number(opc.limiteSeg) || LIMITE_PADRAO));
    const rotulo = opc.texto || 'Falar';
    let estado = 'pronto';
    let gravacao = null;
    let tique = null;
    let inicio = 0;
    let nivel = 0;
    let quadro = 0;
    let entregou = false;        // já mandou algum trecho de texto nesta gravação
    let chamando = false;        // um clique já está sendo atendido
    let viuDownload = false;     // a pessoa viu o preparo: avisa quando terminar
    let esteveNaTela = false;
    const nasceu = Date.now();

    const figura = h('span', { class: 'voz-figura' }, desenho('microfone'));
    const escrito = h('span', { class: 'voz-rotulo', text: rotulo });
    const btn = h('button', {
      type: 'button', class: 'btn btn-suave voz-botao',
      title: 'Clique e fale. O áudio não sai deste computador.',
      'aria-label': rotulo + ' no microfone',
    }, figura, escrito);
    const onda = h('span', { class: 'voz-onda', 'aria-hidden': 'true' }, h('i'), h('i'), h('i'), h('i'), h('i'));
    const tempo = h('span', { class: 'voz-tempo', role: 'timer', 'aria-label': 'Tempo de gravação', text: '0:00' });
    const enchimento = h('i');
    const barra = h('span', { class: 'voz-barra', role: 'progressbar', 'aria-label': 'Preparo da voz', 'aria-valuemin': '0', 'aria-valuemax': '100' }, enchimento);
    const porcento = h('span', { class: 'voz-porcento', 'aria-hidden': 'true' });
    const aviso = h('p', { class: 'voz-aviso', role: 'status', 'aria-live': 'polite' });
    const raiz = h('div', { class: 'voz', 'data-estado': 'pronto' }, btn, onda, tempo, barra, porcento, aviso);

    const TEXTOS = {
      preparando: 'Preparando a voz. Só acontece na primeira vez.',
      abrindo: 'Abrindo o microfone...',
      gravando: 'Ouvindo. Clique em Parar quando terminar.',
      entendendo: 'Entendendo o que você disse...',
    };

    function mostrar(novo, info) {
      info = info || {};
      const mudou = novo !== estado;
      estado = novo;
      raiz.dataset.estado = novo;
      const travado = novo === 'preparando' || novo === 'abrindo' || novo === 'entendendo';
      btn.disabled = travado;
      btn.classList.toggle('ocupado', travado);
      btn.classList.toggle('voz-gravando', novo === 'gravando');
      if (mudou) {
        btn.setAttribute('aria-label', novo === 'gravando' ? 'Parar a gravação' : rotulo + ' no microfone');
        escrito.textContent = novo === 'gravando' ? 'Parar' : rotulo;
        trocar(figura, desenho(novo === 'gravando' ? 'parar' : 'microfone'));
        aviso.classList.toggle('voz-erro', novo === 'erro');
        aviso.setAttribute('role', novo === 'erro' ? 'alert' : 'status');
      }
      if (novo === 'preparando') {
        // A porcentagem fica na barra, para o leitor de tela não repetir o aviso a cada passo.
        const p = Math.max(0, Math.min(100, Math.round(info.progresso || 0)));
        barra.setAttribute('aria-valuenow', String(p));
        enchimento.style.width = p + '%';
        porcento.textContent = p + '%';
      }
      const texto = info.mensagem || TEXTOS[novo] || '';
      if (aviso.textContent !== texto) aviso.textContent = texto;
      if (opc.aoEstado && (mudou || novo === 'preparando')) {
        try { opc.aoEstado(novo, info); } catch (e) { console.error(e); }
      }
    }

    function entregar(texto, final) {
      if (!opc.aoTexto) return;
      if (!final) entregou = true;
      try { opc.aoTexto(texto, final); } catch (e) { console.error(e); }
    }

    function pararRelogio() {
      clearInterval(tique);
      tique = null;
      cancelAnimationFrame(quadro);
      raiz.style.removeProperty('--voz-nivel');
    }

    // desistir encerra a gravação sem esperar o resto do texto. O que já saiu é entregue.
    function desistir(mensagem) {
      const atual = gravacao;
      gravacao = null;
      pararRelogio();
      if (atual) {
        atual.cancelar();
        if (entregou) entregar(atual.texto(), true);
      }
      if (mensagem) mostrar('erro', { mensagem });
      else if (estado !== 'pronto' && estado !== 'erro' && estado !== 'preparando') mostrar('pronto');
    }

    // O download acontece no programa. Aqui só aparece o andamento.
    function aoServidor(s) {
      if (raiz.isConnected) esteveNaTela = true;
      else if (esteveNaTela || Date.now() - nasceu > 60000) {
        if (estado !== 'gravando' && estado !== 'entendendo') { interessados.delete(aoServidor); return; }
      }
      if (estado === 'gravando' || estado === 'entendendo' || estado === 'abrindo') return;
      if (s.baixando) {
        viuDownload = true;
        mostrar('preparando', { progresso: s.progresso });
      } else if (estado === 'preparando') {
        if (s.pronto) mostrar('pronto', { mensagem: viuDownload ? 'A voz está pronta. Clique em ' + rotulo + '.' : '' });
        else mostrar('erro', { mensagem: s.erro || AVISOS.preparo });
        viuDownload = false;
      }
    }

    async function comecar() {
      if (!suportado()) { mostrar('erro', { mensagem: AVISOS.semSuporte }); return; }
      if (gravando) { mostrar('erro', { mensagem: AVISOS.outra }); return; }
      interessados.add(aoServidor);
      if (!servidor || !servidor.pronto) {
        const s = await consultar();
        if (!s.pronto) {
          // Ainda não baixou (ou falhou antes): pede ao programa e mostra o andamento.
          viuDownload = true;
          mostrar('preparando', { progresso: s.progresso });
          if (!s.baixando) await preparar();
          return;
        }
      }
      entregou = false;
      mostrar('abrindo'); // o botão trava enquanto o navegador pede a permissão
      try {
        gravacao = await gravar(
          (texto) => { if (estado === 'gravando' || estado === 'entendendo') entregar(texto, false); },
          (v) => { nivel = Math.max(v, nivel * 0.8); },
          (e) => { if (estado === 'gravando') desistir(avisoDe(e, AVISOS.entender)); },
        );
      } catch (e) {
        mostrar('erro', { mensagem: avisoDe(e, AVISOS.microfone) });
        return;
      }
      inicio = performance.now();
      tempo.textContent = '0:00';
      mostrar('gravando');
      let avisou = false;
      tique = setInterval(() => {
        // A tela mudou (o botão saiu ou a janela dele fechou): o microfone não fica aberto.
        if (!raiz.isConnected || raiz.getClientRects().length === 0) { desistir(); return; }
        const seg = (performance.now() - inicio) / 1000;
        tempo.textContent = relogio(Math.min(seg, limite));
        if (!avisou && limite > AVISO_ANTES + 5 && seg >= limite - AVISO_ANTES) {
          avisou = true;
          aviso.textContent = 'Faltam ' + AVISO_ANTES + ' segundos. Termine a frase e clique em Parar.';
        }
        if (seg >= limite) parar('A gravação parou no limite de ' + porExtenso(limite) + '. Para continuar, clique em ' + rotulo + ' de novo.');
      }, 250);
      const pintar = () => {
        raiz.style.setProperty('--voz-nivel', nivel.toFixed(3));
        nivel *= 0.9;
        if (estado === 'gravando') quadro = requestAnimationFrame(pintar);
      };
      quadro = requestAnimationFrame(pintar);
    }

    async function parar(recado) {
      if (estado !== 'gravando' || !gravacao) return;
      const atual = gravacao;
      pararRelogio();
      mostrar('entendendo');
      try {
        const texto = await atual.parar();
        if (gravacao !== atual) return; // cancelada no meio
        gravacao = null;
        entregar(texto, true);
        mostrar('pronto', { mensagem: recado || '' });
      } catch (e) {
        if (gravacao !== atual) return;
        gravacao = null;
        if (entregou) entregar(atual.texto(), true); // fecha a gravação para quem já recebeu trechos
        mostrar('erro', { mensagem: avisoDe(e, AVISOS.entender) });
      }
    }

    btn.addEventListener('click', async () => {
      if (estado === 'gravando') { parar(); return; }
      if (chamando || (estado !== 'pronto' && estado !== 'erro')) return;
      chamando = true;
      try {
        await comecar();
      } catch (e) {
        console.error(e);
        desistir(AVISOS.microfone);
      } finally {
        chamando = false;
      }
    });

    raiz.voz = { parar: () => parar(), cancelar: () => desistir(), estado: () => estado };

    // Já na chegada, confere se a voz está pronta. Se o programa ainda estiver baixando, mostra.
    if (suportado()) {
      interessados.add(aoServidor);
      consultar();
    }
    return raiz;
  }

  // Ao sair da página, nada fica ligado.
  window.addEventListener('pagehide', () => { desligarMotor(); });

  return { botao, transcrever, suportado };
})();
