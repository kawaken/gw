package output_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/kawaken/gw/output"
)

func TestPrint(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	output.Write(&buf, output.Result{
		Messages: []string{"hello", "world"},
		CD:       "/tmp/foo",
	})

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
