# Implantação no Oracle Linux: Telegram, configuração e produção

Este tutorial parte da sua VM **Oracle Linux já criada**, acessível por SSH e com systemd. O roteiro usa `dnf`, o usuário SSH padrão `opc` (caso sua imagem o utilize) e os binários Linux compilados sem CGO para AMD64 e ARM64. Não é necessário abrir portas para o bot. Execute uma única instância por token.

As credenciais e o acesso à sua VM não foram fornecidos: a implantação real e o teste pelo seu Telegram são passos manuais. Os testes automatizados não acessam serviços externos. Leia cada bloco antes de executar; substitua `IP_DA_VM`, `USUARIO_SSH` e o caminho da chave por seus dados. Não cole os marcadores literalmente.

**Caminho recomendado com GitHub já autenticado:** conclua A e B para criar o bot e descobrir seu ID; depois siga o [roteiro a partir da etapa 8 para Oracle Linux](ORACLE_LINUX.md), que usa `git clone` e compilação diretamente na VM. As partes C e D abaixo também documentam a alternativa de transferência manual de executáveis, que dispensa Go na VM.

| Onde executar | Como reconhecer |
| --- | --- |
| Telegram | Mensagens enviadas ao BotFather ou ao seu próprio bot |
| PowerShell do Windows | Terminal do seu computador, fora da sessão SSH |
| Oracle Linux | Terminal depois de entrar por `ssh`; normalmente aparece `[opc@... ~]$` |

Token e Telegram User ID são informações diferentes. O **token identifica e autoriza o programa a operar seu bot**. O **User ID identifica sua conta pessoal**, que terá acesso aos dados. A parte numérica antes de `:` no token pertence ao bot e não deve ser usada como seu User ID.

## A. Criar o bot e obter suas informações

### A.1. Abrir o BotFather correto

