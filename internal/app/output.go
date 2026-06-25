package app

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

type OutputOptions struct {
	Format        string
	JSONFlag      bool
	NoProgress    bool
	ConfigDir     string
	ExplicitTable bool
}

type profileOutput struct {
	Name          string      `json:"name"`
	Source        string      `json:"source"`
	UserID        string      `json:"userId"`
	Nickname      string      `json:"nickname"`
	AccountName   string      `json:"accountName"`
	ExpiresAt     int64       `json:"expiresAt"`
	ExpiresAtText string      `json:"expiresAtText"`
	HasRefresh    bool        `json:"hasRefreshToken"`
	ActiveDriveID string      `json:"activeDriveId"`
	Drives        []DriveInfo `json:"drives"`
}

type errorOutput struct {
	Error CLIError `json:"error"`
}

func writeOutput(w io.Writer, format string, value any) error {
	if format == "" {
		format = "json"
	}
	switch strings.ToLower(format) {
	case "json":
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(value)
	case "table":
		return writeTable(w, value)
	case "csv":
		return writeCSV(w, value)
	default:
		return usageError("unknown output format %q; use json, table, or csv", format)
	}
}

func writeTable(w io.Writer, value any) error {
	rows, cols, err := tableRows(value)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		_, err := fmt.Fprintln(w, "(empty)")
		return err
	}
	widths := make(map[string]int, len(cols))
	for _, col := range cols {
		widths[col] = len(col)
	}
	for _, row := range rows {
		for _, col := range cols {
			if l := len(row[col]); l > widths[col] {
				widths[col] = l
			}
		}
	}
	for i, col := range cols {
		if i > 0 {
			fmt.Fprint(w, "  ")
		}
		fmt.Fprintf(w, "%-*s", widths[col], col)
	}
	fmt.Fprintln(w)
	for i, col := range cols {
		if i > 0 {
			fmt.Fprint(w, "  ")
		}
		fmt.Fprint(w, strings.Repeat("-", widths[col]))
	}
	fmt.Fprintln(w)
	for _, row := range rows {
		for i, col := range cols {
			if i > 0 {
				fmt.Fprint(w, "  ")
			}
			fmt.Fprintf(w, "%-*s", widths[col], row[col])
		}
		fmt.Fprintln(w)
	}
	return nil
}

func writeCSV(w io.Writer, value any) error {
	rows, cols, err := tableRows(value)
	if err != nil {
		return err
	}
	cw := csv.NewWriter(w)
	if err := cw.Write(cols); err != nil {
		return err
	}
	for _, row := range rows {
		record := make([]string, len(cols))
		for i, col := range cols {
			record[i] = row[col]
		}
		if err := cw.Write(record); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

func tableRows(value any) ([]map[string]string, []string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, nil, err
	}
	var arr []map[string]any
	if err := json.Unmarshal(raw, &arr); err != nil {
		var obj map[string]any
		if err2 := json.Unmarshal(raw, &obj); err2 != nil {
			return nil, nil, err
		}
		arr = []map[string]any{obj}
	}
	colsMap := map[string]bool{}
	rows := make([]map[string]string, 0, len(arr))
	for _, obj := range arr {
		row := map[string]string{}
		flatten("", obj, row)
		for key := range row {
			colsMap[key] = true
		}
		rows = append(rows, row)
	}
	cols := make([]string, 0, len(colsMap))
	for col := range colsMap {
		cols = append(cols, col)
	}
	sort.Strings(cols)
	return rows, cols, nil
}

func flatten(prefix string, value any, row map[string]string) {
	switch v := value.(type) {
	case map[string]any:
		for key, child := range v {
			name := key
			if prefix != "" {
				name = prefix + "." + key
			}
			flatten(name, child, row)
		}
	case []any:
		b, _ := json.Marshal(v)
		row[prefix] = string(b)
	case nil:
		row[prefix] = ""
	default:
		row[prefix] = fmt.Sprint(v)
	}
}
