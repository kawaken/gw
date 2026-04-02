package git

import "strings"

// FakeRunner is a test double for Runner that returns canned responses.
type FakeRunner struct {
	// Responses maps "cmd arg1 arg2..." → output string.
	// RunIn uses the same map (dir is ignored).
	Responses map[string]string
	// Errors maps "cmd arg1 arg2..." → error.
	Errors map[string]error
}

func (f *FakeRunner) key(args []string) string {
	var b strings.Builder
	for i, a := range args {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(a)
	}
	return b.String()
}

// Run returns the canned response for the given args.
func (f *FakeRunner) Run(args ...string) (string, error) {
	k := f.key(args)
	if f.Errors != nil {
		if err, ok := f.Errors[k]; ok {
			return "", err
		}
	}
	return f.Responses[k], nil
}

// RunIn delegates to Run (dir is ignored in tests).
func (f *FakeRunner) RunIn(_ string, args ...string) (string, error) {
	return f.Run(args...)
}

// Toplevel returns the canned response for "rev-parse --show-toplevel".
func (f *FakeRunner) Toplevel() (string, error) {
	return f.Run("rev-parse", "--show-toplevel")
}