No Telegram, abra [@BotFather](https://t.me/BotFather), confira o username e o selo de verificação e toque em **Iniciar / Start**. O BotFather cria e administra a conta do bot. A aplicação Go deste projeto executará as funções financeiras. Referência: [tutorial oficial](https://core.telegram.org/bots/tutorial).

Envie ao **BotFather**:

```text
/newbot
```

Ele pedirá duas informações em mensagens separadas:

1. **Nome de exibição**: envie, por exemplo, `Minhas Finanças`. É o nome mostrado na conversa e pode conter espaços e acentos.
2. **Username**: envie um nome disponível, sem `@`, por exemplo `financas_rinka_pessoal_bot`. Ele deve ter 5–32 caracteres, usar letras latinas, números ou `_`, e terminar em `bot`. Se estiver ocupado, escolha outro e tente novamente. Esse username compõe o link para abrir seu bot.

### A.2. Guardar o token e abrir seu bot

Após criar a conta, o BotFather envia o link do bot e o token. O token é uma linha com números, `:` e um segredo longo. Copie **somente essa linha**, sem o texto ao redor, para um gerenciador de senhas. Você o colocará no arquivo `.env` em B.

Abra o link enviado pelo BotFather e toque em **Iniciar / Start** na conversa do seu bot. **Nesse momento ele pode não responder:** a conta foi criada, mas o programa ainda não foi iniciado. Mais adiante, durante o bootstrap com ID provisório, somente `/id` responderá; `/start` e consultas financeiras continuam restritos.

Não envie o token como mensagem ao seu próprio bot ou neste chat, não faça commit e não o coloque em URLs de diagnóstico. Quem tiver o token pode operar o bot. Se vazar, use `/revoke` no BotFather, selecione o bot, siga as instruções para obter a nova credencial e atualize a configuração da aplicação.

### A.3. Definir o menu de comandos

O programa registra o menu automaticamente ao iniciar. Você pode deixar essa etapa para ele. Para cadastrar manualmente:

1. Volte à conversa com o **BotFather**, não à conversa do seu bot.
2. Envie `/setcommands`.
3. Selecione seu bot ou envie o `@username` dele quando solicitado.
4. Cole a mensagem abaixo inteira, sem as cercas de código e **sem `/` antes dos nomes**:

```text
start - Boas-vindas e comandos
ajuda - Como usar o bot
gasto - Registrar despesa
mes - Resumo mensal por competência
periodo - Resumo entre dois meses
categorias - Gastos por categoria
parcelas - Previsão dos próximos meses
desfazer - Cancelar o último lançamento
cancelar - Cancelar o registro em andamento
id - Mostrar seu Telegram User ID
```

Espere a confirmação de que a lista foi atualizada. Quando o seu bot estiver executando, abra a conversa dele e digite `/` para ver os comandos. O menu só lista os comandos: quem registra despesas e responde é o programa executando no Windows ou na VM.

### A.4. Ajustes do perfil e de grupos

Esses ajustes são opcionais; a autorização financeira já é feita no código pelo seu User ID.

* Para impedir que adicionem o bot a grupos: envie `/setjoingroups` ao BotFather, selecione o bot e escolha **Disable / Desativar**. Use a opção equivalente se os rótulos estiverem traduzidos.
* Não é necessário desativar o modo de privacidade de grupos (`/setprivacy`) para usar conversa privada.
* Se quiser uma descrição: `/setdescription` → selecione o bot → envie `Registro pessoal de despesas e previsão de parcelas. Acesso restrito ao proprietário.`

O bot ainda poderá ser encontrado pelo username; esses ajustes não o tornam invisível. A proteção dos dados vem da verificação do seu ID em cada comando e botão. Referência: [configuração do BotFather](https://core.telegram.org/bots/features#botfather).

### A.5. O que anotar antes de continuar

Você já deve ter o **username do bot** e o **token**. Falta descobrir o ID da sua conta pessoal. Isso será feito em B com o comando `/id` do próprio projeto. Não é necessário criar aplicativo em `my.telegram.org`, cadastrar cartão, configurar webhook ou contratar hospedagem adicional.

## B. Configurar no Windows e descobrir seu Telegram User ID

### B.1. Abrir a pasta e criar o arquivo de configuração

Abra o **PowerShell do Windows**, fora da sessão SSH. Os executáveis já gerados nesta entrega permitem testar sem instalar Go:

```powershell
Set-Location C:\Projetos\Git\telegram-bot-gestor-financa
Test-Path ./bin/windows-amd64/finance-bot.exe
```

O resultado esperado é `True`. Se for `False`, use a alternativa de compilação em B.5 antes de continuar.

Crie `.env` somente se ainda não existir e abra-o:

```powershell
if (-not (Test-Path -LiteralPath .env)) {
    Copy-Item .env.example .env
}
notepad .env
```

Substitua o conteúdo pelo modelo abaixo, preenchendo o token **no editor**:

```dotenv
TELEGRAM_BOT_TOKEN=COLE_AQUI_O_TOKEN_COMPLETO_DO_BOTFATHER
ALLOWED_TELEGRAM_USER_ID=1
DATABASE_PATH=./data/finance.db
TIMEZONE=America/Sao_Paulo
STATE_TTL=15m
CREDIT_CLOSING_DAY=6
CREDIT_DUE_DAY=12
```

Salve com Ctrl+S e feche o editor. O arquivo precisa se chamar `.env`, sem `.txt`. Para conferir o nome sem mostrar o conteúdo:

```powershell
Get-Item -LiteralPath .env -Force | Select-Object Name, Length
```

Não acrescente aspas ou espaços em torno de `=`. O número `1` é provisório: ele permite iniciar a aplicação enquanto você descobre seu ID. Os comandos financeiros permanecem restritos ao ID configurado. Os dias `6` e `12` correspondem ao fechamento e vencimento que você informou para seu cartão Inter.

### B.2. Carregar as variáveis e iniciar o programa

Na mesma janela do PowerShell, execute o bloco inteiro:

```powershell
Get-Content -LiteralPath .env | ForEach-Object {
    if ($_ -match '^([A-Z_]+)=(.*)$') {
        [Environment]::SetEnvironmentVariable($Matches[1], $Matches[2].Trim(), 'Process')
    }
}
./bin/windows-amd64/finance-bot.exe
```

Espere aparecer `Bot iniciado com long polling`. O terminal ficará ocupado: isso significa que o bot está executando. Não feche essa janela por enquanto.

Se sair imediatamente, leia a mensagem: token ausente/formato inválido indica configuração não carregada ou preenchida incorretamente; falha ao validar o bot pode indicar token incorreto ou falta de conexão. O programa não mostra o token nos logs.

Se estiver usando o executável inicial e `/id` resultar apenas em `WARN Não foi possível enviar mensagem pelo Telegram`, atualize para a versão corrigida. A versão inicial enviava um teclado nulo em mensagens de texto simples. Quando o arquivo `finance-bot-corrigido.exe` estiver disponível, pare o processo antigo com **Ctrl+C** e, na mesma janela, execute:

```powershell
Copy-Item ./bin/windows-amd64/finance-bot-corrigido.exe ./bin/windows-amd64/finance-bot.exe -Force
./bin/windows-amd64/finance-bot.exe
```

Depois envie `/id` novamente. Se abrir outra janela do PowerShell, repita o carregamento das variáveis de B.2 antes de executar. A versão corrigida também informa uma `causa` segura nos avisos: erro 400 de formato, 403 de envio proibido, 429 de limite ou falha de rede, sem revelar token ou conteúdo financeiro.

### B.3. Descobrir o ID da sua conta e autorizar o acesso

1. No Telegram, abra a conversa privada do **seu bot**, usando o link recebido do BotFather. Não envie os próximos comandos ao BotFather.
2. Envie `/id`. O programa deve responder, por exemplo, `Seu Telegram User ID: 123456789`. Esse número de exemplo não é necessariamente seu ID.
3. Copie o número que o seu bot retornou.
4. Volte ao PowerShell ocupado e pressione **Ctrl+C**. O programa deverá encerrar e devolver o prompt.
5. Execute `notepad .env`. Troque apenas `ALLOWED_TELEGRAM_USER_ID=1` por `ALLOWED_TELEGRAM_USER_ID=SEU_NUMERO_REAL`, sem `@` e sem aspas. Salve e feche.
6. **Repita o bloco inteiro de B.2**, inclusive o carregamento de variáveis, e então envie `/start` ao seu bot.

O `.env` não é lido automaticamente pelo executável. Editar o arquivo sem repetir o carregamento de variáveis mantém o valor antigo no PowerShell. Em produção, será o systemd que carregará a configuração ao iniciar/reiniciar o serviço.

### B.4. Confirmar registro, consulta e cancelamento

No Telegram, com o programa ainda executando no Windows:

1. Envie `/start`. Agora a mensagem de boas-vindas deve aparecer.
2. Envie `/gasto 42,50`, toque em **Mercado** e **Pix**. Espere a confirmação.
3. Envie `/mes` e confira R$ 42,50 em Mercado se esse era o único lançamento.
4. Envie `/desfazer` e toque em **Sim, desfazer**. Consulte `/mes` novamente.
5. Envie `/gasto 100`, escolha **Compras**, **Crédito**, **3x**. As parcelas serão R$ 33,34, R$ 33,33 e R$ 33,33. Com fechamento no dia 6 e vencimento no dia 12, compras dos dias 1 a 5 começam no próprio mês; pelo modelo do bot, compras do dia 6 em diante começam no mês seguinte.
6. Consulte `/parcelas` e cancele essa compra de teste com `/desfazer`.
7. Pare o programa Windows com **Ctrl+C antes de iniciar na VM**. Duas instâncias com o mesmo token disputam os updates.

Anote seu User ID para a configuração da VM. O banco local é separado do banco de produção; os testes acima não serão transferidos quando você enviar somente os executáveis.

### B.5. Alternativa: instalar Go e compilar no computador de desenvolvimento

Esta seção é opcional se o executável de B.1 já existir.

#### Instalar Go e obter o código

Use Go 1.26 ou superior. Este projeto foi validado com **Go 1.27.2**. Obtenha a versão para seu sistema em [go.dev/dl](https://go.dev/dl/), confira o SHA-256 e use o instalador oficial. No Windows, uma opção é:

```powershell
winget install --id GoLang.Go --exact --source winget
# Abra um novo terminal depois da instalação.
go version
```

O código entregue está no workspace `C:\Projetos\Git\telegram-bot-gestor-financa`. Não foi feito push automaticamente. Use essa pasta local. Se posteriormente publicar os arquivos no repositório, em outra máquina poderá usar:

```sh
git clone https://github.com/VitorRinkawetsky/telegram-bot-gestor-financa.git
cd telegram-bot-gestor-financa
go mod download
```

No workspace atual, para compilar o executável local:

```powershell
Set-Location C:\Projetos\Git\telegram-bot-gestor-financa
go version
go mod download
New-Item -ItemType Directory -Force bin/windows-amd64 | Out-Null
go build -trimpath -o bin/windows-amd64/finance-bot.exe ./cmd/bot
```

Depois volte a B.1. Para desenvolver sem gerar um executável a cada alteração, carregue o ambiente como em B.2 e use `go run ./cmd/bot` no lugar de executar o `.exe`.

#### Alternativa para desenvolvimento em Linux/macOS

Em **Linux/macOS**, na pasta do projeto:

```sh
cp .env.example .env
chmod 600 .env
nano .env
set -a
. ./.env
set +a
go run ./cmd/bot
```

Carregue somente arquivos de configuração que você criou e conferiu. A ausência de token, ID positivo, timezone válido ou par completo de dias do cartão impede a inicialização. A aplicação também verifica o token junto ao Telegram.

Execute as validações automatizadas:

```sh
go test ./...
go vet ./...
go build ./...
```

Não use dados reais nos testes manuais que pretende descartar. O banco local e o de produção são independentes. Para levar dados locais, use o procedimento de backup e restauração da parte F; não copie um banco aberto isoladamente.

## C. Preparar a VM Oracle Linux existente

### Entrar por SSH e identificar o ambiente

No computador local:

```sh
ssh -i /caminho/da/chave.pem USUARIO_SSH@IP_DA_VM
```

No PowerShell, por exemplo:

```powershell
ssh -i "$HOME\.ssh\oracle.key" opc@IP_DA_VM
```

Em imagens Oracle Linux da OCI, o usuário inicial normalmente é `opc`; se sua conta de acesso for outra, substitua-o. Na primeira conexão, confira a impressão digital do servidor por um meio confiável antes de aceitá-la. Referência: [acesso à instância OCI](https://docs.oracle.com/en-us/iaas/Content/Compute/Tasks/accessinginstance.htm).

Na **VM**:

```sh
cat /etc/os-release
uname -m
ps -p 1 -o comm=
```

* `x86_64` → binários `linux-amd64`.
* `aarch64` → binários `linux-arm64`.
* O último comando deve indicar `systemd`. Outra arquitetura ou outro init requer adaptação e não faz parte deste tutorial.

O arquivo `/etc/os-release` deve identificar Oracle Linux e a versão da sua imagem. Atualize o catálogo e instale os pacotes básicos com `dnf`:

```sh
sudo dnf makecache
sudo dnf install -y ca-certificates curl git tar nano
```

Não é necessário instalar SQLite, GCC, Nginx ou Go se enviar os binários prontos. O editor `nano` foi incluído para facilitar a edição da configuração mais adiante.

### Conferir firewall e proteger SSH

Inspeção, sem modificar regras:

```sh
sudo ss -lntp
command -v firewall-cmd >/dev/null && sudo firewall-cmd --list-all
```

Confira também as regras já existentes no console da Oracle. O bot precisa de saída HTTPS TCP/443 e resolução DNS; não precisa de entrada pública adicional. Mantenha somente o SSH necessário e as regras de outros serviços que você já utiliza. Proteja a chave privada, prefira login por chave e restrinja a origem do SSH quando souber fazê-lo. **Não habilite firewall nem altere SSH durante esta instalação**; isso poderia bloquear seu acesso remoto.

### Criar usuário e diretórios

Na VM, execute uma vez:

```sh
NOLOGIN=$(command -v nologin || printf '/sbin/nologin')
sudo useradd --system --user-group --home-dir /var/lib/finance-bot \
  --no-create-home --shell "$NOLOGIN" finance-bot
sudo install -d -o root -g root -m 0755 /opt/finance-bot
sudo install -d -o root -g root -m 0700 /etc/finance-bot
sudo install -d -o finance-bot -g finance-bot -m 0700 /var/lib/finance-bot
sudo install -d -o finance-bot -g finance-bot -m 0700 /var/lib/finance-bot/backups
```

Se a conta já existir por uma instalação anterior, confirme com `id finance-bot` em vez de recriá-la. O usuário de serviço não precisa de login, shell interativo ou sudo.

| Diretório | Conteúdo |
| --- | --- |
| `/opt/finance-bot` | Binários e script de backup, pertencentes a root |
| `/etc/finance-bot/finance-bot.env` | Token e configuração, root, modo 0600 |
| `/var/lib/finance-bot/finance.db` | SQLite e seus arquivos WAL/SHM, usuário do serviço |
| `/var/lib/finance-bot/backups` | Fotografias locais temporárias; copie para fora da VM |

O systemd lê o arquivo de ambiente antes de iniciar o processo sem privilégios. Não é necessário conceder ao usuário do serviço acesso de leitura ao arquivo de segredos.

## D. Compilar e publicar

### Caminho recomendado: enviar os binários já gerados

Para a primeira instalação, os arquivos desta entrega já estão em `bin/linux-amd64` e `bin/linux-arm64`. A arquitetura da VM continua precisando ser conferida com `uname -m`. No **PowerShell do Windows, fora da sessão SSH**:

```powershell
Set-Location C:\Projetos\Git\telegram-bot-gestor-financa
$VmAddress = 'opc@IP_DA_VM'
$SshKey = "$HOME\.ssh\oracle.key"
$TargetArch = 'arm64' # aarch64 => arm64; x86_64 => amd64.
Test-Path -LiteralPath $SshKey
Test-Path "bin/linux-$TargetArch/finance-bot"
ssh -i $SshKey $VmAddress 'mkdir -p ~/finance-bot-upload'
scp -i $SshKey "bin/linux-$TargetArch/finance-bot" "bin/linux-$TargetArch/finance-dbtool" deploy/finance-bot.service scripts/backup.sh .env.example "${VmAddress}:~/finance-bot-upload/"
```

Troque o IP e o caminho da chave pelos seus dados. Os dois `Test-Path` devem retornar `True`; se algum retornar `False`, corrija o caminho antes de prosseguir. Esse envio leva o **modelo sem segredos**, não seu `.env` local.

Quando o `scp` terminar sem erros, pule as duas alternativas de compilação abaixo e siga diretamente para **Instalação comum na VM**. Se preferir reconstruir os arquivos, escolha **uma** alternativa abaixo.

### Opção 1 — Compilar na VM

Instale Go no seu usuário SSH, sem substituir instalações do sistema. Abaixo, uma versão fixa verificada; para atualizá-la, consulte os downloads oficiais e substitua também o checksum:

```sh
case "$(uname -m)" in
  x86_64) GOARCH=amd64; GOSHA=ecbadb99091a3f46e31f5f934b068b1864eafa7995211b39eaddf76996045fe5 ;;
  aarch64) GOARCH=arm64; GOSHA=94f3e30b8e374bc285e7dadc11e0865726b9bc6e85b841ccceaabc0214c6b7c8 ;;
  *) echo 'Arquitetura não suportada'; exit 1 ;;
esac
curl -fL "https://go.dev/dl/go1.27.2.linux-${GOARCH}.tar.gz" -o /tmp/finance-go.tar.gz
printf '%s  %s\n' "$GOSHA" /tmp/finance-go.tar.gz | sha256sum -c -
mkdir -p "$HOME/.local/lib/go1.27.2"
tar -xzf /tmp/finance-go.tar.gz -C "$HOME/.local/lib/go1.27.2" --strip-components=1
export PATH="$HOME/.local/lib/go1.27.2/bin:$PATH"
go version
```

O `export` vale para o terminal atual; repita em sessões futuras de compilação. Nenhum Go é necessário para executar o serviço.

Se o código já estiver publicado no GitHub:

```sh
git clone https://github.com/VitorRinkawetsky/telegram-bot-gestor-financa.git
cd telegram-bot-gestor-financa
```

Se ainda estiver somente no workspace, envie um pacote de fontes. No **computador local**, com `tar` disponível, na pasta do projeto:

```sh
tar --exclude=.git --exclude=.env --exclude=bin --exclude=data --exclude=backups \
  --exclude='*.db' --exclude='*.db-*' --exclude=finance-bot-source.tar.gz \
  -czf finance-bot-source.tar.gz .
scp -i /caminho/da/chave.pem finance-bot-source.tar.gz USUARIO_SSH@IP_DA_VM:~/
```

Na VM, extraia esse pacote criado por você em uma pasta nova:

```sh
mkdir -p "$HOME/finance-bot-source"
tar -xzf "$HOME/finance-bot-source.tar.gz" -C "$HOME/finance-bot-source"
cd "$HOME/finance-bot-source"
```

Compile na VM dentro da pasta de fontes escolhida:

```sh
go mod download
go test ./...
go vet ./...
sh scripts/build.sh
case "$(uname -m)" in x86_64) ARCH=amd64 ;; aarch64) ARCH=arm64 ;; esac
mkdir -p "$HOME/finance-bot-upload"
cp "bin/linux-$ARCH/finance-bot" "bin/linux-$ARCH/finance-dbtool" "$HOME/finance-bot-upload/"
cp deploy/finance-bot.service scripts/backup.sh .env.example "$HOME/finance-bot-upload/"
```

### Opção 2 — Compilar em outra máquina e enviar

No **Windows/PowerShell**, na pasta do projeto:

```powershell
go test ./...
go vet ./...
./scripts/build.ps1 -Arch all
$VmAddress = 'USUARIO_SSH@IP_DA_VM'
$SshKey = "$HOME\.ssh\oracle.key"
$TargetArch = 'arm64' # Troque para amd64 se uname -m retornar x86_64.
ssh -i $SshKey $VmAddress 'mkdir -p ~/finance-bot-upload'
scp -i $SshKey "bin/linux-$TargetArch/finance-bot" "bin/linux-$TargetArch/finance-dbtool" deploy/finance-bot.service scripts/backup.sh .env.example "${VmAddress}:~/finance-bot-upload/"
```

Se a execução de `.ps1` estiver bloqueada, use `powershell -NoProfile -ExecutionPolicy Bypass -File scripts/build.ps1 -Arch all` para este script local revisado, ou compile explicitamente:

```powershell
$env:CGO_ENABLED='0'
$env:GOOS='linux'
$env:GOARCH='arm64' # ou amd64
New-Item -ItemType Directory -Force bin/upload | Out-Null
go build -trimpath -ldflags='-s -w' -o bin/upload/finance-bot ./cmd/bot
go build -trimpath -ldflags='-s -w' -o bin/upload/finance-dbtool ./cmd/dbtool
Remove-Item Env:GOOS, Env:GOARCH, Env:CGO_ENABLED
```

Esse último bloco grava em `bin/upload`, então ajuste o `scp` para essa pasta. Remova as variáveis de compilação cruzada antes de tentar executar `go run` ou testes no Windows.

Em **Linux/macOS**:

```sh
go test ./...
go vet ./...
sh scripts/build.sh all
ssh -i /caminho/da/chave.pem USUARIO_SSH@IP_DA_VM 'mkdir -p ~/finance-bot-upload'
# Escolha linux-arm64 ou linux-amd64 conforme a VM:
scp -i /caminho/da/chave.pem bin/linux-arm64/finance-bot bin/linux-arm64/finance-dbtool \
  deploy/finance-bot.service scripts/backup.sh .env.example USUARIO_SSH@IP_DA_VM:~/finance-bot-upload/
```

### Instalação comum na VM

Na **VM**, com os arquivos em `~/finance-bot-upload`:

```sh
cd "$HOME/finance-bot-upload"
sudo install -o root -g root -m 0755 finance-bot /opt/finance-bot/finance-bot
sudo install -o root -g root -m 0755 finance-dbtool /opt/finance-bot/finance-dbtool
sudo install -o root -g root -m 0755 backup.sh /opt/finance-bot/backup.sh
sudo install -o root -g root -m 0600 .env.example /etc/finance-bot/finance-bot.env
sudo nano /etc/finance-bot/finance-bot.env
```

No editor, preencha o token verdadeiro, seu ID já descoberto e **use este caminho absoluto**:

```dotenv
TELEGRAM_BOT_TOKEN=SEU_TOKEN_REAL
ALLOWED_TELEGRAM_USER_ID=SEU_NUMERO_REAL
DATABASE_PATH=/var/lib/finance-bot/finance.db
TIMEZONE=America/Sao_Paulo
STATE_TTL=15m
CREDIT_CLOSING_DAY=6
CREDIT_DUE_DAY=12
```

Substitua `SEU_TOKEN_REAL` pelo token completo e `SEU_NUMERO_REAL` pelo ID obtido em B.3. O ID não é o username nem o número que aparece antes dos dois-pontos no token.

No nano: **Ctrl+O** salva, **Enter** confirma o nome e **Ctrl+X** sai. O token permanece neste arquivo privado da VM; não é necessário colocá-lo em nenhum comando do terminal.

Para seu cartão Inter, você informou fechamento no dia **6** e vencimento no dia **12**. Use `CREDIT_CLOSING_DAY=6` e `CREDIT_DUE_DAY=12`, como no bloco acima. Uma compra em 01/11 é prevista para 12/11; uma compra em 07/11, para 12/12. Pelo modelo do bot, uma compra no próprio dia 6 também entra no próximo ciclo. A alteração vale para novos registros e não recalcula compras já salvas.

O bot usa esses dias fixos como previsão. O Inter informa que o fechamento pode variar em relação ao vencimento e ser antecipado em determinadas situações; confira a data efetiva no aplicativo perto do corte. Referência: [fechamento da fatura no Inter](https://ajuda.inter.co/cartao/qual-a-data-de-fechamento-da-minha-fatura). A convenção detalhada do projeto está ao final deste documento.

Não rode novamente o comando `install .env.example` em uma atualização: ele substituiria seus segredos pelo modelo.

Instale e inicie o serviço:

```sh
sudo install -o root -g root -m 0644 finance-bot.service /etc/systemd/system/finance-bot.service
sudo systemd-analyze verify /etc/systemd/system/finance-bot.service
sudo systemctl daemon-reload
sudo systemctl enable --now finance-bot.service
sudo systemctl status finance-bot.service --no-pager
sudo journalctl -u finance-bot.service -n 50 --no-pager
```

O status esperado é **active (running)**. `enabled` indica que o serviço está configurado para iniciar no boot. O menu do Telegram é registrado durante a inicialização. Se aparecer **failed**, não repita a instalação: leia os logs e consulte G.

Se você corrigir o token, o ID ou o cartão depois, edite o arquivo e **reinicie o serviço** para carregar os novos valores:

```sh
sudo nano /etc/finance-bot/finance-bot.env
sudo systemctl restart finance-bot.service
sudo systemctl status finance-bot.service --no-pager
```

Ao concluir as verificações abaixo, você pode sair do SSH com `exit`. O systemd mantém o bot executando mesmo com seu computador desligado.

O SQLite e o schema são criados automaticamente no primeiro início válido. Verifique sem exibir segredos:

```sh
sudo ls -ld /etc/finance-bot /var/lib/finance-bot
sudo ls -l /var/lib/finance-bot/finance.db*
sudo -u finance-bot /opt/finance-bot/finance-dbtool \
  -action check -database /var/lib/finance-bot/finance.db
```

Não use o dbtool em um caminho inexistente; ele recusa essa condição para não criar um banco vazio por engano. O próprio bot cria um banco novo se o caminho configurado mudar; mantenha `DATABASE_PATH` estável.

No Telegram, envie `/start`, registre uma compra pequena de teste, consulte `/mes` e cancele-a com `/desfazer`. Confira também crédito e `/parcelas`. Esse é o teste real de produção que você precisa executar.

Operação cotidiana:

```sh
sudo journalctl -u finance-bot.service -f
# Ctrl+C sai somente da visualização dos logs.
sudo systemctl stop finance-bot.service
sudo systemctl start finance-bot.service
sudo systemctl restart finance-bot.service
sudo systemctl is-enabled finance-bot.service
sudo systemctl is-active finance-bot.service
```

O serviço inicia no boot e reinicia após falhas, aguardando 10 segundos entre tentativas. `systemctl stop` é uma parada intencional e não provoca reinício. O processo responde a SIGTERM. A configuração restringe escrita a `/var/lib/finance-bot`, além da área temporária privada. Se usar outros diretórios, adapte também `ReadWritePaths` e permissões. Nenhum mecanismo gera carga artificial na VM.

## E. Atualizar com segurança

1. Obtenha o código novo (`git pull --ff-only`, se já publicado, ou novo pacote de fontes).
2. Rode os testes, `go vet` e compile para a arquitetura correta. Envie os novos binários para `~/finance-bot-upload`, sem substituir os que estão executando.
3. Na VM, pare o serviço, faça backup preventivo e guarde os executáveis anteriores:

```sh
sudo systemctl stop finance-bot.service
sudo -u finance-bot /opt/finance-bot/backup.sh
sudo cp -p /opt/finance-bot/finance-bot /opt/finance-bot/finance-bot.previous
sudo cp -p /opt/finance-bot/finance-dbtool /opt/finance-bot/finance-dbtool.previous
```

4. Se o backup falhar, resolva antes de prosseguir. Copie-o para fora da VM como na parte F.
5. Substitua os binários, preservando ambiente e banco:

```sh
sudo install -o root -g root -m 0755 "$HOME/finance-bot-upload/finance-bot" /opt/finance-bot/finance-bot
sudo install -o root -g root -m 0755 "$HOME/finance-bot-upload/finance-dbtool" /opt/finance-bot/finance-dbtool
sudo systemctl start finance-bot.service
sudo systemctl status finance-bot.service --no-pager
sudo journalctl -u finance-bot.service -n 50 --no-pager
```

6. Migrações suportadas são aplicadas na abertura do banco, dentro de transação. Nesta versão só existe o schema 1; nenhuma migração manual é necessária. Um banco de versão maior é recusado. Leia as notas de futuras versões antes de atualizar.
7. Teste `/mes` e um registro controlado no Telegram. Se somente o binário falhar e o schema continuar compatível, pare o serviço e reinstale `.previous`. Se uma versão futura tiver alterado o schema, restaure o par **binário + backup compatível** seguindo F; não force downgrade.

## F. Backup, cópia externa e restauração

### Criar uma fotografia consistente

O banco usa WAL. **Não copie apenas `finance.db` enquanto o bot estiver ativo**, pois transações confirmadas podem estar em `finance.db-wal`. O utilitário incluído usa [SQLite VACUUM INTO](https://www.sqlite.org/lang_vacuum.html), que gera uma fotografia consistente; o arquivo resultante não depende dos arquivos WAL/SHM da origem.

Com o serviço em execução ou parado:

```sh
sudo -u finance-bot /opt/finance-bot/backup.sh
sudo ls -lh /var/lib/finance-bot/backups
```

O script usa nomes novos, verifica a integridade e não sobrescreve backups anteriores. Não é preciso instalar o CLI `sqlite3`. O processo exige espaço para outro banco e pode bloquear brevemente gravações. Não mova nem apague WAL/SHM de um banco em uso.

Alternativa explícita, usando um nome ainda inexistente:

```sh
sudo -u finance-bot /opt/finance-bot/finance-dbtool -action backup \
  -database /var/lib/finance-bot/finance.db \
  -out /var/lib/finance-bot/backups/finance-antes-atualizacao.db
```

### Copiar para fora da VM

Backups na mesma VM não protegem contra sua perda. Selecione o arquivo criado pelo script; substitua o nome abaixo pelo arquivo real. Na VM:

```sh
umask 077
mkdir -p "$HOME/finance-export"
sudo install -o "$(id -un)" -g "$(id -gn)" -m 0600 \
  /var/lib/finance-bot/backups/NOME_REAL_DO_BACKUP.db "$HOME/finance-export/finance-backup.db"
```

No computador local:

```sh
scp -i /caminho/da/chave.pem USUARIO_SSH@IP_DA_VM:~/finance-export/finance-backup.db ./finance-backup.db
```

Armazene em disco local criptografado ou outro destino privado que você já usa, com cópia adicional se necessário. SSH protege a transferência; o arquivo SQLite em si **não é criptografado**. Não publique backups, não os anexe a issues e não use pastas compartilhadas. Depois de conferir a cópia local, remova apenas o arquivo intermediário de exportação da VM:

```sh
rm -i "$HOME/finance-export/finance-backup.db"
```

Faça backup após registros importantes e antes de cada atualização. Guarde separadamente o token e a configuração em seu gerenciador de senhas. Não há agendamento ou sincronização externa automática nesta versão.

### Testar a restauração sem mexer na produção

Envie um backup para a conta SSH:

```sh
scp -i /caminho/da/chave.pem ./finance-backup.db USUARIO_SSH@IP_DA_VM:~/finance-restore-upload.db
```

Na VM, crie uma pasta temporária nova e verifique uma cópia:

```sh
RESTORE_TEST=$(sudo -u finance-bot mktemp -d /var/lib/finance-bot/restore-test.XXXXXX)
sudo install -o finance-bot -g finance-bot -m 0600 \
  "$HOME/finance-restore-upload.db" "$RESTORE_TEST/restored.db"
sudo -u finance-bot /opt/finance-bot/finance-dbtool -action check -database "$RESTORE_TEST/restored.db"
```

O verificador checa integridade, chaves estrangeiras, soma e quantidade de parcelas de cada compra. Os testes automatizados também reabrem backups e conferem totais de relatórios. Para uma conferência manual financeira, após a restauração real compare `/mes` com valores previamente anotados. **Não inicie um segundo processo com o token de produção** para testar uma cópia.

### Restaurar efetivamente

Só prossiga quando tiver escolhido explicitamente o backup correto. A restauração substitui o histórico pelo estado dessa fotografia; registros posteriores não estarão nela. Na VM:

```sh
sudo systemctl stop finance-bot.service
# Confirme que o processo encerrou antes de mover qualquer arquivo do banco:
sudo systemctl is-active finance-bot.service
# O resultado esperado é inactive; esse comando retorna status diferente de zero.
```

Se estiver `active` ou `deactivating`, não prossiga. Se já houver banco, faça um backup preventivo:

```sh
sudo -u finance-bot /opt/finance-bot/backup.sh
```

Se o banco estiver corrompido e o backup falhar, preserve os arquivos originais no passo seguinte para possível recuperação. Não os exclua. Crie um diretório **novo** para guardar o conjunto antigo e mova os arquivos somente com o serviço parado:

```sh
RESCUE=$(sudo -u finance-bot mktemp -d /var/lib/finance-bot/before-restore.XXXXXX)
for file in finance.db finance.db-wal finance.db-shm; do
  if sudo test -e "/var/lib/finance-bot/$file"; then
    sudo mv "/var/lib/finance-bot/$file" "$RESCUE/$file"
  fi
done
sudo install -o finance-bot -g finance-bot -m 0600 \
  "$HOME/finance-restore-upload.db" /var/lib/finance-bot/finance.db
sudo -u finance-bot /opt/finance-bot/finance-dbtool \
  -action check -database /var/lib/finance-bot/finance.db
```

Somente se a verificação terminar com sucesso:

```sh
sudo systemctl start finance-bot.service
sudo systemctl status finance-bot.service --no-pager
sudo journalctl -u finance-bot.service -n 50 --no-pager
```

Envie `/mes` e confira os totais esperados do backup, depois faça e desfaça uma pequena compra de teste. Preserve o diretório `before-restore.*` e a cópia externa até validar tudo. Reiniciar o serviço invalida fluxos incompletos, mas mantém compras salvas.

## G. Problemas comuns

| Sintoma | Verificações e solução |
| --- | --- |
| Token inválido | Confira o token do BotFather e espaços na configuração. `TELEGRAM_BOT_TOKEN` ausente ou formato inválido falha imediatamente; um formato plausível ainda é validado por `getMe`. Revogue token vazado. Não imprima o arquivo de ambiente em logs. |
| User ID incorreto | `/id` em conversa privada funciona mesmo sem autorização. Copie o número do próprio remetente, edite a configuração correta e reinicie; username e Chat ID de grupo não servem. |
| Bot não responde | Inicie a conversa, confira `/id`, ID permitido, token, `systemctl status`, logs e saída de rede. Registros incompletos expiram: use `/gasto` novamente. |
| Serviço não inicia | `sudo journalctl -u finance-bot -n 100 --no-pager`; confira ambiente obrigatório, caminhos, arquivo do serviço, executável, usuário e diretórios. Rode `sudo systemd-analyze verify /etc/systemd/system/finance-bot.service`. |
| Permissão negada no SQLite | Diretório e banco devem pertencer a `finance-bot`, com diretório 0700 e banco 0600; WAL/SHM exigem escrita no diretório. Confira `DATABASE_PATH` e `ReadWritePaths`. Não execute o bot como root para contornar. |
| `Exec format error` ou build de arquitetura errada | Compare `uname -m` com AMD64/ARM64. Use `file /opt/finance-bot/finance-bot` se `file` estiver instalado. Recompile com `GOOS=linux`, `CGO_ENABLED=0` e a arquitetura correta. Binário Windows não roda no Linux. |
| Não reinicia automaticamente | Confirme `systemctl is-enabled`, `Restart=on-failure` e `systemctl daemon-reload` depois de editar a unit. Aguarde 10s. Parada manual não reinicia. Confira falta de memória/disco e se a VM realmente está ligada. |
| Outra instância consome updates | Pare `go run`, processos locais e serviços antigos que usam o mesmo token. A API pode responder conflito 409; por proteção do token os logs da aplicação são genéricos. Mantenha somente uma instância. |
| SQLite não encontrado ou relatório vazio | Confira o caminho absoluto e o nome do arquivo em `DATABASE_PATH`, sem divulgar o token. Caminho novo cria banco novo no bot. Pare, localize o banco anterior e ajuste a configuração ou restaure backup. O dbtool recusa arquivo inexistente. |
| Falha na API Telegram | Confira horário do sistema, DNS com `getent hosts api.telegram.org`, certificados CA, saída TCP/443 e conectividade com `curl -I https://api.telegram.org`. Não coloque token no comando curl. Polling tenta novamente; falhas durante inicialização encerram e o systemd reinicia. |
| Botões antigos não funcionam | Podem ter expirado, sido usados, pertencer a fluxo reiniciado ou ter sido invalidados por reinício. Consulte `/mes` e use `/gasto` para começar outro lançamento. |
| Confirmação não chegou | A compra pode já estar salva. Consulte `/mes` ou o mês previsto de crédito antes de lançar novamente. `/desfazer` apresenta o último registro ativo. |
| Consulta de crédito difere do banco | Leia a convenção de fechamento abaixo; é uma projeção. Ajustar dias não muda compras históricas nem acompanha faturas pagas. |
| Backup falha | Confira espaço livre (`df -h /var/lib/finance-bot`), diretório privado e destino novo. O utilitário nunca deve receber o caminho da produção como destino. |
| SELinux no Oracle Linux impede execução | Preserve os logs de auditoria e confira contextos com `ls -Z`. Se houver labels incorretos após cópia, use `sudo restorecon -v /opt/finance-bot/finance-bot /opt/finance-bot/finance-dbtool`. Não desative SELinux. |

### Convenção do cartão, limites e segurança operacional

Sem dias: primeira parcela no mês seguinte, inclusive crédito 1x, sem vencimento específico. Com dias: fechamento estritamente posterior ao dia da compra, vencimento estritamente posterior ao fechamento. **No próprio dia de fechamento a compra vai ao próximo ciclo**. Dias iguais de fechamento/vencimento implicam vencimento no mês seguinte ao fechamento. Exemplo 20/10: compra em 19/01 → 10/02; compra em 20/01 → 10/03. Os dias configuráveis são 1–28.

Valores sempre em centavos inteiros, compra até R$ 1.000.000,00 e parcelas até 12. Menos centavos que parcelas é recusado. Períodos até 120 meses, incluindo meses zerados na média; `/parcelas` começa no mês seguinte. O fuso padrão é America/Sao_Paulo e está embutido no binário. Logs não contêm token nem valores das compras. O SQLite e os backups não têm criptografia própria.

O bot não usa HTTP de entrada nem exige alterações nas regras da Oracle. A configuração do serviço segue os recursos de restrição de processos descritos em [systemd.exec](https://www.freedesktop.org/software/systemd/man/latest/systemd.exec.html). Não há garantia de que um provedor manterá uma VM gratuita indefinidamente: a proteção dos dados depende de **backups fora da VM**.
