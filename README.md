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

O processo não carrega `.env` automaticamente. A chave do gateway é independente das chaves dos provedores. O servidor escuta em `127.0.0.1:8080` por padrão.

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

## Contrato do MVP

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

Logs de inicialização/encerramento usam JSON e não registram prompts nem chaves. O MVP não inclui métricas por requisição, rate limiting, quotas, cobrança, persistência, painel administrativo, catálogo `/v1/models`, tools ou Responses API. Para uso fora da máquina local, configure terminação TLS e controles de acesso/cotas na implantação.

## Estrutura e validação

- `cmd/gateway/main.go`: configuração, servidor e encerramento por sinal.
- `internal/gateway/gateway.go`: autenticação, validação e comunicação HTTP.
- `internal/gateway/anthropic.go`: tradução de mensagens, respostas e eventos SSE.
- `internal/gateway/gateway_test.go`: testes com servidores HTTP locais, sem chamadas pagas.

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

Validação desta entrega: 10 testes principais passaram com `-race`, além de `go vet` e compilação. O executável também foi iniciado localmente para verificar saúde HTTP, rejeição de acesso sem chave e encerramento por SIGTERM. As integrações com provedores reais não foram executadas.
