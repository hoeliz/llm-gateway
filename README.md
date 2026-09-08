# LLM Gateway em Go

MVP de um gateway HTTP para OpenAI e Anthropic, usando somente a biblioteca padrão do Go. Recebe um subconjunto textual de Chat Completions e traduz a Anthropic Messages API para o mesmo formato de resposta.

## Executar

Requer Go 1.22 ou superior e uma chave de pelo menos um provedor.

```sh
cp .env.example .env
# Edite .env com suas chaves antes de continuar.
set -a
. ./.env
set +a
go run ./cmd/gateway
```

Para gerar um executável local, use `make build`. Depois de exportar as variáveis acima, execute `./bin/llm-gateway`. Binários não são versionados.

O processo não carrega `.env` automaticamente. A chave do gateway é independente das chaves dos provedores. O servidor escuta em `127.0.0.1:8080` por padrão. Abra **http://127.0.0.1:8080** para usar o playground React integrado.

```sh
curl http://127.0.0.1:8080/healthz

# Substitua MODELO_DA_SUA_CONTA pelo identificador habilitado no provedor.
curl http://127.0.0.1:8080/v1/chat/completions \
  -H "Authorization: Bearer $GATEWAY_API_KEY" \
  -H 'Content-Type: application/json' \
  -d '{"model":"openai/MODELO_DA_SUA_CONTA","messages":[{"role":"user","content":"Explique o que é um gateway em uma frase."}]}'

curl -N http://127.0.0.1:8080/v1/chat/completions \
  -H "Authorization: Bearer $GATEWAY_API_KEY" \
  -H 'Content-Type: application/json' \
  -d '{"model":"anthropic/MODELO_DA_SUA_CONTA","messages":[{"role":"user","content":"Olá!"}],"max_tokens":256,"stream":true,"stream_options":{"include_usage":true}}'
```

Modelos não são fixados no código: o prefixo escolhe o provedor e o restante é enviado como seu identificador de modelo. As respostas retornam o modelo sem prefixo. As chamadas reais usam os créditos da conta do respectivo provedor.

## Interface gráfica

O playground permite selecionar OpenAI ou Anthropic, informar o ID do modelo, configurar instruções, temperatura e limite de tokens, conversar com streaming, interromper a geração e iniciar nova conversa. Mostra tempo total, tempo até o primeiro texto e tokens informados pelo provedor.

Informe no painel a mesma `GATEWAY_API_KEY` configurada no servidor. Chaves OpenAI/Anthropic continuam exclusivamente nas variáveis de ambiente do backend. A chave do gateway e a conversa ficam apenas na memória da página; não são gravadas em localStorage, sessionStorage ou cookies. Recarregar a página limpa esses dados. Conversas são renderizadas como texto, sem interpretar HTML gerado pelo modelo.

Respostas incompletas ficam visíveis, mas o turno incompleto não é reenviado no histórico; o prompt volta ao campo para uma nova tentativa. Ao trocar de provedor, o histórico concluído permanece e será enviado ao provedor selecionado na próxima mensagem. Use **Nova conversa** para começar sem histórico.

A interface e seus assets são públicos; a API permanece autenticada. O painel não configura as chaves dos provedores nem autentica usuários individuais.

### Desenvolver a interface

O código React está em `web/`. O build é incorporado ao executável via `go:embed`, sem CDN e sem precisar de Node em produção. O build gerado em `internal/webui/dist/` é versionado para preservar `go run` após clonar. Após alterar React/CSS, gere e versione novamente os assets.

```sh
# Node.js 22.16+ e npm; a versão exata das dependências está no lockfile.
cd web
npm ci
npm run build
npm test
cd ..
go run ./cmd/gateway
```

Para atualização instantânea durante desenvolvimento, mantenha o Go na porta 8080 e execute `make ui-dev` em outro terminal. O Vite encaminha `/v1` e `/healthz` ao servidor Go.

## Contrato do MVP

