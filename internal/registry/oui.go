package registry

import (
	"bufio"
	"database/sql"
	"os"
	"strings"
)

// ouiDB wraps vendor lookups from the SQLite OUI database or falls back to a
// small built-in table for the most common prefixes.
type ouiDB struct {
	db *sql.DB
}

func newOUIDB(db *sql.DB) *ouiDB {
	return &ouiDB{db: db}
}

// Lookup returns the vendor name for a MAC address prefix (first 3 bytes).
// Falls back to the built-in map when the OUI table is empty or missing.
func (o *ouiDB) Lookup(mac string) string {
	if len(mac) < 8 {
		return ""
	}
	// Normalize to XX:XX:XX
	prefix := strings.ToUpper(mac[:8])

	var vendor string
	err := o.db.QueryRow(`SELECT vendor FROM oui WHERE prefix = ? LIMIT 1`, prefix).Scan(&vendor)
	if err == nil {
		return vendor
	}

	// Built-in fallback for common prefixes
	return builtinOUI[prefix]
}

// builtinOUI covers the most common consumer device vendors.
// Updated occasionally; the full database comes from scripts/update-oui.sh.
var builtinOUI = map[string]string{
	"B8:27:EB": "Raspberry Pi Foundation",
	"DC:A6:32": "Raspberry Pi Foundation",
	"E4:5F:01": "Raspberry Pi Foundation",
	"00:17:88": "Philips Lighting",
	"EC:FA:BC": "Amazon Technologies",
	"F0:27:2D": "Amazon Technologies",
	"FC:65:DE": "Amazon Technologies",
	"18:74:2E": "Amazon Technologies",
	"44:65:0D": "Amazon Technologies",
	"00:1A:11": "Google",
	"54:60:09": "Google",
	"F4:F5:D8": "Google",
	"3C:5A:B4": "Google",
	"AC:67:5D": "Google",
	"94:EB:2C": "Apple",
	"F0:DB:F8": "Apple",
	"28:CF:E9": "Apple",
	"A4:C3:F0": "Apple",
	"BC:92:6B": "Apple",
	"7C:D1:C3": "Apple",
	"00:17:F2": "Apple",
	"00:26:08": "Apple",
	"3C:15:C2": "Apple",
	"AC:BC:32": "Apple",
	"18:65:90": "Apple",
	"00:50:F2": "Microsoft",
	"28:18:78": "Microsoft",
	"60:45:BD": "Microsoft",
	"00:17:FA": "Microsoft",
	"98:5F:D3": "Samsung Electronics",
	"78:AB:BB": "Samsung Electronics",
	"8C:71:F8": "Samsung Electronics",
	"CC:07:AB": "Samsung Electronics",
	"00:16:6C": "Synology",
	"00:11:32": "Synology",
	"BC:AE:C5": "Synology",
	"00:24:A8": "QNAP Systems",
	"24:5E:BE": "QNAP Systems",
	"00:08:9B": "QNAP Systems",
	"00:E0:4C": "Realtek",
	"00:13:10": "Linksys",
	"00:18:39": "Cisco-Linksys",
	"00:1C:10": "Cisco-Linksys",
	"C4:04:15": "Netgear",
	"A0:21:B7": "Netgear",
	"20:4E:7F": "Netgear",
	"C8:3A:35": "Tenda Technology",
	"00:1D:0F": "TP-Link",
	"50:3E:AA": "TP-Link",
	"10:FE:ED": "TP-Link",
	"B0:BE:76": "TP-Link",
	"00:0C:29": "VMware",
	"00:50:56": "VMware",
	"08:00:27": "VirtualBox",
}

// createOUITable ensures the oui table exists (will be populated by update-oui.sh).
func createOUITable(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS oui (
		prefix  TEXT PRIMARY KEY,
		vendor  TEXT NOT NULL
	)`)
	return err
}

// loadOUIFromFile loads OUI data from a plain text file (IEEE format).
// Used for testing or manual import; prefer scripts/update-oui.sh for production.
func loadOUIFromFile(db *sql.DB, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`INSERT OR REPLACE INTO oui(prefix, vendor) VALUES(?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.Contains(line, "(hex)") {
			continue
		}
		parts := strings.SplitN(line, "(hex)", 2)
		if len(parts) != 2 {
			continue
		}
		prefix := strings.TrimSpace(strings.ReplaceAll(parts[0], "-", ":"))
		vendor := strings.TrimSpace(parts[1])
		if _, err := stmt.Exec(prefix, vendor); err != nil {
			continue
		}
	}
	return tx.Commit()
}
