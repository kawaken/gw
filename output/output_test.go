package output_test

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	"github.com/kawaken/gw/output"
)

func TestPrint(t *testing.T) {
	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	output.Print(output.Result{
		Messages: []string{"hello", "world"},
		CD:       "/tmp/foo",
	})

	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	os.Stdout = old

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatal(err)
	}

	var got map[string]json.RawMessage
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	var msgs []string
	if err := json.Unmarshal(got["messages"], &msgs); err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 2 || msgs[0] != "hello" || msgs[1] != "world" {
		t.Errorf("unexpected messages: %v", msgs)
	}

	var cd string
	if err := json.Unmarshal(got["cd"], &cd); err != nil {
		t.Fatal(err)
	}
	if cd != "/tmp/foo" {
		t.Errorf("unexpected cd: %q", cd)
	}
}
