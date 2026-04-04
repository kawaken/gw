package git

import (
	"fmt"
	"strings"
)

// FakeRunner is a test double for Runner that returns canned responses.
type FakeRunner struct {
	// Responses maps "cmd arg1 arg2..." → output string.
	// RunIn uses the same map (dir is ignored).
	Responses map[string]string
	// Errors maps "cmd arg1 arg2..." → error.
	Errors map[string]error
}

// Run returns the canned response for the given args.
func (f *FakeRunner) Run(args ...string) (string, error) {
	key := strings.Join(args, " ")

	if err, ok := f.Errors[key]; ok {
		return "", err
	}

	if out, ok := f.Responses[key]; ok {
		return out, nil
	}

	return "", fmt.Errorf("fake runner: unexpected command %q", key)
}

// RunIn delegates to Run (dir is ignored in tests).
func (f *FakeRunner) RunIn(_ string, args ...string) (string, error) {
	return f.Run(args...)
}

// Toplevel returns the canned response for "rev-parse --show-toplevel".
func (f *FakeRunner) Toplevel() (string, error) {
	return f.Run("rev-parse", "--show-toplevel")
}
