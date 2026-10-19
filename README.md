# Personal Finance Telegram Bot

Bot de despesas para **uma pessoa**, em português brasileiro. Go, SQLite puro Go (`modernc.org/sqlite`), `database/sql` e `github.com/go-telegram/bot`. Usa long polling; não abre portas nem precisa de servidor web, contêiner ou serviço pago.

## Começar

Para compilar, requer Go **1.26 ou superior**; desenvolvimento e builds validados com Go 1.27.2. Instalação completa, descoberta do User ID, produção na Oracle Cloud e recuperação estão em [DEPLOY.md](DEPLOY.md).

Para uma VM **Oracle Linux com GitHub já autenticado**, siga o [roteiro a partir da etapa 8](ORACLE_LINUX.md): publicação dos fontes, `git clone`, compilação na VM e instalação com systemd. O [tutorial completo](DEPLOY.md) explica BotFather, descoberta do ID, alternativas com binários prontos e restauração de backups.

```sh
go mod download
cp .env.example .env
# Edite .env com seu token e Telegram User ID; proteja o arquivo.
chmod 600 .env
set -a
. ./.env
set +a
go run ./cmd/bot
```

O `.env` é um arquivo de configuração local: **o executável não o carrega automaticamente**. O shell acima exporta as variáveis; em produção o systemd lê `EnvironmentFile`. Para PowerShell, veja DEPLOY.md. Não execute configurações de origem desconhecida com o shell.

## Uso

| Comando | Função |
| --- | --- |
| `/start`, `/ajuda` | Orientação e comandos |
| `/gasto`, `/gasto 42,50` | Valor → categoria → pagamento → parcelas se crédito |
| `/mes`, `/mes 08/2026` | Total, categorias, à vista e crédito por competência |
| `/periodo 12/2026 02/2027` | Total inclusivo, categorias, cada mês e média mensal |
| `/categorias`, `/categorias 08/2026` | Categorias do mês |
| `/parcelas` | Parcelas dos meses posteriores ao mês atual; soma do futuro |
| `/desfazer` | Confirma por botão o cancelamento da última compra ativa inteira |
| `/cancelar` | Limpa o fluxo incompleto |
| `/id` | Exibe somente o ID do remetente em conversa privada |

O menu é registrado com `setMyCommands` ao iniciar. Todos os comandos, exceto `/id`, exigem o único ID autorizado e conversa privada. Nenhum comando financeiro é aceito em grupos. Usuários não autorizados são ignorados; callbacks recebem ACK neutro.

As categorias e seus IDs estáveis estão em `internal/finance/finance.go`. Pix, dinheiro e débito contam no mês da compra. O valor pode ser `42`, `42,50`, `42.50` ou `1.250,90`. Um separador decimal aceita uma ou duas casas. `1.250` e `1,250` são ambíguos e recusados. Faixa por compra: R$ 0,01 a R$ 1.000.000,00. Parcelas: 1–12, sempre com pelo menos um centavo por parcela.

## Crédito e competência

O valor original da compra **não é somado diretamente nos relatórios**. Eles somam exclusivamente a distribuição mensal persistida. R$ 100,00 em 3x resulta em R$ 33,34 + R$ 33,33 + R$ 33,33; os centavos restantes vão para as primeiras parcelas.

* **Simplificado**: com ambos os dias vazios, a primeira parcela entra no mês seguinte à compra, mesmo em 1x. `due_date` fica nulo. É uma previsão que pode diferir da fatura real.
* **Configurado**: com ambos os dias entre 1 e 28, calcula o fechamento estritamente posterior ao dia da compra e o primeiro vencimento estritamente posterior a esse fechamento. Uma compra **no dia do fechamento entra no próximo ciclo**, sem considerar a hora. Vencimento igual ao fechamento ocorre no mês seguinte.

Exemplo: fechamento 20, vencimento 10. Compra em 19/01 → primeira parcela em 10/02; compra em 20/01 → em 10/03. Fechamento 5, vencimento 20, compra em 04/01 → primeira parcela em 20/01. As outras parcelas avançam meses de calendário, sem somar 30 dias. Alterar a configuração não recalcula compras antigas.

## Configuração

