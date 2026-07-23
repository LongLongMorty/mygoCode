package main

import "testing"

func TestHandleProjectTrustFlagRejectsNormalArguments(t *testing.T) {
	for _, args := range [][]string{nil, {"--remote"}, {"--trust-project", "extra"}} {
		if handleProjectTrustFlag(args) {
			t.Fatalf("handleProjectTrustFlag(%q) unexpectedly handled arguments", args)
		}
	}
}
