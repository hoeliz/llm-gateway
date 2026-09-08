import React from 'react';
import guide from './client-guides.json';

export default function ClientGuide() {
  return <details className="client-guide" id="connect-clients">
    <summary><span>Conectar clientes</span><span className="guide-summary-hint">Open WebUI · LibreChat · Continue · Cline · Cursor</span></summary>
    <div className="guide-content">
      <p>{guide.intro}</p>
      <div className="guide-values">{guide.connection.map(item => <div key={item.label}><strong>{item.label}</strong><code>{item.value}</code></div>)}</div>
      <p>Substitua os IDs de exemplo por modelos habilitados na sua conta. As chaves dos provedores continuam no servidor.</p>
      <details className="guide-section"><summary>Endereços locais, Docker e nuvem</summary><ul>{guide.network.map(line => <li key={line}>{line}</li>)}</ul></details>
      {guide.clients.map(client => <details className="guide-section" key={client.id}>
        <summary><span>{client.name}</span><span className="guide-status">{client.status}</span></summary>
        <ol>{client.steps.map(step => <li key={step}>{step}</li>)}</ol>
        {client.example && <pre tabIndex="0" aria-label={`Exemplo de configuração para ${client.name}`}><code>{client.example}</code></pre>}
        <p className="guide-caveat">{client.caveat}</p>
        <a href={client.source} target="_blank" rel="noreferrer">Documentação oficial ↗</a>
      </details>)}
      <details className="guide-section"><summary>Resolver problemas de conexão</summary><dl className="guide-troubleshooting">{guide.troubleshooting.map(item => <div key={item.problem}><dt>{item.problem}</dt><dd>{item.solution}</dd></div>)}</dl></details>
      <p className="field-help">Documentação consultada em {guide.checked}. Esta versão ainda não tem catálogo de modelos, ferramentas, embeddings ou Responses API.</p>
    </div>
  </details>;
}
