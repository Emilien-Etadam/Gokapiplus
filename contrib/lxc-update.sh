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
API_ROOT="https://api.github.com/repos/${REPO}/releases"
MIN_SIZE_BYTES=$((10 * 1024 * 1024))

die() {
    echo "Échec : $*" >&2
    exit 1
}

# Silent GitHub API call. The download below keeps the visible progress output.
github_get() {
    curl -fsSL --retry 3 --retry-delay 2 \
        -H "Accept: application/vnd.github+json" \
        -H "User-Agent: gokapi-lxc-update" \
        -- "$1"
}

# First JSON string value of a key. GitHub returns one release object here.
json_string() {
    local json=$1 key=$2
    local re="\"${key}\":[[:space:]]*\"([^\"]*)\""
    [[ $json =~ $re ]] || return 1
    printf '%s' "${BASH_REMATCH[1]}"
}

# "commit tag" lines. tag_name and target_commitish are one pair per release.
release_pairs() {
    local json=$1 rest=$1 matched
    local -a tags=() commits=()
    local tag_re='"tag_name":[[:space:]]*"([^"]*)"'
    local commit_re='"target_commitish":[[:space:]]*"([^"]*)"'
    while [[ $rest =~ $tag_re ]]; do
        tags+=("${BASH_REMATCH[1]}")
        matched=${BASH_REMATCH[0]}
        rest=${rest#*"$matched"}
    done
    rest=$json
    while [[ $rest =~ $commit_re ]]; do
        commits+=("${BASH_REMATCH[1]}")
        matched=${BASH_REMATCH[0]}
        rest=${rest#*"$matched"}
    done
    (( ${#tags[@]} > 0 && ${#tags[@]} == ${#commits[@]} )) || return 1
    local i
    for (( i = 0; i < ${#tags[@]}; i++ )); do
        printf '%s %s\n' "${commits[i]}" "${tags[i]}"
    done
}

# YYYYMMDDHHMM from a build-YYYYMMDD-HHMM tag. Fixed width, so numeric order is time order.
release_stamp() {
    local day=${1#build-}
    printf '%s%s' "${day%-*}" "${day#*-}"
}

# sha256 digest of the asset named exactly $2, or nothing when that asset has none.
asset_sha256() {
    local json=$1 want=$2
    local -i i=0 n=${#json} depth=0 in_str=0 esc=0 in_assets=0 asset_depth=0
    local c buf="" reading="" key="" cur_name="" cur_digest="" found=""

    while (( i < n )); do
        c=${json:i:1}
        i=$((i + 1))
        if (( in_str )); then
            if (( esc )); then
                buf+="$c"
                esc=0
            elif [[ $c == '\' ]]; then
                esc=1
            elif [[ $c == '"' ]]; then
                in_str=0
                if [[ $reading == key ]]; then
                    key=$buf
                elif [[ $reading == value && $in_assets -eq 1 && $depth -eq $asset_depth ]]; then
                    case $key in
                        name) cur_name=$buf ;;
                        digest) cur_digest=$buf ;;
                    esac
                fi
                reading=""
                buf=""
            else
                buf+="$c"
            fi
            continue
        fi
        case $c in
            '"')
                in_str=1
                buf=""
                if [[ $reading == colon ]]; then
                    reading=value
                else
                    reading=key
                fi
                ;;
            ':')
                reading=colon
                ;;
            '{')
                depth=$((depth + 1))
                if (( in_assets )) && (( depth == asset_depth )); then
                    cur_name=""
                    cur_digest=""
                fi
                reading=""
                ;;
            '}')
                if (( in_assets )) && (( depth == asset_depth )) && [[ $cur_name == "$want" && -z $found ]]; then
                    found=$cur_digest
                fi
                depth=$((depth - 1))
                reading=""
                ;;
            '[')
                if (( in_assets == 0 )) && [[ $key == assets ]] && (( depth == 1 )); then
                    in_assets=1
                    asset_depth=$((depth + 1))
                fi
                reading=""
                ;;
            ']')
                if (( in_assets )) && (( depth == asset_depth - 1 )); then
                    in_assets=0
                fi
                reading=""
                ;;
            [[:space:]])
                ;;
            *)
                [[ $reading == colon ]] && reading=""
                ;;
        esac
    done

    [[ $found =~ ^sha256:[0-9a-fA-F]{64}$ ]] || return 0
    printf '%s' "${found,,}"
}

[ "$(id -u)" -eq 0 ] || die "cette commande doit être lancée en root."

command -v systemctl >/dev/null 2>&1 || die "systemctl est introuvable, ce n'est pas une installation gérée par systemd."
command -v curl >/dev/null 2>&1 || die "curl est introuvable. Installez-le avec : apt install curl"

# The binary is read from the service definition, as installations made before the
# upstream rename still run gokapi-linux_amd64 rather than gokapi.
BIN=$(systemctl show -p ExecStart --value "$SERVICE" | grep -o 'path=[^ ;]*' | cut -d= -f2 || true)
[ -n "$BIN" ] || die "impossible de trouver le binaire du service ${SERVICE}."
[ -f "$BIN" ] || die "le binaire ${BIN} n'existe pas."

# Every published build prints the same version line ("Gokapi v2.2.5-dev"). The release
# number is the tag build-YYYYMMDD-HHMM, and --version prints the git commit that tag
# points at. Those two tags are the values compared below.
if ! version_text=$("$BIN" --version); then
    die "impossible de lire la version installée. Le serveur n'a pas été modifié."
fi

installed_version=""
installed_commit=""
while IFS= read -r line; do
    case $line in
        "Gokapi v"*) installed_version=${line#"Gokapi v"} ;;
        "Git Commit: "*) installed_commit=${line#"Git Commit: "} ;;
    esac
done <<< "$version_text"
[[ -n $installed_version && $installed_version != *" "* && $installed_commit =~ ^[0-9a-f]{40}$ ]] \
    || die "impossible de lire la version installée. Le serveur n'a pas été modifié."

latest_json=$(github_get "${API_ROOT}/latest") \
    || die "impossible de lire le numéro de la dernière version. Le serveur n'a pas été modifié."
latest_tag=$(json_string "$latest_json" tag_name) \
    || die "impossible de lire le numéro de la dernière version. Le serveur n'a pas été modifié."
latest_commit=$(json_string "$latest_json" target_commitish) \
    || die "impossible de lire le numéro de la dernière version. Le serveur n'a pas été modifié."
[[ $latest_tag =~ ^build-[0-9]{8}-[0-9]{4}$ && $latest_commit =~ ^[0-9a-f]{40}$ ]] \
    || die "impossible de lire le numéro de la dernière version. Le serveur n'a pas été modifié."

if [[ $installed_commit == "$latest_commit" ]]; then
    echo "Le serveur est déjà à jour, rien à faire."
    exit 0
fi

installed_tag=""
page=1
while (( page <= 20 )); do
    page_json=$(github_get "${API_ROOT}?per_page=100&page=${page}") \
        || die "impossible de lire le numéro de la dernière version. Le serveur n'a pas été modifié."
    [[ $page_json == "[]" ]] && break
    pairs=$(release_pairs "$page_json") \
        || die "impossible de lire le numéro de la dernière version. Le serveur n'a pas été modifié."
    pair_count=0
    while read -r commit tag; do
        [[ -n $commit ]] || continue
        pair_count=$((pair_count + 1))
        [[ $tag =~ ^build-[0-9]{8}-[0-9]{4}$ && $commit =~ ^[0-9a-f]{40}$ ]] \
            || die "impossible de lire le numéro de la dernière version. Le serveur n'a pas été modifié."
        if [[ $commit == "$installed_commit" ]]; then
            if [[ -z $installed_tag ]] || (( 10#$(release_stamp "$tag") > 10#$(release_stamp "$installed_tag") )); then
                installed_tag=$tag
            fi
        fi
    done <<< "$pairs"
    [[ $pair_count -ge 1 ]] || die "impossible de lire le numéro de la dernière version. Le serveur n'a pas été modifié."
    [[ $pair_count -lt 100 ]] && break
    page=$((page + 1))
done

[[ -n $installed_tag ]] \
    || die "la version installée ne correspond à aucune version publiée. Le serveur n'a pas été modifié."

if (( 10#$(release_stamp "$installed_tag") >= 10#$(release_stamp "$latest_tag") )); then
    echo "Le serveur est déjà à jour, rien à faire."
    exit 0
fi

expected_digest=$(asset_sha256 "$latest_json" "$ASSET")

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

if [[ -n $expected_digest ]]; then
    read -r downloaded_sum _ < <(sha256sum "$TEMPORARY") \
        || die "impossible de vérifier l'empreinte du fichier téléchargé. Le serveur n'a pas été touché."
    [[ $expected_digest == "sha256:${downloaded_sum}" ]] \
        || die "le fichier téléchargé ne correspond pas à l'empreinte publiée. Le serveur n'a pas été touché."
fi

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
