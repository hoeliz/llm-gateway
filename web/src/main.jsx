import React, { useEffect, useRef, useState } from 'react';
import { createRoot } from 'react-dom/client';
import { makePayload, readChatStream } from './stream.js';
import './style.css';

function Icon({ name, ...props }) {
  const paths = {
    arrow: <><path d="M5 12h14M13 6l6 6-6 6" /></>,
    plus: <path d="M12 5v14M5 12h14" />,
    stop: <rect x="6" y="6" width="12" height="12" rx="2" />,
    bolt: <path d="m13 2-9 12h7l-1 8 10-13h-7z" />,
    copy: <><rect x="8" y="8" width="12" height="12" rx="2" /><path d="M16 8V4H4v12h4" /></>,
    settings: <><path d="M4 6h16M4 12h16M4 18h16" /><path d="M8 3v6M16 9v6M10 15v6" /></>,
    chat: <path d="M20 15a3 3 0 0 1-3 3H8l-5 3V6a3 3 0 0 1 3-3h11a3 3 0 0 1 3 3z" />,
  };
  return <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true" {...props}>{paths[name]}</svg>;
}
const initialStats = { elapsed: null, first: null, usage: null };
function App() {
  const [provider, setProvider] = useState('openai');
  const [configOpen, setConfigOpen] = useState(false);
  const [model, setModel] = useState('');
  const [key, setKey] = useState('');
  const [system, setSystem] = useState('');
  const [temperature, setTemperature] = useState('');
  const [maxTokens, setMaxTokens] = useState('1024');
  const [draft, setDraft] = useState('');
  const [messages, setMessages] = useState([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [health, setHealth] = useState('checking');
  const [stats, setStats] = useState(initialStats);
  const [copied, setCopied] = useState(null);
  const [notice, setNotice] = useState('');
  const controller = useRef(null);
  const bottom = useRef(null);
  const composer = useRef(null);
  const messageID = useRef(0);
  useEffect(() => {
    const abort = new AbortController();
    fetch('/healthz', { signal: abort.signal }).then(r => { if (!r.ok) throw new Error(); return r.json(); }).then(d => setHealth(d.status === 'ok' ? 'online' : 'offline')).catch(() => setHealth('offline'));
    return () => { abort.abort(); controller.current?.abort(); };
  }, []);
  useEffect(() => { bottom.current?.scrollIntoView({ block: 'nearest' }); }, [messages]);
  function newConversation() {
    if (controller.current) return;
    setMessages([]); setError(''); setStats(initialStats); setNotice(''); setDraft(''); composer.current?.focus();
  }
  async function copy(text, id) {
    try { await navigator.clipboard.writeText(text); setCopied(id); setTimeout(() => setCopied(null), 1800); }
    catch { setNotice('Não foi possível copiar. Selecione o texto e copie manualmente.'); }
  }
  async function send(event) {
    event.preventDefault();
    if (controller.current || !draft.trim()) return;
    setError(''); setNotice('');
    if (!key.trim()) { setConfigOpen(true); setError('Informe a chave do gateway nas configurações.'); return; }
    const previous = messages.filter(m => m.complete).map(({ role, content }) => ({ role, content }));
    const prompt = draft.trim();
    const history = [...previous, { role: 'user', content: prompt }];
    let payload;
    try { payload = makePayload({ provider, model, system, temperature, maxTokens, messages: history }); }
    catch (e) { setConfigOpen(true); setError(e.message); return; }
    const assistantID = ++messageID.current;
    setMessages([...messages, { id: ++messageID.current, role: 'user', content: prompt, complete: false }, { id: assistantID, role: 'assistant', content: '', complete: false }]);
    setDraft(''); setBusy(true); setStats(initialStats);
    const abort = new AbortController(); controller.current = abort;
    const start = performance.now(); let first = null; let usage = null; let reason = null;
    try {
      const response = await fetch('/v1/chat/completions', { method: 'POST', headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${key.trim()}` }, body: JSON.stringify(payload), signal: abort.signal });
      if (!response.ok) {
        const data = await response.json().catch(() => null);
        if (response.status === 401) throw new Error('Chave do gateway inválida. Confira a configuração e tente novamente.');
        throw new Error(data?.error?.message || `O gateway retornou HTTP ${response.status}.`);
      }
      if (!response.headers.get('content-type')?.includes('text/event-stream')) throw new Error('O gateway não retornou uma resposta em streaming.');
      await readChatStream(response.body, chunk => {
        if (chunk.usage) usage = chunk.usage;
        const choice = chunk.choices?.[0];
        if (choice?.finish_reason) reason = choice.finish_reason;
        const text = choice?.delta?.content;
        if (typeof text === 'string' && text) {
          if (first === null) first = performance.now() - start;
          setMessages(current => current.map(m => m.id === assistantID ? { ...m, content: m.content + text } : m));
        }
        setStats({ elapsed: performance.now() - start, first, usage });
      });
      if (!reason) throw new Error('O provedor não confirmou o término da resposta.');
      setMessages(current => current.map(m => m.id === assistantID || m.id === assistantID + 1 ? { ...m, complete: true, reason: m.id === assistantID ? reason : undefined } : m));
    } catch (e) {
      const label = abort.signal.aborted ? 'Resposta interrompida por você.' : e.message;
      if (abort.signal.aborted) setNotice(label); else setError(label);
      setMessages(current => current.map(m => m.id === assistantID ? { ...m, interrupted: true } : m));
      setDraft(prompt);
    } finally {
      setStats({ elapsed: performance.now() - start, first, usage });
      controller.current = null; setBusy(false); composer.current?.focus();
    }
  }
  const seconds = value => value === null ? '—' : `${(value / 1000).toFixed(2)} s`;
  return <div className="app">
    <aside className="rail">
      <a className="brand" href="/" aria-label="LLM Gateway, início"><span className="brand-mark"><Icon name="bolt" /></span><span>LLM<span className="brand-light">Gateway</span></span></a>
      <div className="rail-label">WORKSPACE</div>
      <div className="nav-item"><Icon name="chat" /> Playground <span className="nav-dot" /></div>
      <div className="rail-bottom"><span className={`status-dot ${health}`} /><span>{health === 'online' ? 'Gateway disponível' : health === 'checking' ? 'Conectando…' : 'Gateway indisponível'}</span></div>
    </aside>
    <main>
      <header className="page-header"><div><div className="eyebrow">DESENVOLVIMENTO / PLAYGROUND</div><h1>Teste uma ideia.</h1><p>Dois provedores. Um lugar para conversar.</p></div><button className="secondary" onClick={newConversation} disabled={busy || messages.length === 0}><Icon name="plus" /> Nova conversa</button></header>
      <div className="workspace">
        <section className="conversation" aria-label="Conversa">
          <div className="conversation-header"><span><span className="live-dot" /> Chat de teste</span><span className="badge">{provider === 'openai' ? 'OpenAI' : 'Anthropic'}</span></div>
          <div className="messages" aria-label="Mensagens">
            {!messages.length && <div className="empty"><div className="empty-mark"><Icon name="bolt" width="30" height="30" /></div><h2>O que vamos explorar?</h2><p>Configure seu modelo ao lado e envie a primeira mensagem.</p><div className="suggestions">{['Explique um conceito complexo', 'Revise um trecho de código', 'Transforme uma ideia em um plano'].map(text => <button key={text} onClick={() => { setDraft(text); composer.current?.focus(); }}>{text}<Icon name="arrow" width="16" /></button>)}</div></div>}
            {messages.map(m => <article key={m.id} className={`message ${m.role}`}><div className="message-label"><span className="avatar">{m.role === 'user' ? 'V' : <Icon name="bolt" width="14" height="14" />}</span><strong>{m.role === 'user' ? 'Você' : 'Assistente'}</strong>{m.role === 'assistant' && m.content && <button className="copy" aria-label="Copiar resposta" onClick={() => copy(m.content, m.id)}><Icon name="copy" width="15" />{copied === m.id ? 'Copiado' : 'Copiar'}</button>}</div><div className="message-text">{m.content || (busy ? <span className="thinking">Aguardando resposta…</span> : 'Nenhum texto recebido.')}</div>{m.interrupted && <span className="message-note">Incompleta · não enviada no histórico</span>}{m.reason === 'length' && <span className="message-note">Limite de tokens atingido</span>}{m.reason === 'content_filter' && <span className="message-note">Resposta limitada pelo provedor</span>}</article>)}
            <div ref={bottom} />
          </div>
          <div className="composer-area">
            {error && <div className="alert" role="alert">{error}</div>}
            {notice && <div className="notice" role="status">{notice}</div>}
            <form onSubmit={send} className="composer"><label className="sr-only" htmlFor="prompt">Sua mensagem</label><textarea id="prompt" ref={composer} value={draft} onChange={e => setDraft(e.target.value)} placeholder="Escreva sua mensagem…" disabled={busy} rows="3" onKeyDown={e => { if (e.key === 'Enter' && !e.shiftKey && !e.nativeEvent.isComposing) { e.preventDefault(); if (!busy) e.currentTarget.form.requestSubmit(); } }} /><div className="composer-actions"><span>Enter para enviar <span className="hint-divider">·</span> Shift + Enter para nova linha</span>{busy ? <button type="button" className="primary stop" onClick={() => controller.current?.abort()}><Icon name="stop" width="16" /> Interromper</button> : <button className="primary" type="submit" disabled={!draft.trim()}>Enviar <Icon name="arrow" width="17" /></button>}</div></form>
            <div className="session-note"><span className="tiny-square" /> Conversa mantida apenas nesta página</div>
          </div>
        </section>
        <aside className={`config ${configOpen ? "expanded" : ""}`}><div className="config-title"><Icon name="settings" /><h2>Configurações</h2><button className="config-toggle" type="button" aria-expanded={configOpen} aria-controls="config-content" onClick={() => setConfigOpen(value => !value)}>{configOpen ? "Recolher" : "Configurar"}</button></div><div id="config-content" className="config-content">
          <fieldset disabled={busy}>
            <legend className="sr-only">Configuração da geração</legend>
            <label htmlFor="provider">Provedor</label><select id="provider" value={provider} onChange={e => { setProvider(e.target.value); setModel(''); setTemperature(''); }}>{['openai', 'anthropic'].map(p => <option value={p} key={p}>{p === 'openai' ? 'OpenAI' : 'Anthropic'}</option>)}</select>
            <label htmlFor="model">Modelo</label><input id="model" value={model} onChange={e => setModel(e.target.value)} placeholder="ID do modelo na sua conta" autoComplete="off" spellCheck="false" /><p className="field-help">Use o identificador habilitado no provedor, sem prefixo.</p>
            <label htmlFor="gateway-key">Chave do gateway <span className="required">*</span></label><input id="gateway-key" type="password" value={key} onChange={e => setKey(e.target.value)} placeholder="Sua GATEWAY_API_KEY" autoComplete="off" spellCheck="false" /><p className="field-help">Não é a chave da OpenAI ou Anthropic. Não será salva no navegador.</p>
            <div className="divider" />
            <label htmlFor="system">Instruções do sistema <span className="optional">Opcional</span></label><textarea id="system" rows="3" value={system} onChange={e => setSystem(e.target.value)} placeholder="Ex.: responda em português, de forma objetiva." />
            <div className="parameter-grid"><div><label htmlFor="temperature">Temperatura</label><input id="temperature" type="number" min="0" max={provider === 'anthropic' ? 1 : 2} step="0.1" value={temperature} onChange={e => setTemperature(e.target.value)} placeholder="Padrão" /></div><div><label htmlFor="tokens">Máx. tokens</label><input id="tokens" type="number" min="1" step="1" value={maxTokens} onChange={e => setMaxTokens(e.target.value)} placeholder="Padrão" /></div></div>
            <p className="field-help">Deixe a temperatura vazia para usar o padrão do modelo.</p>
          </fieldset>
          <div className="metrics"><div className="metric-heading">ÚLTIMA REQUISIÇÃO <span className={busy ? 'running' : ''}>{busy ? 'Gerando…' : stats.elapsed !== null ? 'Finalizada' : 'Sem dados'}</span></div><dl><div><dt>Tempo total</dt><dd>{seconds(stats.elapsed)}</dd></div><div><dt>Primeiro texto</dt><dd>{seconds(stats.first)}</dd></div><div><dt>Tokens de entrada</dt><dd>{stats.usage?.prompt_tokens?.toLocaleString('pt-BR') ?? '—'}</dd></div><div><dt>Tokens de saída</dt><dd>{stats.usage?.completion_tokens?.toLocaleString('pt-BR') ?? '—'}</dd></div></dl><p className="field-help">Tokens informados pelo provedor. “—” indica dado indisponível.</p></div>
        </div></aside>
      </div>
      <footer><span>LLM GATEWAY <span className="footer-version">/ PLAYGROUND</span></span><span>Texto & streaming</span></footer>
      <div className="sr-only" role="status" aria-live="polite">{busy ? 'Gerando resposta' : messages.length ? 'Requisição finalizada' : ''}</div>
    </main>
  </div>;
}
createRoot(document.getElementById('root')).render(<App />);
