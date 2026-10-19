# Oracle Linux: continuar da etapa 8 usando git clone

Este roteiro pressupõe que o bot já foi criado no BotFather, que você já descobriu seu Telegram User ID e que o GitHub já está autenticado. O cartão usado nos exemplos fecha no dia 6 e vence no dia 12; esses dias são configuração, não uma regra geral do Inter.

Execute cada bloco no local indicado. Não precisa enviar executáveis por SCP. Você compilará os fontes diretamente na VM. A execução do serviço não depende de manter SSH aberto nem de manter o computador Windows ligado.

## Antes da etapa 8: publicar os fontes

O código precisa estar no GitHub antes de ser clonado na VM. Os arquivos foram preparados localmente; este roteiro não executa commit ou push automaticamente.

No **PowerShell do Windows**:

```powershell
Set-Location C:\Projetos\Git\telegram-bot-gestor-financa
git status --short --untracked-files=all
git check-ignore .env data/finance.db bin/windows-amd64/finance-bot.exe
git add --dry-run .
```

O `check-ignore` deve mostrar os três caminhos. O `dry-run` deve listar somente fontes, testes, scripts, documentação, `.env.example`, `.gitignore`, `.gitattributes`, `go.mod` e `go.sum`. Não inclua `.env`, banco, backups, logs, executáveis ou chaves SSH, nem use `git add -f` nesses arquivos.

Para efetivamente publicar, depois de conferir a lista:

```powershell
git add .
git diff --cached --stat
git diff --cached --check
git status --short
git commit -m "Implementa bot pessoal de finanças no Telegram"
git push -u origin main
```

O remoto configurado para este projeto é `https://github.com/VitorRinkawetsky/telegram-bot-gestor-financa.git`. Se publicar em outro repositório, ajuste a URL nos comandos de clone abaixo. `.env.example` contém somente marcadores de exemplo; não preencha nele seu token ou ID reais. Credenciais do GitHub permanecem no gerenciador de credenciais/SSH, fora do projeto.

## Etapa 8 — Acessar o Oracle Linux e conferir arquitetura

No **PowerShell do Windows**, ajuste IP e chave SSH:

```powershell
$VmAddress = 'opc@IP_DA_VM'
$SshKey = "$HOME\.ssh\oracle.key"
ssh -i $SshKey $VmAddress
```

Se você já está conectado à VM, continue diretamente pelos próximos comandos. Nas imagens padrão Oracle Linux o usuário costuma ser `opc`; use o usuário real da sua instância.

Na **VM Oracle Linux**:

```sh
cat /etc/os-release
uname -m
ps -p 1 -o comm=
```

O primeiro comando identifica versão/distribuição. `x86_64` corresponde a AMD64 e `aarch64` a ARM64. O último deve mostrar `systemd`. Os próximos comandos de compilação reconhecem a arquitetura automaticamente.

## Etapa 9 — Instalar os pacotes básicos

Todos os próximos blocos, salvo indicação contrária, são executados na **VM**:

```sh
sudo dnf makecache
sudo dnf install -y ca-certificates curl git tar nano
```

O bot não abre portas. Ele precisa de saída HTTPS TCP/443 e DNS para o Telegram; baixar Go e dependências também requer conexão de saída. Não altere regras de SSH ou firewall para instalar o bot.

## Etapa 10 — Instalar Go no usuário SSH

Use este bloco inteiro, que escolhe o arquivo correto e confere o SHA-256 publicado pelo projeto Go:

```sh
(
  set -eu
  case "$(uname -m)" in
    x86_64)
      GO_ARCH=amd64
      GO_SHA=ecbadb99091a3f46e31f5f934b068b1864eafa7995211b39eaddf76996045fe5
      ;;
    aarch64)
      GO_ARCH=arm64
      GO_SHA=94f3e30b8e374bc285e7dadc11e0865726b9bc6e85b841ccceaabc0214c6b7c8
      ;;
    *) echo 'Arquitetura não suportada por este roteiro.' >&2; exit 1 ;;
  esac
  GO_DOWNLOAD_DIR=$(mktemp -d)
  curl -fL "https://go.dev/dl/go1.27.2.linux-${GO_ARCH}.tar.gz" \
    -o "$GO_DOWNLOAD_DIR/go.tar.gz"
  printf '%s  %s\n' "$GO_SHA" "$GO_DOWNLOAD_DIR/go.tar.gz" | sha256sum -c -
  mkdir -p "$HOME/.local/lib/go1.27.2"
  tar -xzf "$GO_DOWNLOAD_DIR/go.tar.gz" \
    -C "$HOME/.local/lib/go1.27.2" --strip-components=1
)
```

O resultado da conferência deve terminar em `OK`; se ocorrer erro, corrija antes de seguir. O bloco não substitui instalações do sistema nem exige Go como root. Depois:

