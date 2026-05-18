// Package scanner runs active nmap scans on demand and parses the results.
// Scans are triggered manually per device and require nmap to be installed.
package scanner

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"net"
	"os/exec"
	"strings"
)

// maxErrorOutput limits how many bytes of nmap stderr we include in error
// messages — enough context to diagnose a problem without log spam.
const maxErrorOutput = 200

// dangerousNmapFlags are flags that could read local files, write output
// files, or load scripts — none of which belong in a simple port scan.
var dangerousNmapFlags = []string{
	"--script", "-iL", "-iR",
	"-oA", "-oN", "-oX", "-oG", "-oS",
	"--resume", "--stylesheet",
}

// NmapResult holds parsed output from an nmap run.
type NmapResult struct {
	OsInfo string   // from "OS details:" or "Running:" line
	Ports  []string // open ports, e.g. ["22/tcp open ssh OpenSSH 8.4"]
	Raw    string
}

// Summary returns a short human-readable one-liner for the event log.
func (r *NmapResult) Summary() string {
	var parts []string
	if r.OsInfo != "" {
		parts = append(parts, "OS: "+r.OsInfo)
	}
	if len(r.Ports) > 0 {
		portSummary := make([]string, 0, len(r.Ports))
		for _, p := range r.Ports {
			fields := strings.Fields(p)
			if len(fields) >= 3 {
				portSummary = append(portSummary, fields[0]+" "+fields[2])
			}
		}
		parts = append(parts, strings.Join(portSummary, ", "))
	}
	if len(parts) == 0 {
		return "keine offenen Ports gefunden"
	}
	return strings.Join(parts, " | ")
}

// RunNmap executes nmap against ip using the provided args string.
// Returns an error if nmap is not installed, ip is invalid, or the scan fails.
func RunNmap(ctx context.Context, ip, args string) (*NmapResult, error) {
	if net.ParseIP(ip) == nil {
		return nil, fmt.Errorf("ungültige IP-Adresse: %q", ip)
	}
	if _, err := exec.LookPath("nmap"); err != nil {
		return nil, fmt.Errorf("nmap nicht installiert")
	}

	fields := strings.Fields(args)
	if err := validateNmapArgs(fields); err != nil {
		return nil, err
	}
	fields = append(fields, ip)

	cmd := exec.CommandContext(ctx, "nmap", fields...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	if err := cmd.Run(); err != nil {
		output := buf.String()
		if len(output) > maxErrorOutput {
			output = output[:maxErrorOutput]
		}
		return nil, fmt.Errorf("nmap: %w — %s", err, strings.TrimSpace(output))
	}

	result := &NmapResult{Raw: buf.String()}
	parseOutput(result)
	return result, nil
}

// validateNmapArgs rejects flags that could read files, write output, or load
// scripts. Note: exec.Command passes args directly to nmap (no shell), so
// shell metacharacters are not a concern — only nmap-level misuse is.
func validateNmapArgs(args []string) error {
	for _, arg := range args {
		for _, bad := range dangerousNmapFlags {
			if arg == bad || strings.HasPrefix(arg, bad+"=") {
				return fmt.Errorf("nmap-Flag nicht erlaubt: %q", arg)
			}
		}
		// Reject anything that looks like a file path.
		if strings.Contains(arg, "/") || strings.Contains(arg, "\\") {
			return fmt.Errorf("nmap-Flag enthält Pfad: %q", arg)
		}
	}
	return nil
}

func parseOutput(r *NmapResult) {
	inPortTable := false
	scanner := bufio.NewScanner(strings.NewReader(r.Raw))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if strings.HasPrefix(line, "OS details: ") {
			r.OsInfo = strings.TrimPrefix(line, "OS details: ")
		} else if r.OsInfo == "" && strings.HasPrefix(line, "Running: ") {
			r.OsInfo = strings.TrimPrefix(line, "Running: ")
		}

		if strings.HasPrefix(line, "PORT") && strings.Contains(line, "STATE") && strings.Contains(line, "SERVICE") {
			inPortTable = true
			continue
		}
		if inPortTable {
			if line == "" {
				inPortTable = false
				continue
			}
			fields := strings.Fields(line)
			if len(fields) >= 3 && (fields[1] == "open" || fields[1] == "filtered") {
				r.Ports = append(r.Ports, line)
			}
		}
	}
}
