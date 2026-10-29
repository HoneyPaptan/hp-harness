package skill

import "testing"

func TestParseFrontmatter(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		wantFM  bool
		wantKey map[string]string
		wantPre string
	}{
		{
			name:   "plain",
			in:     "---\nname: deploy\ndescription: Ship it\n---\n\nBody",
			wantFM: true, wantKey: map[string]string{"name": "deploy", "description": "Ship it"}, wantPre: "\nBody",
		},
		{
			name:   "quoted",
			in:     "---\ndescription: 'say hello'\n---\nrest",
			wantFM: true, wantKey: map[string]string{"description": "say hello"}, wantPre: "rest",
		},
		{
			name:   "folded",
			in:     "---\ndescription: >\n  line one\n  line two\n---\ntail",
			wantFM: true, wantKey: map[string]string{"description": "line one line two"}, wantPre: "tail",
		},
		{
			name:   "literal",
			in:     "---\ndescription: |\n  a\n  b\n---\n",
			wantFM: true, wantKey: map[string]string{"description": "a\nb"}, wantPre: "",
		},
		{
			name:   "none",
			in:     "no front\n",
			wantFM: false, wantPre: "no front\n",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, body, ok := parseFrontmatter(tc.in)
			if ok != tc.wantFM {
				t.Fatalf("ok %v", ok)
			}
			if tc.wantFM {
				for k, v := range tc.wantKey {
					if got[k] != v {
						t.Fatalf("key %s got %q want %q", k, got[k], v)
					}
				}
			}
			if body != tc.wantPre {
				t.Fatalf("body %q want %q", body, tc.wantPre)
			}
		})
	}
}