```sh
export PATH="$HOME/.local/lib/go1.27.2/bin:$PATH"
go version
```

O resultado esperado é `go version go1.27.2 linux/amd64` ou `linux/arm64`. Essa versão atende ao `go.mod`. Fonte dos arquivos/checksums: [downloads oficiais do Go](https://go.dev/dl/).

O `export` vale para a sessão atual. Em uma nova conexão SSH usada para compilar, repita essa linha. O serviço executará o binário compilado e não precisa de Go no PATH.

## Etapa 11 — Clonar, testar e compilar

Como o GitHub já está autenticado, clone diretamente. Não coloque token de acesso do GitHub na URL:

```sh
cd "$HOME"
git clone https://github.com/VitorRinkawetsky/telegram-bot-gestor-financa.git
cd "$HOME/telegram-bot-gestor-financa"
git log -1 --oneline
go mod download
go test ./...
go vet ./...
sh scripts/build.sh
```

Confira o sucesso de cada comando antes de prosseguir. `git log -1` ajuda a confirmar que o commit publicado no Windows chegou à VM. Se a pasta já existir, use `cd` nela e `git pull --ff-only` para obter a versão publicada; não clone por cima nem apague a pasta.

Se seu acesso GitHub usa exclusivamente SSH, substitua apenas o comando de clone por:

```sh
git clone git@github.com:VitorRinkawetsky/telegram-bot-gestor-financa.git
```

Os testes não precisam do token do Telegram. A compilação desabilita CGO e gera `finance-bot` e `finance-dbtool` em `bin/linux-amd64` ou `bin/linux-arm64`. Não é necessário GCC nem instalar SQLite no sistema. A primeira compilação pode demorar; não é o consumo esperado do serviço em execução.

## Etapa 12 — Criar o usuário de serviço e os diretórios

```sh
if ! id finance-bot >/dev/null 2>&1; then
  NOLOGIN=$(command -v nologin || printf '/sbin/nologin')
  sudo useradd --system --user-group --home-dir /var/lib/finance-bot \
    --no-create-home --shell "$NOLOGIN" finance-bot
fi
sudo install -d -o root -g root -m 0755 /opt/finance-bot
sudo install -d -o root -g root -m 0700 /etc/finance-bot
sudo install -d -o finance-bot -g finance-bot -m 0700 /var/lib/finance-bot
sudo install -d -o finance-bot -g finance-bot -m 0700 /var/lib/finance-bot/backups
```

A conta `finance-bot` executará o serviço sem privilégios. A conta SSH mantém os fontes no seu home; o serviço usa executáveis em `/opt` e dados em `/var/lib`.

## Etapa 13 — Instalar os binários

Este passo é para a primeira instalação. Se um serviço já estiver rodando, siga o procedimento de atualização com parada e backup em DEPLOY.md.

```sh
cd "$HOME/telegram-bot-gestor-financa"
case "$(uname -m)" in x86_64) ARCH=amd64 ;; aarch64) ARCH=arm64 ;; esac
sudo install -o root -g root -m 0755 \
  "bin/linux-$ARCH/finance-bot" /opt/finance-bot/finance-bot
sudo install -o root -g root -m 0755 \
  "bin/linux-$ARCH/finance-dbtool" /opt/finance-bot/finance-dbtool
sudo install -o root -g root -m 0755 scripts/backup.sh /opt/finance-bot/backup.sh
```

## Etapa 14 — Configurar token, seu ID e o cartão

Crie o arquivo privado a partir do modelo **somente se ainda não existir**:

```sh
if ! sudo test -e /etc/finance-bot/finance-bot.env; then
  sudo install -o root -g root -m 0600 \
    .env.example /etc/finance-bot/finance-bot.env
fi
sudo nano /etc/finance-bot/finance-bot.env
```

No editor, preencha:

```dotenv
TELEGRAM_BOT_TOKEN=COLE_AQUI_O_TOKEN_COMPLETO_DO_BOTFATHER
ALLOWED_TELEGRAM_USER_ID=COLE_AQUI_SEU_ID_NUMERICO_REAL
DATABASE_PATH=/var/lib/finance-bot/finance.db
TIMEZONE=America/Sao_Paulo
STATE_TTL=15m
CREDIT_CLOSING_DAY=6
CREDIT_DUE_DAY=12
```

Substitua os marcadores de token e ID; não use o número do bot como seu ID. Salve no nano com **Ctrl+O**, confirme com **Enter** e saia com **Ctrl+X**.

O arquivo está fora do repositório: `git push` e `git pull` não o incluem. O `.env` local do Windows também não será clonado. O systemd lê o arquivo protegido ao iniciar o serviço; não é necessário exportar o token no terminal da VM.

Com fechamento 6 e vencimento 12, uma compra em 01/11 é prevista para 12/11; em 07/11, para 12/12. No próprio dia 6, o bot considera o ciclo seguinte. É uma projeção com dias fixos; confira a data efetiva da fatura perto do fechamento. Alterar os dias afeta apenas novos registros.

## Etapa 15 — Instalar e iniciar o serviço systemd

Antes deste passo, pare o bot que estiver rodando no Windows com **Ctrl+C**. Mantenha uma única instância com o mesmo token.

```sh
cd "$HOME/telegram-bot-gestor-financa"
sudo install -o root -g root -m 0644 \
  deploy/finance-bot.service /etc/systemd/system/finance-bot.service
sudo systemd-analyze verify /etc/systemd/system/finance-bot.service
sudo systemctl daemon-reload
sudo systemctl enable --now finance-bot.service
sudo systemctl status finance-bot.service --no-pager
sudo journalctl -u finance-bot.service -n 50 --no-pager
```

O esperado é **active (running)** e a mensagem `Bot iniciado com long polling`. `enable --now` inicia agora e configura o início no boot, conforme a [documentação do Oracle Linux](https://docs.oracle.com/en/operating-systems/oracle-linux/9/systemd/EnablingandDisablingServices.html). O SQLite e o schema são criados automaticamente.

Se aparecer `failed`, leia os logs antes de repetir comandos. Não imprima nem compartilhe o arquivo de configuração com o token. Para corrigir uma variável e carregar o novo valor:

```sh
sudo nano /etc/finance-bot/finance-bot.env
sudo systemctl restart finance-bot.service
sudo systemctl status finance-bot.service --no-pager
```

## Etapa 16 — Conferir pelo Telegram e pelo banco

Na conversa privada do seu bot, envie `/start`, registre `/gasto 1`, escolha categoria e Pix, confira `/mes` e remova o teste com `/desfazer`, confirmando no botão. Teste crédito e `/parcelas` se desejar validar a projeção.

Na VM:

```sh
sudo -u finance-bot /opt/finance-bot/finance-dbtool \
  -action check -database /var/lib/finance-bot/finance.db
sudo systemctl is-enabled finance-bot.service
sudo systemctl is-active finance-bot.service
```

O verificador deve informar `OK`; os outros comandos, `enabled` e `active`. Você pode encerrar o SSH com `exit`: o serviço continua executando.

Se o verificador disser que o arquivo não existe ou não pode ser acessado, `active` por si só não confirma o caminho usado pelo banco. Execute somente o diagnóstico abaixo antes de criar, mover ou substituir arquivos:

```sh
sudo journalctl -u finance-bot.service -n 30 --no-pager
sudo grep -nE '^[[:space:]]*DATABASE_PATH[[:space:]]*=' /etc/finance-bot/finance-bot.env
sudo find /var/lib/finance-bot -maxdepth 3 -type f -name '*.db' -printf '%u:%g %m %p\n'
sudo -u finance-bot stat /var/lib/finance-bot/finance.db
```

Com a unit fornecida, `WorkingDirectory=/var/lib/finance-bot`. Se `DATABASE_PATH` estiver ausente ou com `./data/finance.db`, o caminho usado será `/var/lib/finance-bot/data/finance.db`, diferente do caminho do comando de verificação acima. Localize o banco original e confirme o erro antes de alterar a configuração, para não passar a usar outro banco vazio. A mensagem genérica do dbtool inicial também podia indicar falta de permissão; versões atualizadas distinguem essas condições. Não compartilhe o conteúdo completo do arquivo de ambiente.

## Etapa 17 — Fazer backup e guardar fora da VM

Na VM:

```sh
sudo -u finance-bot /opt/finance-bot/backup.sh
sudo ls -lh /var/lib/finance-bot/backups
```

O script cria e verifica uma fotografia consistente, incluindo dados que estejam no WAL. Guarde uma cópia **fora da VM** usando o procedimento de transferência da parte F de [DEPLOY.md](DEPLOY.md). Banco e backups contêm dados pessoais e não devem ser colocados no GitHub.

## Atualizações futuras usando o clone

Na VM, obtenha a nova versão e compile antes de parar o serviço:

```sh
export PATH="$HOME/.local/lib/go1.27.2/bin:$PATH"
cd "$HOME/telegram-bot-gestor-financa"
git pull --ff-only
go mod download
go test ./...
go vet ./...
sh scripts/build.sh
```

Se algum comando falhar, resolva antes de substituir os binários. Em seguida, siga a parte E de [DEPLOY.md](DEPLOY.md): parada do serviço, backup, preservação do binário anterior, instalação e reinício. O `.env` de produção e o banco permanecem fora do clone.
