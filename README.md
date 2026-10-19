# finance-api

API JSON de um app pessoal de finanças: registra entradas, gastos (inclusive parcelados), investimentos, metas e dívidas, e calcula o painel com a evolução do patrimônio, dos gastos, das dívidas e das metas. Um binário Go único, com os dados em um arquivo SQLite local.

A interface fica em um repositório separado: [finance-web](https://github.com/Leonardoongaratto/finance-web) (React + Vite).

![Painel com patrimônio, gastos, dívidas e metas](docs/screenshots/dashboard.png)

| Tema claro | Dívidas no celular |
| :--- | :--- |
| ![Painel no tema claro](docs/screenshots/dashboard-light.png) | ![Tela de dívidas em 360 px](docs/screenshots/debts-mobile.png) |

- Go 1.27, `chi`, SQLite em Go puro (`modernc.org/sqlite`, **sem CGO**: não precisa de gcc).
- Só localhost por padrão: escuta em `127.0.0.1`, não tem login, recusa `Host` de fora (403) contra DNS rebinding.
- Dinheiro é sempre inteiro em centavos (`int64`, BRL). Nunca `float`.

## Arquitetura

```
cmd/finance-api     composição, config por env, shutdown gracioso
internal/domain     regras puras: parcelamento, saldo e projeção de dívida, metas, rendimento
internal/store      SQLite (migrations embutidas, repositórios, seed de categorias)
internal/report     painel: KPIs e séries mensais derivadas dos lançamentos
internal/api        roteador chi, handlers JSON, validação, erros padronizados, CORS
```

O navegador carrega o `finance-web` (outra origem) e chama esta API direto pela URL; por isso a API libera CORS só para as origens configuradas.

## Rodar

Sem Docker:

```powershell
go run ./cmd/finance-api          # http://127.0.0.1:8080/health
# ou
go build -o finance-api.exe ./cmd/finance-api
.\finance-api.exe
```

Com Docker Compose (clone `finance-api` e `finance-web` lado a lado):

```
finance-api/
finance-web/
```

```powershell
cd finance-api
docker compose up --build         # API em 127.0.0.1:8080, interface em http://127.0.0.1:5173
```

As duas portas são publicadas **só em `127.0.0.1`**. Dentro do contêiner a API precisa ouvir em `0.0.0.0`, o que só é permitido com `FINANCE_ALLOW_ANY_ADDR=true` (o compose liga). O `Host` continua precisando ser loopback, e o banco fica no volume `finance-data`. Para usar a interface sem Docker, siga o README do [finance-web](https://github.com/Leonardoongaratto/finance-web).

Variáveis de ambiente:

| Variável | Padrão | Função |
| :--- | :--- | :--- |
| `FINANCE_ADDR` | `127.0.0.1:8080` | Endereço de escuta. Só loopback, salvo com o opt-in abaixo |
| `FINANCE_ALLOW_ANY_ADDR` | (vazio) | `true` permite ouvir fora do loopback (uso no contêiner). Qualquer outro valor mantém a trava |
| `FINANCE_DB` | `finance.db` (pasta de trabalho) | Arquivo SQLite. Contém dados pessoais: já está no `.gitignore` |
| `FINANCE_CORS_ORIGINS` | `http://127.0.0.1:5173,http://localhost:5173,http://127.0.0.1:4173,http://localhost:4173` | Origens do front liberadas (separadas por vírgula, no formato `esquema://host[:porta]`). Vazio usa o padrão; `*` ou valor inválido faz a API falhar ao iniciar |

CORS: origem na lista recebe `Access-Control-Allow-Origin` igual à origem (nunca `*`), com `GET, POST, PUT, DELETE` e `Content-Type`, e o preflight `OPTIONS` responde 204. Origem fora da lista não recebe cabeçalhos CORS.

## Testes e gates

| Gate | Comando | Equivalente multiplataforma (CI) |
| :--- | :--- | :--- |
| Formatação | `powershell -NoProfile -ExecutionPolicy Bypass -File scripts\format.ps1` | `gofmt -l cmd internal` |
| Lint | `powershell -NoProfile -ExecutionPolicy Bypass -File scripts\lint.ps1` | `go vet ./...` e `staticcheck ./...` |
| Testes + cobertura (≥ 80%) | `powershell -NoProfile -ExecutionPolicy Bypass -File scripts\coverage.ps1` | `go test ./... -coverprofile=coverage.out` |
| Build | `go build -o finance-api.exe ./cmd/finance-api` | `go build ./cmd/finance-api` |

Se o Windows (Smart App Control) bloquear um binário de teste recém-compilado ("An Application Control policy has blocked this file"), `scripts\coverage.ps1` refaz o pacote com outro build id; nos demais comandos basta repetir.

O CI (`.github/workflows/ci.yml`) roda os gates em Linux e um job `compose` que sobe a pilha com Docker Compose (clonando o `finance-web` ao lado) e faz curl em `/health`, na página e no CORS.

## Decisões técnicas

- **Centavos em `int64`**: sem erro de ponto flutuante; a soma das parcelas é exata (o resto vai para a 1ª parcela).
- **Juros em 128 bits com teto**: o saldo da dívida é calculado sem estouro de `int64` e satura em R$ 10 trilhões (`domain.MaxBalanceCents`); sem pagamento ele nunca diminui.
- **Histórico derivado, sem snapshots**: as séries do painel saem dos eventos (transações, aportes, avaliações, pagamentos) no fim de cada mês. Corrigir um lançamento antigo corrige o histórico inteiro.
- **Segurança local**: loopback por padrão, guarda de `Host`, CORS com lista explícita, corpo limitado a 1 MB, `Content-Type: application/json` obrigatório em escritas.
- **Privacidade nos logs**: só método, rota, status e duração; falha de banco registra apenas a categoria do erro. Erros 500 não devolvem detalhes.

Regras de domínio que vale conhecer:

- Parcelamento (2 a 48): as parcelas compartilham um `installment_group`.
- Investimento: valor atual = última avaliação até a data + aportes feitos depois dela; sem avaliação, é o total aportado.
- Meta: ritmo ideal linear do `start_date` até o prazo; `on_track`, `behind`, `overdue` ou `done`. Mensal necessário = `ceil(falta / meses restantes)`.
- Dívida: saldo do mês = saldo anterior + juros (pontos-base ao mês, arredondado) − pagamentos do mês, nunca negativo; o mês de início não rende juros. Projeção até 600 meses; parcela ≤ juros do mês marca `never_pays_off`. Dívida com início futuro vem com `not_started: true`.
- O painel cobre só meses já vividos: um `to` no mês seguinte ao atual do servidor é trazido para o mês atual; mais adiante é rejeitado (400). Datas e meses aceitam anos de 1900 a 2199.

## API

Valores em centavos, datas `YYYY-MM-DD`, meses `YYYY-MM`. Erros: `{ "error": { "code", "message" } }` com `validation_error` (400), `forbidden` (403), `not_found` (404, inclusive para qualquer caminho fora de `/health` e `/api/*`), `method_not_allowed` (405), `conflict` (409), `payload_too_large` (413), `unsupported_media_type` (415) e `internal` (500). A API não serve páginas: `GET /` devolve 404 JSON.

- `GET /health`
- `GET|POST /api/accounts`, `PUT|DELETE /api/accounts/{id}`
- `GET|POST /api/categories`, `PUT|DELETE /api/categories/{id}`
- `GET /api/transactions?from=&to=&account_id=&category_id=&type=`, `POST /api/transactions`, `PUT|DELETE /api/transactions/{id}`
- `POST /api/transactions/installments`, `DELETE /api/installments/{group}`
- `GET|POST /api/investments`, `PUT|DELETE /api/investments/{id}`, `POST /api/investments/{id}/contributions|valuations`, `DELETE /api/investments/{id}/contributions|valuations/{childID}`
- `GET|POST /api/goals`, `PUT|DELETE /api/goals/{id}`, `POST /api/goals/{id}/contributions`, `DELETE /api/goals/{id}/contributions/{childID}`
- `GET|POST /api/debts`, `PUT|DELETE /api/debts/{id}`, `POST /api/debts/{id}/payments`, `DELETE /api/debts/{id}/payments/{childID}`
- `GET /api/dashboard?from=YYYY-MM&to=YYYY-MM` (`to` padrão e máximo: o mês atual; período de até 600 meses)

Exemplo:

```powershell
curl.exe -i http://127.0.0.1:8080/health
curl.exe -i -H "Origin: http://127.0.0.1:5173" http://127.0.0.1:8080/api/dashboard
```
