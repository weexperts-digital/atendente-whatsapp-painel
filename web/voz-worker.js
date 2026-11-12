// Reconhecimento de voz local: a parte que roda fora da tela. Ver voz.js e voz.go.
//
// Este arquivo é carregado de dois jeitos, nunca como script da página:
//   1. como Web Worker (new Worker): é onde o Whisper transcreve, para a tela não travar;
//   2. como módulo de áudio (audioWorklet.addModule): só recolhe o som do microfone.
// Nos dois casos, tudo vem do próprio painel (/voz/...). Nada é buscado de fora e o
// áudio não sai deste computador.

if (typeof AudioWorkletProcessor !== 'undefined' && typeof registerProcessor === 'function') {
  recolherMicrofone();
} else {
  transcreverNoWorker();
}

// ---------- 1. Módulo de áudio: junta o som do microfone em blocos e entrega à página ----------

function recolherMicrofone() {
  const BLOCO = 2048; // cerca de 43 ms a 48 kHz

  class VozCaptura extends AudioWorkletProcessor {
    constructor() {
      super();
      this.bloco = new Float32Array(BLOCO);
      this.cheio = 0;
    }

    process(entradas) {
      const canal = entradas[0] && entradas[0][0];
      if (!canal) return true;
      let i = 0;
      while (i < canal.length) {
        const cabe = Math.min(canal.length - i, BLOCO - this.cheio);
        this.bloco.set(canal.subarray(i, i + cabe), this.cheio);
        this.cheio += cabe;
        i += cabe;
        if (this.cheio === BLOCO) {
          this.port.postMessage(this.bloco, [this.bloco.buffer]);
          this.bloco = new Float32Array(BLOCO);
          this.cheio = 0;
        }
      }
      return true;
    }
  }

  registerProcessor('voz-captura', VozCaptura);
}

// ---------- 2. Web Worker: o Whisper ----------

function transcreverNoWorker() {
  const TAXA = 16000;
  // Só aceita endereços do próprio painel, dentro de /voz/.
  const CAMINHO = /^\/voz\/[A-Za-z0-9._@/-]+$/;
  const NOME = /^[A-Za-z0-9._-]+$/;

  let motor = null; // promessa do Whisper carregado
  let vez = Promise.resolve(); // um áudio por vez, na ordem em que chegaram

  function local(caminho) {
    if (typeof caminho !== 'string' || !CAMINHO.test(caminho) || caminho.includes('..') || caminho.includes('//')) {
      throw new Error('Endereço recusado: ' + caminho);
    }
    return caminho;
  }

  async function carregar(arquivos) {
    if (!arquivos || !NOME.test(String(arquivos.modelo || ''))) throw new Error('Modelo recusado.');
    const lib = await import(local(arquivos.biblioteca));
    const env = lib.env;
    // Tudo local: os modelos e o motor vêm do painel, e a biblioteca fica proibida de ir à internet.
    env.allowRemoteModels = false;
    env.allowLocalModels = true;
    env.localModelPath = local(arquivos.modelos);
    // O painel já serve os arquivos deste computador: guardar outra cópia no navegador só gasta disco.
    env.useBrowserCache = false;
    env.useWasmCache = false;
    env.backends.onnx.wasm.wasmPaths = { mjs: local(arquivos.wasm_mjs), wasm: local(arquivos.wasm) };
    return lib.pipeline('automatic-speech-recognition', arquivos.modelo, { dtype: 'q8', device: 'wasm' });
  }

  async function transcrever(audio) {
    const whisper = await motor;
    const segundos = audio.length / TAXA;
    const saida = await whisper(audio, {
      language: 'portuguese',
      task: 'transcribe',
      // Teto de palavras pelo tempo de fala: se o modelo entrar em repetição, para logo.
      max_new_tokens: Math.min(440, 32 + Math.ceil(segundos * 14)),
    });
    return String((saida && saida.text) || '');
  }

  const motivo = (e) => String((e && e.message) || e || 'falha');

  self.onmessage = (ev) => {
    const m = ev.data || {};
    if (m.tipo === 'carregar') {
      if (!motor) {
        motor = carregar(m.arquivos);
        motor.then(
          () => self.postMessage({ tipo: 'carregado', nucleos: self.crossOriginIsolated ? 'varios' : 'um' }),
          (e) => self.postMessage({ tipo: 'falha', erro: motivo(e) }),
        );
      }
      return;
    }
    if (m.tipo === 'transcrever') {
      vez = vez.then(async () => {
        const inicio = performance.now();
        try {
          if (!motor) throw new Error('O Whisper ainda não foi carregado.');
          const texto = await transcrever(m.audio);
          self.postMessage({ tipo: 'texto', id: m.id, texto, ms: Math.round(performance.now() - inicio) });
        } catch (e) {
          self.postMessage({ tipo: 'falha', id: m.id, erro: motivo(e) });
        }
      });
    }
  };
}
