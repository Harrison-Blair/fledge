package catalog

import "strings"

// piRow is one model row of the pi --list-models table.
type piRow struct {
	provider string
	model    string
}

// parsePiTable reads the provider and model columns of the pi --list-models
// table, skipping the header row and any line without both columns.
func parsePiTable(out string) []piRow {
	var rows []piRow
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[0] == "provider" {
			continue
		}
		rows = append(rows, piRow{provider: fields[0], model: fields[1]})
	}
	return rows
}

// parseLines returns the trimmed, non-empty lines of out.
func parseLines(out string) []string {
	var lines []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		lines = append(lines, line)
	}
	return lines
}
