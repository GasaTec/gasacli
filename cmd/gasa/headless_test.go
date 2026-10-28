package main

import "testing"

func TestIsHeadless(t *testing.T) {
	for _, tc := range []struct {
		name string
		tty  bool
		env  map[string]string
		want bool
	}{
		{name: "no tty", want: true},
		{name: "interactive", tty: true},
		{name: "CI true", tty: true, env: map[string]string{"CI": "true"}, want: true},
		{name: "CI false", tty: true, env: map[string]string{"CI": "false"}},
		{name: "SSH without display", tty: true, env: map[string]string{"SSH_TTY": "/dev/pts/1"}, want: true},
		{name: "SSH with display", tty: true, env: map[string]string{"SSH_TTY": "/dev/pts/1", "DISPLAY": ":0"}},
		{name: "SSH with wayland", tty: true, env: map[string]string{"SSH_TTY": "/dev/pts/1", "WAYLAND_DISPLAY": "wayland-0"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			getenv := func(key string) string { return tc.env[key] }
			if got := isHeadless(tc.tty, getenv); got != tc.want {
				t.Fatalf("isHeadless() = %v, want %v", got, tc.want)
			}
		})
	}
}
