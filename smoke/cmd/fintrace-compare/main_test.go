package main

import (
	"regexp"
	"testing"
)

// Core's name rule (§4.13.2), as its 400 states it
var coreName = regexp.MustCompile(`^[\p{L}\p{N}][\p{L}\p{N}\p{M} \-_\[\]()',’&#@№]*$`)

func TestWorkspaceNameIsOneCoreAccepts(t *testing.T) {
	cases := map[string]string{
		"2026-11-19_21-46.mok":  "compare 2026-11-19_21-46",
		"dump.v2.mok":           "compare dump-v2",
		"test dump S":           "compare test dump S",
		"Мои деньги backup.mok": "compare Мои деньги backup",
	}
	for label, want := range cases {
		got := workspaceName(label)
		if got != want || !coreName.MatchString(got) {
			t.Errorf("workspaceName(%q) = %q, want %q and a name Core accepts", label, got, want)
		}
	}
}
