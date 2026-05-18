#!/bin/bash
# Läuft auf dem Container-Host (172.31.3.160).
# Holt den aktuellen Source-Tarball von debdev-srv01, baut das Image neu,
# löscht das vorherige Image und räumt dangling layers auf.
#
# Einmalig Setup:
#   chmod +x update-image.sh
#   SSH-Key von container-host → debdev-srv01 einrichten (ssh-copy-id)
#
# Aufruf:
#   ./update-image.sh
#   ./update-image.sh --restart     # Stack nach Build automatisch neu starten

set -euo pipefail

# ── Konfiguration ────────────────────────────────────────────────────────────
SOURCE_HOST="fischerman@172.31.10.100"
SOURCE_TAR="/tmp/silentmap-src.tar.gz"
BUILD_DIR="/opt/silentmap-build"
IMAGE="silentmap:latest"
IMAGE_PREV="silentmap:previous"

# Optional: Portainer API für automatischen Stack-Restart
# PORTAINER_URL="http://localhost:9000"
# PORTAINER_TOKEN="ptr_xxx"
# PORTAINER_STACK_ID="1"
# ────────────────────────────────────────────────────────────────────────────

RESTART=false
[[ "${1:-}" == "--restart" ]] && RESTART=true

echo "╔═══════════════════════════════════════╗"
echo "║      silentmap image update           ║"
echo "╚═══════════════════════════════════════╝"

# 1. Tarball holen
echo ""
echo "→ Hole Source-Tarball von ${SOURCE_HOST}…"
mkdir -p "${BUILD_DIR}"
scp -q "${SOURCE_HOST}:${SOURCE_TAR}" "${BUILD_DIR}/silentmap-src.tar.gz"
echo "  ✓ $(du -sh "${BUILD_DIR}/silentmap-src.tar.gz" | cut -f1) empfangen"

# 2. Entpacken (alten src-Ordner ersetzen)
echo ""
echo "→ Entpacke Build-Kontext…"
rm -rf "${BUILD_DIR}/src"
mkdir -p "${BUILD_DIR}/src"
tar -xzf "${BUILD_DIR}/silentmap-src.tar.gz" -C "${BUILD_DIR}/src"
echo "  ✓ Entpackt nach ${BUILD_DIR}/src"

# 3. Altes Image als :previous merken (für Rollback)
if docker image inspect "${IMAGE}" &>/dev/null 2>&1; then
    echo ""
    echo "→ Markiere bisheriges Image als ${IMAGE_PREV}…"
    docker tag "${IMAGE}" "${IMAGE_PREV}"
fi

# 4. Neues Image bauen
echo ""
echo "→ Baue ${IMAGE}…"
docker build --quiet -t "${IMAGE}" "${BUILD_DIR}/src"
echo "  ✓ Image gebaut: $(docker image inspect "${IMAGE}" --format '{{.Id}}' | cut -c8-19)"

# 5. Vorheriges Image löschen
if docker image inspect "${IMAGE_PREV}" &>/dev/null 2>&1; then
    echo ""
    echo "→ Lösche vorheriges Image (${IMAGE_PREV})…"
    docker rmi "${IMAGE_PREV}" >/dev/null
    echo "  ✓ Gelöscht"
fi

# 6. Dangling layers wegräumen
echo ""
echo "→ Räume dangling layers auf…"
PRUNED=$(docker image prune -f --format '{{.SpaceReclaimed}}' 2>/dev/null || true)
echo "  ✓ Freigegeben: ${PRUNED:-0 B}"

# 7. Optional: Portainer Stack-Restart via API
if [[ "${RESTART}" == true ]]; then
    if [[ -z "${PORTAINER_TOKEN:-}" || -z "${PORTAINER_STACK_ID:-}" ]]; then
        echo ""
        echo "  ⚠ --restart gesetzt, aber PORTAINER_TOKEN / PORTAINER_STACK_ID nicht konfiguriert."
        echo "  → Stack manuell in Portainer neu starten."
    else
        echo ""
        echo "→ Starte Portainer Stack #${PORTAINER_STACK_ID} neu…"
        curl -sf -X POST \
            "${PORTAINER_URL}/api/stacks/${PORTAINER_STACK_ID}/redeploy" \
            -H "X-API-Key: ${PORTAINER_TOKEN}" \
            -H "Content-Type: application/json" \
            -d '{"pullImage":false}' >/dev/null
        echo "  ✓ Stack neu gestartet"
    fi
fi

echo ""
echo "╔═══════════════════════════════════════╗"
echo "║  ✓ Update abgeschlossen               ║"
echo "║    Image: ${IMAGE}"
echo "║    → Stack in Portainer neu starten   ║"
echo "╚═══════════════════════════════════════╝"
