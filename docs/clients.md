# Conectar clientes ao LLM Gateway

Documentação consultada em 2026-09-08.

Configurações baseadas na documentação oficial. Não foram testadas de ponta a ponta nestes aplicativos. O MVP aceita apenas chat de texto com conteúdo string e streaming.

## Dados da conexão

| Campo | Valor |
| --- | --- |
| Base URL · cliente na mesma máquina | http://127.0.0.1:8080/v1 |
| Base URL · cliente em Docker Desktop | http://host.docker.internal:8080/v1 |
| API Key | O valor de GATEWAY_API_KEY do servidor |
| Model ID | openai/SEU_MODELO ou anthropic/SEU_MODELO |

As chaves dos provedores ficam no servidor Go. Substitua os placeholders antes de conectar; os exemplos não contêm credenciais reais.

## Endereços e rede

- A porta 5173 é a prévia do frontend. A conexão dos clientes deve apontar para a porta 8080 do gateway, com /v1. Não acrescente /chat/completions ao campo Base URL.
- Dentro de containers, localhost aponta para o próprio container. No Docker Desktop, use host.docker.internal. Em Linux, pode ser necessário adicionar --add-host=host.docker.internal:host-gateway ao container.
- Para aceitar conexões de containers ou de outra máquina, configure LISTEN_ADDR=0.0.0.0:8080 antes de iniciar o gateway e restrinja o acesso pelo firewall/rede. O padrão 127.0.0.1 aceita apenas conexões locais.
- Se cliente e gateway estiverem na mesma rede Docker, use o nome do serviço do gateway: http://gateway:8080/v1, substituindo gateway pelo nome real.
- Clientes em nuvem não alcançam seu localhost. Use um gateway hospedado e acessível via HTTPS. O MVP não habilita CORS para conexões diretas entre origens no navegador; prefira clientes que façam as chamadas pelo servidor.

## Teste básico antes de conectar

Execute no mesmo ambiente de rede do cliente. Ajuste a URL se estiver em Docker.

```sh
curl -fsS http://127.0.0.1:8080/healthz

curl -N http://127.0.0.1:8080/v1/chat/completions \
  -H "Authorization: Bearer $GATEWAY_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"model":"openai/SEU_MODELO","messages":[{"role":"user","content":"Olá"}],"stream":true}'
```

O healthcheck verifica apenas o processo; o segundo comando valida a chamada e usa os créditos do provedor.

## Open WebUI

**Configuração para chat de texto.**

1. Abra Settings → Admin → Connections e adicione uma conexão OpenAI.
2. Preencha URL com o endereço do gateway terminado em /v1 e API Key com sua GATEWAY_API_KEY. Mantenha o tipo/provedor em Default/OpenAI-compatible.
3. Em Model IDs (Filter), adicione manualmente openai/SEU_MODELO ou anthropic/SEU_MODELO usando o botão + e salve. A descoberta automática não funciona neste MVP.
4. Selecione o modelo e teste uma mensagem curta de texto. Deixe ferramentas, anexos e recursos de agentes desativados.

```text
URL: http://127.0.0.1:8080/v1
API Key: SUA_GATEWAY_API_KEY
Model IDs (Filter):
  openai/SEU_MODELO_OPENAI
  anthropic/SEU_MODELO_ANTHROPIC
```

Se o teste de conexão reclamar de /models, confira os IDs manuais. O cliente ainda pode enviar parâmetros extras: um HTTP 400 indica que o payload precisa ser ajustado. Prefira conexão administrada pelo servidor, não Direct Connections do navegador.