- `GET /` e `GET /assets/*`: interface React e arquivos estáticos, sem autenticação.
- `GET /healthz`: saúde do processo, sem autenticação; não testa credenciais/conectividade.
- `POST /v1/chat/completions`: exige `Authorization: Bearer <GATEWAY_API_KEY>`.
- `messages`: conteúdo string, papéis `system`, `developer`, `user`, `assistant`.
- `model`: obrigatório, `openai/<id>` ou `anthropic/<id>`.
- `max_tokens` ou `max_completion_tokens`: inteiro positivo; não envie ambos. Na Anthropic ambos são convertidos em `max_tokens` e o padrão é 1024. Na OpenAI, a opção é repassada; o suporte depende do modelo.
- `temperature`: opcional, 0–2 para OpenAI e 0–1 para Anthropic; o modelo também pode restringir o parâmetro.
- `stream` e `stream_options.include_usage`: streaming SSE com chunks de Chat Completions e `[DONE]` no término bem-sucedido.
- Na Anthropic, instruções `system`/`developer` devem preceder a conversa e são reunidas no campo `system`. A contagem de entrada inclui tokens de cache.
- Campos desconhecidos, ferramentas, imagens e demais conteúdos multimodais são rejeitados. Não é compatibilidade integral com a API OpenAI.

## Operação

Limite de requisição: 1 MiB. Resposta JSON: 8 MiB. Evento SSE: 1 MiB. `UPSTREAM_TIMEOUT` limita toda a chamada, incluindo streaming. O cancelamento HTTP é propagado ao provedor. Não há retries ou fallback automáticos.

Erros anteriores ao streaming usam JSON com `error.type` e `error.message`. Falhas no stream geram um evento SSE de erro e fecham a conexão sem `[DONE]`. Corpos de erro dos provedores não são expostos. HTTP 401/403 do provedor viram 502; 429 preserva `Retry-After`; timeout vira 504. Uma chave não configurada retorna 503.

Logs de inicialização/encerramento usam JSON e não registram prompts nem chaves. O MVP não inclui métricas por requisição, rate limiting, quotas, cobrança, persistência, gestão administrativa de usuários, catálogo `/v1/models`, tools ou Responses API. Para uso fora da máquina local, configure terminação TLS e controles de acesso/cotas na implantação.

## Estrutura e validação

- `cmd/gateway/main.go`: configuração, servidor e encerramento por sinal.
- `internal/gateway/gateway.go`: autenticação, validação e comunicação HTTP.
- `internal/gateway/anthropic.go`: tradução de mensagens, respostas e eventos SSE.
- `internal/gateway/gateway_test.go`: testes com servidores HTTP locais, sem chamadas pagas.
- `internal/webui/`: build React incorporado e entrega segura de arquivos estáticos.
- `web/src/`: playground, estilos e parser SSE com testes em Node.

```sh
go test -race ./...
go vet ./...
go build -o bin/llm-gateway ./cmd/gateway
```

Os testes cobrem roteamento, separação de credenciais, conversão de mensagens e uso de tokens, autenticação, validação, timeout, cancelamento, erros de upstream, SSE e streams incompletos. Chamadas reais dependem das suas credenciais e de modelos habilitados.

## Referências

- [OpenAI Chat Completions](https://developers.openai.com/api/reference/resources/chat/subresources/completions/methods/create)
- [Anthropic Messages](https://platform.claude.com/docs/en/api/messages/create)
- [Anthropic streaming](https://platform.claude.com/docs/en/build-with-claude/streaming)

Validação inicial do backend: 10 testes principais passaram com `-race`, além de `go vet` e compilação. O executável também foi iniciado localmente para verificar saúde HTTP, rejeição de acesso sem chave e encerramento por SIGTERM. As integrações com provedores reais não foram executadas.

Validação do playground: build de produção React, 8 testes Node de parsing SSE/validação, suíte Go com `-race`, `go vet` e teste HTTP do executável com provedor simulado (HTML, assets, CSP, autenticação e streaming). Não foram feitas chamadas pagas nem validação visual automatizada em navegador.
