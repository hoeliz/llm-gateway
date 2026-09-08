import test from 'node:test';
import assert from 'node:assert/strict';
import { makePayload, readChatStream } from './stream.js';

function stream(text, step = 1) {
  const bytes = new TextEncoder().encode(text);
  let offset = 0;
  return new ReadableStream({ pull(controller) {
    if (offset >= bytes.length) { controller.close(); return; }
    controller.enqueue(bytes.slice(offset, offset + step)); offset += step;
  }});
}
test('handles split UTF-8, CRLF, comments and usage events', async () => {
  const chunks = [];
  await readChatStream(stream(': ping\r\nevent: message\r\ndata: {"choices":[{"delta":{"content":"Olá 👋"}}]}\r\n\r\ndata: {"choices":[],"usage":{"total_tokens":9}}\n\ndata: [DONE]\n\n'), c => chunks.push(c));
  assert.equal(chunks[0].choices[0].delta.content, 'Olá 👋');
  assert.equal(chunks[1].usage.total_tokens, 9);
});
test('delivers first text before source completes', async () => {
  let source;
  const body = new ReadableStream({ start(c) { source = c; c.enqueue(new TextEncoder().encode('data: {"choices":[{"delta":{"content":"first"}}]}\n\n')); }});
  let first;
  const arrived = new Promise(resolve => { first = resolve; });
  const reading = readChatStream(body, first);
  const chunk = await arrived;
  assert.equal(chunk.choices[0].delta.content, 'first');
  source.enqueue(new TextEncoder().encode('data: [DONE]\n\n')); source.close();
  await reading;
});
test('rejects truncated and malformed streams', async () => {
  await assert.rejects(readChatStream(stream('data: {"choices":[]}\n\n'), () => {}), /antes de concluir/);
  await assert.rejects(readChatStream(stream('data: invalid\n\n'), () => {}), /evento inválido/);
});
test('propagates errors in SSE even with HTTP 200', async () => {
  await assert.rejects(readChatStream(stream('data: {"error":{"message":"provider failed"}}\n\n'), () => {}), /provider failed/);
});
test('accepts multiline event data and DONE without final newline', async () => {
  const chunks = [];
  await readChatStream(stream('data: {\ndata: "choices": []}\n\ndata: [DONE]', 8), c => chunks.push(c));
  assert.deepEqual(chunks, [{ choices: [] }]);
});
test('propagates transport cancellation', async () => {
  const body = new ReadableStream({ start(c) { c.error(new DOMException('Aborted', 'AbortError')); }});
  await assert.rejects(readChatStream(body, () => {}), { name: 'AbortError' });
});
const base = { provider: 'openai', model: ' test-model ', system: 'Be concise', temperature: '', maxTokens: '1024', messages: [{ role: 'user', content: 'hello' }] };
test('sends provider-appropriate limits and omits optional temperature', () => {
  const oa = makePayload(base);
  assert.equal(oa.model, 'openai/test-model');
  assert.equal(oa.max_completion_tokens, 1024);
  assert.equal('temperature' in oa, false);
  assert.deepEqual(oa.messages[0], { role: 'system', content: 'Be concise' });
  const anth = makePayload({ ...base, provider: 'anthropic', temperature: '0.5' });
  assert.equal(anth.max_tokens, 1024);
  assert.equal(anth.temperature, 0.5);
  assert.equal('max_completion_tokens' in anth, false);
});
test('rejects invalid model and generation settings', () => {
  for (const overrides of [{ model: '' }, { model: 'openai/foo' }, { maxTokens: '0' }, { maxTokens: '1.5' }, { temperature: 'NaN' }, { provider: 'anthropic', temperature: '2' }]) assert.throws(() => makePayload({ ...base, ...overrides }));
});
