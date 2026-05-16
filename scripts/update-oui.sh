#!/usr/bin/env bash
# Lädt die aktuelle IEEE OUI-Datenbank herunter und konvertiert sie für silentmap.
set -euo pipefail

OUI_URL="https://standards-oui.ieee.org/oui/oui.csv"
OUTPUT="${1:-data/oui.db}"

echo "[update-oui] Lade OUI-Datenbank von IEEE..."
curl -fsSL "$OUI_URL" -o /tmp/oui.csv

echo "[update-oui] Konvertiere nach SQLite: $OUTPUT"
mkdir -p "$(dirname "$OUTPUT")"

sqlite3 "$OUTPUT" <<SQL
DROP TABLE IF EXISTS oui;
CREATE TABLE oui (
    prefix TEXT PRIMARY KEY,
    vendor TEXT NOT NULL
);
SQL

# CSV einlesen (Format: Registry,Assignment,Organization Name,Organization Address)
tail -n +2 /tmp/oui.csv | while IFS=',' read -r _registry assignment organization _address; do
    # Assignment von AABBCC zu AA:BB:CC normalisieren
    prefix=$(echo "$assignment" | sed 's/../&:/g;s/:$//')
    vendor=$(echo "$organization" | tr -d '"')
    sqlite3 "$OUTPUT" "INSERT OR REPLACE INTO oui VALUES('${prefix}', '${vendor//\'/\'\'}');"
done

COUNT=$(sqlite3 "$OUTPUT" "SELECT COUNT(*) FROM oui;")
echo "[update-oui] Fertig: $COUNT Einträge in $OUTPUT"
rm /tmp/oui.csv
