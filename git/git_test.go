package git

import "testing"

func TestShortRef(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		ref  string
		want string
	}{
		{
			name: "heads ref keeps nested branch path",
			ref:  "refs/heads/feature/auth",
			want: "feature/auth",
		},
		{
			name: "non heads ref drops refs prefix",
			ref:  "refs/remotes/origin/main",
			want: "remotes/origin/main",
		},
		{
			name: "tag ref drops refs prefix",
			ref:  "refs/tags/v1.0.0",
			want: "tags/v1.0.0",
		},
		{
			name: "plain branch is unchanged",
			ref:  "main",
			want: "main",
		},
		{
			name: "bare refs becomes empty",
			ref:  "refs",
			want: "",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			got := shortRef(c.ref)
			if got != c.want {
				t.Errorf("shortRef(%q) = %q, want %q", c.ref, got, c.want)
			}
		})
	}
}
