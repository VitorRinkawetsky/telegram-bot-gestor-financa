#!/bin/sh
# Executar como finance-bot. O diretório de destino deve existir e ser privado.
set -eu
umask 077
database=${1:-/var/lib/finance-bot/finance.db}
directory=${2:-/var/lib/finance-bot/backups}
tool=${DBTOOL:-/opt/finance-bot/finance-dbtool}
if [ ! -d "$directory" ]; then
    echo 'Crie previamente o diretório privado de backups.' >&2
    exit 1
fi
destination="$directory/finance-$(date -u +%Y%m%dT%H%M%SZ)-$$.db"
"$tool" -action backup -database "$database" -out "$destination"
"$tool" -action check -database "$destination"
printf 'Backup verificado: %s\n' "$destination"
