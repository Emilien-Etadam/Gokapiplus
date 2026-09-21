#!/usr/bin/env bash
#
# Updates a Gokapi server that was installed by the Proxmox VE helper script, using the
# binary published by this fork.
#
# It is meant to replace /usr/bin/update on the container. The update command of the
# helper script downloads the upstream release, which would silently overwrite this fork
# and undo every change it carries.
#
# Install:
#   curl -fL -o /usr/bin/update \
#     https://raw.githubusercontent.com/Emilien-Etadam/Gokapiplus/master/contrib/lxc-update.sh
#   chmod +x /usr/bin/update
#
# Then update the server by running: update

set -euo pipefail

REPO="Emilien-Etadam/Gokapiplus"
ASSET="gokapi-linux-amd64"
SERVICE="gokapi"
DOWNLOAD_URL="https://github.com/${REPO}/releases/latest/download/${ASSET}"
MIN_SIZE_BYTES=$((10 * 1024 * 1024))

die() {
    echo "Échec : $*" >&2
    exit 1
}

[ "$(id -u)" -eq 0 ] || die "cette commande doit être lancée en root."

command -v systemctl >/dev/null 2>&1 || die "systemctl est introuvable, ce n'est pas une installation gérée par systemd."
command -v curl >/dev/null 2>&1 || die "curl est introuvable. Installez-le avec : apt install curl"

# The binary is read from the service definition, as installations made before the
# upstream rename still run gokapi-linux_amd64 rather than gokapi.
BIN=$(systemctl show -p ExecStart --value "$SERVICE" | grep -o 'path=[^ ;]*' | cut -d= -f2 || true)
[ -n "$BIN" ] || die "impossible de trouver le binaire du service ${SERVICE}."
[ -f "$BIN" ] || die "le binaire ${BIN} n'existe pas."

TEMPORARY=$(mktemp /tmp/gokapi-update.XXXXXX)
trap 'rm -f "$TEMPORARY"' EXIT

echo "Téléchargement de la dernière version..."
curl -fL --retry 3 --retry-delay 2 -o "$TEMPORARY" "$DOWNLOAD_URL" \
    || die "le téléchargement a échoué. Le serveur n'a pas été touché."

# A truncated or redirected download would otherwise be installed as the server
SIZE=$(stat -c %s "$TEMPORARY")
[ "$SIZE" -ge "$MIN_SIZE_BYTES" ] || die "le fichier téléchargé fait ${SIZE} octets, c'est trop peu pour être le serveur."
[ "$(head -c 4 "$TEMPORARY" | od -An -tx1 | tr -d ' \n')" = "7f454c46" ] \
    || die "le fichier téléchargé n'est pas un programme Linux."

if cmp -s "$TEMPORARY" "$BIN"; then
    echo "Le serveur est déjà à jour, rien à faire."
    exit 0
fi

echo "Installation dans ${BIN}..."
cp -a "$BIN" "${BIN}.previous"
systemctl stop "$SERVICE"
cp "$TEMPORARY" "$BIN"
chmod +x "$BIN"
systemctl start "$SERVICE"

# A binary that cannot start would leave the service down until someone notices, so the
# previous one is put back automatically.
sleep 3
if ! systemctl is-active --quiet "$SERVICE"; then
    echo "Le nouveau serveur n'a pas démarré, retour à la version précédente." >&2
    systemctl stop "$SERVICE" || true
    cp "${BIN}.previous" "$BIN"
    systemctl start "$SERVICE"
    die "mise à jour annulée. Consultez les journaux avec : journalctl -u ${SERVICE} -n 50"
fi

echo "Mise à jour terminée, le service tourne."
echo "Version précédente conservée dans ${BIN}.previous"
echo "Pensez à recharger la page avec Ctrl+F5."
