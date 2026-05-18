#!/usr/bin/env bash
# Lädt die aktuelle IEEE OUI-Datenbank herunter und schreibt sie nach SQLite.
# Benötigt: curl, python3 (kein sqlite3-CLI nötig)
set -euo pipefail

OUI_URL="https://standards-oui.ieee.org/oui/oui.csv"
DB="${1:-data/silentmap.db}"

echo "[update-oui] Lade OUI-Datenbank von IEEE..."
curl -fsSL "$OUI_URL" -o /tmp/oui.csv

echo "[update-oui] Schreibe in $DB ..."
mkdir -p "$(dirname "$DB")"

python3 - "$DB" <<'PYEOF'
import sys, csv, sqlite3

db_path = sys.argv[1]
con = sqlite3.connect(db_path)
cur = con.cursor()
cur.execute("""
    CREATE TABLE IF NOT EXISTS oui (
        prefix TEXT PRIMARY KEY,
        vendor TEXT NOT NULL
    )
""")

with open("/tmp/oui.csv", newline="", encoding="utf-8", errors="replace") as f:
    reader = csv.DictReader(f)
    rows = []
    for row in reader:
        assignment = row.get("Assignment", "").strip()
        vendor = row.get("Organization Name", "").strip().strip('"')
        if len(assignment) == 6:
            prefix = ":".join(assignment[i:i+2] for i in range(0, 6, 2)).upper()
            rows.append((prefix, vendor))

cur.executemany("INSERT OR REPLACE INTO oui (prefix, vendor) VALUES (?, ?)", rows)
con.commit()
print(f"[update-oui] {len(rows)} Einträge geschrieben.")
con.close()
PYEOF

rm /tmp/oui.csv
echo "[update-oui] Fertig."
