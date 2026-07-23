package commands

import "testing"

func TestPICommandsAreRegisteredAsLocalUI(t *testing.T) {
	r := CreateDefaultRegistry()
	for _, name := range []string{"fork", "export", "trust"} {
		cmd := r.Find(name)
		if cmd == nil || cmd.Type != TypeLocalUI {
			t.Fatalf("/%s = %#v, want local-ui command", name, cmd)
		}
	}
}