[Documentação oficial — Open WebUI](https://docs.openwebui.com/getting-started/quick-start/connect-a-provider/starting-with-openai-compatible)

## LibreChat

**Configuração para chat de texto.**

1. Defina LLM_GATEWAY_API_KEY no ambiente do servidor LibreChat com a chave do gateway. Não coloque a chave real no YAML versionado.
2. Mescle o bloco endpoints.custom abaixo ao seu librechat.yaml existente. Preserve a versão e as demais configurações do arquivo.
3. Substitua os IDs de modelo e ajuste baseURL para a rede do LibreChat. Mantenha models.fetch: false.
4. Reinicie o LibreChat conforme sua instalação, selecione LLM Gateway e use conversa de texto sem agentes ou anexos.

```yaml
endpoints:
  custom:
    - name: "LLM Gateway"
      apiKey: "${LLM_GATEWAY_API_KEY}"
      baseURL: "http://127.0.0.1:8080/v1"
      models:
        default:
          - "openai/SEU_MODELO_OPENAI"
          - "anthropic/SEU_MODELO_ANTHROPIC"
        fetch: false
      titleConvo: false
      modelDisplayLabel: "LLM Gateway"
      dropParams:
        - user
        - stop
        - top_p
        - frequency_penalty
        - presence_penalty
        - seed
        - logit_bias
        - logprobs
        - top_logprobs
        - n
```

Não defina provider: anthropic: mesmo para modelos Anthropic, este gateway recebe Chat Completions. dropParams remove parâmetros que o MVP não aceita; outros campos que sua versão enviar também podem exigir adaptação. Não remova ferramentas para simular suporte a agentes.

[Documentação oficial — LibreChat](https://www.librechat.ai/docs/configuration/librechat_yaml/object_structure/custom_endpoint)

## Continue · VS Code / JetBrains

**Somente modo Chat; requer validação.**

1. Abra a configuração local YAML do Continue e mescle o modelo abaixo à lista models.
2. Use provider: openai para ambos os provedores e troque model pelo identificador completo com prefixo.
3. Informe sua chave do gateway apenas na configuração local privada; não a versione. Ajuste apiBase para o endereço visto pela extensão, inclusive em desenvolvimento remoto.
4. Use o modo Chat. Mantenha useResponsesApi: false e não ative useLegacyCompletionsEndpoint. Não use agentes, MCP, autocomplete ou edição automática neste MVP.

```yaml
name: LLM Gateway
version: 1.0.0
schema: v1
models:
  - name: Gateway Chat
    provider: openai
    model: openai/SEU_MODELO_OPENAI
    apiBase: http://127.0.0.1:8080/v1
    apiKey: SUA_GATEWAY_API_KEY
    useResponsesApi: false
```

A configuração segue a documentação do Continue, mas o chat pode enviar opções extras ou conteúdo estruturado que este gateway rejeita. O exemplo não garante compatibilidade integral com a extensão.

[Documentação oficial — Continue · VS Code / JetBrains](https://docs.continue.dev/customize/model-providers/top-level/openai)

## Cline

**Agentes ainda não suportados.**

1. O Cline permite selecionar API Provider → OpenAI Compatible nas configurações.
2. Os campos correspondentes seriam Base URL, API Key e Model ID, preenchidos com os valores abaixo.
3. Não trate esta configuração como uma integração funcional de agentes. O gateway precisa evoluir para suportar o protocolo de ferramentas e os payloads usados pelo cliente.

```text
API Provider: OpenAI Compatible
Base URL: http://127.0.0.1:8080/v1
API Key: SUA_GATEWAY_API_KEY
Model ID: openai/SEU_MODELO_OPENAI
```

Este MVP não implementa tool_calls, mensagens tool, imagens ou Responses API. O Cline não foi validado, e seu fluxo de agente não é suportado por este gateway.

[Documentação oficial — Cline](https://docs.cline.bot/provider-config/openai-compatible)

## Cursor

**Integração com o gateway não confirmada.**

1. A documentação de Bring your own API key orienta usar Cursor Settings → Models para cadastrar chaves de provedores. Essa configuração, por si só, não aponta para este gateway.
2. A página consultada não confirma um fluxo de URL personalizada para este gateway. Não cole GATEWAY_API_KEY em uma conexão destinada diretamente à OpenAI ou Anthropic.
3. As chamadas BYOK passam pelos servidores do Cursor; portanto, um endereço 127.0.0.1 da sua máquina não é suficiente. Uma futura integração exigiria um endpoint HTTPS alcançável e validação do protocolo da versão usada.

Não há configuração operacional validada para Cursor neste MVP. Ferramentas, Responses API e descoberta de modelos continuam fora do contrato do gateway.

[Documentação oficial — Cursor](https://cursor.com/docs/settings/api-keys)

## Resolver problemas

| Sintoma | Ação |
| --- | --- |
| 401 · chave inválida | Use GATEWAY_API_KEY, não OPENAI_API_KEY nem ANTHROPIC_API_KEY. Nos campos API Key, informe apenas o valor; o cliente adiciona Bearer. |
| 404/405 em /v1/models | O catálogo ainda não existe. Informe modelos manualmente e desative a descoberta quando o cliente permitir. |
| 400 · campo ou conteúdo não suportado | O decoder é estrito: tools, top_p, stop, user, response_format e conteúdo em arrays não são aceitos, mesmo vazios. Desative a função ou remova o parâmetro na configuração do cliente. Não é um problema de credenciais. |
| Modelo não encontrado | Troque SEU_MODELO por um ID habilitado na conta e preserve openai/ ou anthropic/. No playground integrado, selecione o provedor e use o ID sem prefixo; nos clientes externos, use o prefixo completo. |
| 503 · provedor indisponível | Configure a chave do provedor correspondente no ambiente do gateway e reinicie o processo. |
| 502 · provedor recusou / 504 · timeout | Confira as credenciais, o modelo e a conectividade do servidor. Para chamadas longas, revise UPSTREAM_TIMEOUT. Se a conexão cair após HTTP 200, o erro pode chegar em um evento SSE. |
| Connection refused / falha de rede | Verifique /healthz, a porta, o endereço usado pelo cliente e LISTEN_ADDR. O serviço deve estar rodando antes de conectar o aplicativo. |

## Alcance da compatibilidade

Este guia não muda o protocolo do gateway. `/v1/models`, `/v1/responses`, `/v1/messages`, embeddings, multimodalidade e ferramentas ainda não estão implementados. OpenAI-compatible descreve o formato de chat aceito, não compatibilidade integral com qualquer cliente. As instruções e a tela usam a mesma fonte: `web/src/client-guides.json`. Após editar, execute `npm --prefix web run docs:clients` e gere novamente o build da interface.
