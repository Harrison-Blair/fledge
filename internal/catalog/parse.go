package catalog

import "strings"

// piRow is one model row of the pi --list-models table.
type piRow struct {
	provider string
	model    string
}

// parsePiTable reads the provider and model columns of the pi --list-models
// table. Rows are accepted only after the table header so surrounding command
// output cannot be mistaken for models.
func parsePiTable(out string) []piRow {
	var rows []piRow
	headerSeen := false
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "provider" && fields[1] == "model" {
			headerSeen = true
			continue
		}
		if !headerSeen || len(fields) < 2 {
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