| Variável | Regra / padrão |
| --- | --- |
| `TELEGRAM_BOT_TOKEN` | Obrigatória; formato validado e token conferido pelo Telegram ao iniciar |
| `ALLOWED_TELEGRAM_USER_ID` | Obrigatória; único inteiro positivo autorizado |
| `DATABASE_PATH` | `./data/finance.db`; use caminho absoluto em produção |
| `TIMEZONE` | `America/Sao_Paulo`; base de fusos embutida no binário |
| `STATE_TTL` | `15m`; entre `1m` e `1h`, renovado ao avançar uma etapa válida |
| `CREDIT_CLOSING_DAY`, `CREDIT_DUE_DAY` | Ambos vazios, ou ambos de 1 a 28 |

O usuário informa despesas do momento do registro: não há edição de datas, múltiplos cartões ou integração bancária. Meses consultáveis vão de 01/2000 a 12/9999; o período tem limite de 120 meses. A média considera inclusive meses sem despesas e arredonda a exibição ao centavo mais próximo. `/parcelas` exclui o mês atual; consulte `/mes` para esse mês.

## Persistência e consistência

* `expenses`: compra original, total em centavos, categoria, pagamento, data local (`YYYY-MM-DD`), quantidade de parcelas, `source_key` única do fluxo, timestamps UTC e cancelamento lógico.
* `allocations`: atribuição aos meses (`YYYY-MM`), número, valor inteiro e vencimento opcional. Contém uma linha no mês da compra para pagamentos à vista e uma por parcela para crédito.
* `processed_updates`: IDs já recebidos, retidos por sete dias. Não pressupõe IDs sequenciais; o Telegram retém updates por até 24 horas.

Chaves estrangeiras, índices por competência, unicidade `(expense_id, installment_number)`, validação dos totais e transação impedem gravações parciais. `PRAGMA user_version=1` controla o schema, criado automaticamente; versões futuras desconhecidas fazem o binário falhar. WAL, `synchronous=FULL`, espera por lock de 5 segundos e uma conexão por processo são configurados em cada conexão.

O fluxo temporário fica em memória. Um novo `/gasto` reinicia o anterior; `/desfazer` também encerra um registro incompleto e pede confirmação do ID específico exibido. Reinícios e expiração invalidam botões antigos. O processamento é serializado, cada fluxo tem chave aleatória e a compra mantém essa chave mesmo cancelada. Cliques repetidos não criam outra despesa nem cancelam outra compra.

Uma queda pode interromper um fluxo ou impedir que a confirmação chegue depois de salvar. Consulte `/mes` antes de iniciar outro lançamento. A biblioteca de polling pode confirmar o recebimento de updates antes do término do handler; o projeto garante consistência e ausência de duplicação dos registros persistidos, mas não promete reentrega de toda interação após uma queda.

## Estrutura

```text
cmd/bot/             entrada, sinais, menu e long polling
cmd/dbtool/          backup consistente e verificação offline, sem token
internal/config/    ambiente e validação
internal/finance/   moeda, categorias, calendário e parcelas
internal/storage/   schema, transações, consultas e cancelamento
internal/telegram/  comandos, botões, autorização e estado
deploy/             serviço systemd
scripts/            builds e backup
```

## Validar e compilar

```sh
go test ./...
go vet ./...
go build ./...
sh scripts/build.sh all
```

No PowerShell: `./scripts/build.ps1 -Arch all`. Os executáveis Linux ficam em `bin/linux-amd64/` e `bin/linux-arm64/`; cada pasta contém `finance-bot` e `finance-dbtool`. CGO é desabilitado. Testes usam `testing`, API simulada e SQLite temporário; não precisam de token ou rede. Cobrem moeda, calendário, autorização, concorrência, duplicação, rollback, cancelamento e restauração de backup.

Não há CI/CD, coleta de métricas ou dados financeiros em logs. O banco não é criptografado; proteja o disco, a conta Linux, o token e os backups. Não use duas instâncias do mesmo bot em paralelo.

## Referências

[Go oficial](https://go.dev/dl/), [biblioteca Telegram](https://github.com/go-telegram/bot), [driver SQLite](https://pkg.go.dev/modernc.org/sqlite), [criação de bots](https://core.telegram.org/bots/tutorial), [VACUUM INTO](https://www.sqlite.org/lang_vacuum.html), [systemd.exec](https://www.freedesktop.org/software/systemd/man/latest/systemd.exec.html).
