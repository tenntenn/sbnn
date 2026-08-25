package cmd

import (
	"testing"
)

// setHookFlags sets flags on hookCmd the way a command line would - marking
// them Changed, which is what cobra's flag groups look at - and puts every
// flag it touched back afterwards. hookCmd and the variables behind it are
// package state shared with every other test in this package.
func setHookFlags(t *testing.T, flags map[string]string) {
	t.Helper()
	f := hookCmd.Flags()
	for name := range flags {
		flag := f.Lookup(name)
		if flag == nil {
			t.Fatalf("hook has no --%s", name)
		}
		was, wasChanged := flag.Value.String(), flag.Changed
		t.Cleanup(func() {
			if err := flag.Value.Set(was); err != nil {
				t.Errorf("restore --%s: %v", flag.Name, err)
			}
			flag.Changed = wasChanged
		})
	}
	for name, value := range flags {
		if err := f.Set(name, value); err != nil {
			t.Fatalf("set --%s=%s: %v", name, value, err)
		}
	}
}

// "sbnn hook --clear --on-review '...'" reads as "replace what is
// registered", but runHook's switch has --clear first and returns there: the
// hooks went, the new one was dropped on the floor, and the only thing said
// was "removed 1 hook(s)". The user walks away believing a hook is
// registered when none is. The combination has to be refused.
func TestHookClearAndRegisterAreMutuallyExclusive(t *testing.T) {
	tests := map[string]struct {
		flags   map[string]string
		wantErr bool
	}{
		"clear with a command is refused": {
			flags:   map[string]string{"clear": "true", "on-review": "notify-send done"},
			wantErr: true,
		},
		"clear with a URL is refused": {
			flags:   map[string]string{"clear": "true", "on-review-url": "http://localhost:9000/reviews"},
			wantErr: true,
		},
		"clear on its own is fine": {
			flags: map[string]string{"clear": "true"},
		},
		"a command on its own is fine": {
			flags: map[string]string{"on-review": "notify-send done"},
		},
		"a URL on its own is fine": {
			flags: map[string]string{"on-review-url": "http://localhost:9000/reviews"},
		},
		// Registering both at once is one hook doing two things, which
		// AddHook has always supported. The exclusion must not catch it.
		"a command and a URL together stay allowed": {
			flags: map[string]string{"on-review": "notify-send done", "on-review-url": "http://localhost:9000/reviews"},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			setHookFlags(t, tt.flags)
			err := hookCmd.ValidateFlagGroups()
			if tt.wantErr && err == nil {
				t.Errorf("ValidateFlagGroups() = nil, want an error for %v", tt.flags)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("ValidateFlagGroups() = %v, want nil for %v", err, tt.flags)
			}
		})
	}
}

// Nothing else on the command may be dragged into the exclusion: --target,
// --port and --json say where and how, not what to do.
func TestHookFlagsThatAreNotActionsStayCombinable(t *testing.T) {
	setHookFlags(t, map[string]string{"clear": "true", "target": "api", "json": "true"})
	if err := hookCmd.ValidateFlagGroups(); err != nil {
		t.Errorf("--clear --target api --json: %v", err)
	}
}
