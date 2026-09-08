import { readFile, writeFile } from 'node:fs/promises';
const data = JSON.parse(await readFile(new URL('../src/client-guides.json', import.meta.url), 'utf8'));
let doc = `# Conectar clientes ao LLM Gateway\n\nDocumentação consultada em ${data.checked}.\n\n${data.intro}\n\n## Dados da conexão\n\n| Campo | Valor |\n| --- | --- |\n`;
for (const item of data.connection) doc += `| ${item.label} | ${item.value} |\n`;
doc += '\nAs chaves dos provedores ficam no servidor Go. Substitua os placeholders antes de conectar; os exemplos não contêm credenciais reais.\n\n## Endereços e rede\n\n';
for (const line of data.network) doc += `- ${line}\n`;
doc += '\n## Teste básico antes de conectar\n\nExecute no mesmo ambiente de rede do cliente. Ajuste a URL se estiver em Docker.\n\n```sh\ncurl -fsS http://127.0.0.1:8080/healthz\n\ncurl -N http://127.0.0.1:8080/v1/chat/completions \\\n  -H "Authorization: Bearer $GATEWAY_API_KEY" \\\n  -H "Content-Type: application/json" \\\n  -d \'{"model":"openai/SEU_MODELO","messages":[{"role":"user","content":"Olá"}],"stream":true}\'\n```\n\nO healthcheck verifica apenas o processo; o segundo comando valida a chamada e usa os créditos do provedor.\n';
for (const client of data.clients) {
 doc += `\n## ${client.name}\n\n**${client.status}.**\n\n`;
 client.steps.forEach((step,i) => { doc += `${i+1}. ${step}\n`; });
 if (client.example) doc += `\n\`\`\`${client.language}\n${client.example}\n\`\`\`\n`;
 doc += `\n${client.caveat}\n\n[Documentação oficial — ${client.name}](${client.source})\n`;
}
doc += '\n## Resolver problemas\n\n| Sintoma | Ação |\n| --- | --- |\n';
for (const item of data.troubleshooting) doc += `| ${item.problem} | ${item.solution} |\n`;
doc += '\n## Alcance da compatibilidade\n\nEste guia não muda o protocolo do gateway. `/v1/models`, `/v1/responses`, `/v1/messages`, embeddings, multimodalidade e ferramentas ainda não estão implementados. OpenAI-compatible descreve o formato de chat aceito, não compatibilidade integral com qualquer cliente. As instruções e a tela usam a mesma fonte: `web/src/client-guides.json`. Após editar, execute `npm --prefix web run docs:clients` e gere novamente o build da interface.\n';
await writeFile(new URL('../../docs/clients.md', import.meta.url), doc);
