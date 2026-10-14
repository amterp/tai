package main

import (
	"testing"

	"github.com/amterp/tai/internal/profile"
)

// The failure summary lists steps by this name, so an unnamed step must still
// get one; it used to come out as "".
func TestPostInstallStepName(t *testing.T) {
	tests := []struct {
		step profile.PostInstallStep
		want string
	}{
		{profile.PostInstallStep{}, "post_install hook"},
		{profile.PostInstallStep{Name: "render"}, `"render"`},
		{profile.PostInstallStep{Name: "render", Bundle: "work:approval"}, `"render" from bundle "work:approval"`},
	}
	for _, tt := range tests {
		if got := postInstallStepName(tt.step); got != tt.want {
			t.Errorf("postInstallStepName(%+v) = %s, want %s", tt.step, got, tt.want)
		}
	}
}
