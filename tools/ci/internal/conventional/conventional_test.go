package conventional

import "testing"

func TestParseHeader_Boundaries(t *testing.T) {
	tests := []struct {
		name         string
		subject      string
		wantType     string
		wantBreaking bool
		wantOK       bool
	}{
		{name: "plain type", subject: "feat: add x", wantType: "feat", wantOK: true},
		{name: "scoped type", subject: "fix(api): repair x", wantType: "fix", wantOK: true},
		{name: "empty scope", subject: "feat(): add x", wantType: "feat", wantOK: true},
		{name: "scope with spaces", subject: "fix(api gateway): x", wantType: "fix", wantOK: true},
		{name: "breaking marker", subject: "feat!: add x", wantType: "feat", wantBreaking: true, wantOK: true},
		{name: "breaking marker after scope", subject: "feat(api)!: add x", wantType: "feat", wantBreaking: true, wantOK: true},
		{name: "breaking marker after empty scope", subject: "feat()!: add x", wantType: "feat", wantBreaking: true, wantOK: true},
		{name: "surrounding whitespace trimmed", subject: "  feat: add x  ", wantType: "feat", wantOK: true},
		{name: "tab after colon", subject: "feat:\tadd x", wantType: "feat", wantOK: true},
		{name: "two spaces after colon", subject: "feat:  add x", wantType: "feat", wantOK: true},
		{name: "no space after colon", subject: "feat:add x"},
		{name: "colon without description", subject: "feat:"},
		{name: "marker before scope", subject: "feat!(api): add x"},
		{name: "two scopes", subject: "feat(a)(b): add x"},
		{name: "stray closing parenthesis", subject: "feat(a)b): add x"},
		{name: "capitalized type", subject: "Feat: add x"},
		{name: "uppercase type", subject: "FEAT: add x"},
		{name: "type with digit", subject: "feat2: add x"},
		{name: "empty subject", subject: ""},
		{name: "whitespace subject", subject: "  \t "},
		{name: "colon only", subject: ":"},
		{name: "marker without type", subject: "!: add x"},
		{name: "fullwidth colon", subject: "feat： add x"},
		{name: "leading byte order mark", subject: "\ufeff" + "feat: add x"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			header, ok := ParseHeader(tc.subject)
			if ok != tc.wantOK {
				t.Fatalf("ParseHeader(%q) ok = %t, want %t", tc.subject, ok, tc.wantOK)
			}
			if header.Type != tc.wantType || header.Breaking != tc.wantBreaking {
				t.Errorf("ParseHeader(%q) = %+v, want type %q and breaking %t", tc.subject, header, tc.wantType, tc.wantBreaking)
			}
		})
	}
}

func FuzzParseHeader(f *testing.F) {
	for _, seed := range []string{"feat: x", "fix(a)!: y", "", "\x00", "feat(\n): x", "  chore: z  "} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, subject string) {
		header, ok := ParseHeader(subject)
		if !ok {
			if header != (Header{}) {
				t.Fatalf("ParseHeader(%q) returned %+v alongside ok false", subject, header)
			}
			return
		}
		if header.Type == "" {
			t.Fatalf("ParseHeader(%q) accepted an empty type", subject)
		}
	})
}
