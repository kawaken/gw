// Package subcmd implements each gw subcommand.
package subcmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

// Fmt reads JSON from stdin and extracts the specified field.
// Usage: gw __fmt <field>.
// Supported fields: messages, cd.
func Fmt(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: gw __fmt <field>")
		return 1
	}
	field := args[0]

	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return 0 // silent: wrapper should not fail on read error
	}
	data = []byte(strings.TrimSpace(string(data)))
	if len(data) == 0 {
		return 0
	}

	var result map[string]json.RawMessage
	if err := json.Unmarshal(data, &result); err != nil {
		return 0 // invalid JSON: output nothing
	}

	raw, ok := result[field]
	if !ok {
		return 0
	}

	switch field {
	case "messages":
		var msgs []string
		if err := json.Unmarshal(raw, &msgs); err != nil {
			return 0
		}
		for _, m := range msgs {
			fmt.Println(m)
		}
	case "cd":
		var cd string
		if err := json.Unmarshal(raw, &cd); err != nil {
			return 0
		}
		if cd != "" {
			fmt.Print(cd) // no newline: shell uses $() which strips trailing newlines anyway
		}
	default:
		// Unknown field: output nothing
	}
	return 0
}
