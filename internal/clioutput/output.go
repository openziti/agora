package clioutput

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/jedib0t/go-pretty/v6/table"
)

func RenderJSON(v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(os.Stdout, string(data))
	return err
}

func NewTable() table.Writer {
	t := table.NewWriter()
	t.SetOutputMirror(os.Stdout)
	t.SetStyle(table.StyleRounded)
	return t
}

func PrintTable(t table.Writer) {
	fmt.Println()
	t.Render()
	fmt.Println()
}

func PrintTotal(label string, count int) {
	fmt.Printf("total: %d %s\n\n", count, label)
}

func StringOrDash(v string) string {
	if v == "" {
		return "-"
	}
	return v
}

func TimeUTC(v time.Time) string {
	return v.UTC().Format("2006-01-02 15:04:05")
}
