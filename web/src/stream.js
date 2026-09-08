// Parse SSE incrementally, including UTF-8 split across network chunks.
export async function readChatStream(body, onChunk) {
  if (!body) throw new Error('O servidor não retornou um fluxo de resposta.');
  const reader = body.getReader();
  const decoder = new TextDecoder();
  let buffer = '';
  let lines = [];
  let complete = false;
  function dispatch() {
    if (!lines.length) return;
    const data = lines.join('\n');
    lines = [];
    if (data === '[DONE]') { complete = true; return; }
    let chunk;
    try { chunk = JSON.parse(data); } catch { throw new Error('O gateway retornou um evento inválido.'); }
    if (chunk.error) throw new Error(chunk.error.message || 'O provedor interrompeu a resposta.');
    onChunk(chunk);
  }
  function line(value) {
    if (value === '') dispatch();
    else if (value.startsWith('data:')) lines.push(value.slice(5).replace(/^ /, ''));
  }
  function process(final = false) {
    let index;
    while (!complete && (index = buffer.indexOf('\n')) >= 0) {
      line(buffer.slice(0, index).replace(/\r$/, ''));
      buffer = buffer.slice(index + 1);
    }
    if (final && !complete) { if (buffer) line(buffer.replace(/\r$/, '')); dispatch(); }
  }
  try {
    while (!complete) {
      const { value, done } = await reader.read();
      buffer += decoder.decode(value, { stream: !done });
      process(done);
      if (buffer.length + lines.reduce((n, l) => n + l.length, 0) > 1024 * 1024) throw new Error('Evento de resposta muito grande.');
      if (done) break;
    }
    if (!complete) throw new Error('A conexão terminou antes de concluir a resposta.');
  } finally {
    await reader.cancel().catch(() => {});
    reader.releaseLock();
  }
}

export function makePayload({ provider, model, system, temperature, maxTokens, messages }) {
  if (!['openai', 'anthropic'].includes(provider)) throw new Error('Selecione um provedor válido.');
  if (!model.trim() || model.includes('/')) throw new Error('Informe o identificador do modelo sem o prefixo do provedor.');
  const payload = {
    model: `${provider}/${model.trim()}`,
    messages: [...(system.trim() ? [{ role: 'system', content: system.trim() }] : []), ...messages],
    stream: true,
    stream_options: { include_usage: true },
  };
  if (maxTokens !== '') {
    const tokens = Number(maxTokens);
    if (!Number.isSafeInteger(tokens) || tokens < 1) throw new Error('O limite de tokens deve ser um inteiro positivo.');
    payload[provider === 'openai' ? 'max_completion_tokens' : 'max_tokens'] = tokens;
  }
  if (temperature !== '') {
    const temp = Number(temperature);
    if (!Number.isFinite(temp) || temp < 0 || temp > (provider === 'anthropic' ? 1 : 2)) throw new Error('Temperatura fora do intervalo do provedor.');
    payload.temperature = temp;
  }
  return payload;
}
