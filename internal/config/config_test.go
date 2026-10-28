package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestHome(t *testing.T) {
	t.Setenv("GASACLI_HOME", "")
	base, err := os.UserConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	got, err := Home()
	if err != nil || got != filepath.Join(base, "gasacli") {
		t.Fatalf("Home() = %q, %v", got, err)
	}
	t.Setenv("GASACLI_HOME", t.TempDir())
	got, err = Home()
	if err != nil || got != os.Getenv("GASACLI_HOME") {
		t.Fatalf("override = %q, %v", got, err)
	}
}

func TestEnsureDefaultsAndIdempotence(t *testing.T) {
	home := filepath.Join(t.TempDir(), "gasacli")
	t.Setenv("GASACLI_HOME", home)
	if err := Ensure(); err != nil {
		t.Fatal(err)
	}
	got, err := Load()
	if err != nil || got != defaults() {
		t.Fatalf("defaults = %+v, %v", got, err)
	}
	for _, dir := range []string{"", "agents", "prompts", "skills", "plugins", "segundo-cerebro", "backups", "logs"} {
		info, err := os.Stat(filepath.Join(home, dir))
		if err != nil {
			t.Fatal(err)
		}
		if !info.IsDir() {
			t.Errorf("%s is not directory", dir)
		}
		if runtime.GOOS != "windows" && info.Mode().Perm() != 0700 {
			t.Errorf("%s mode %o", dir, info.Mode().Perm())
		}
	}
	path := filepath.Join(home, "config.yaml")
	custom := "# retained\nweb:\n  mode: manual\n  port: 4312\n  fallback_port: false\n"
	if err := os.WriteFile(path, []byte(custom), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Ensure(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != custom {
		t.Fatalf("config overwritten: %q, %v", data, err)
	}
	got, err = Load()
	if err != nil || got.Web.Mode != "manual" || got.Web.Port != 4312 || got.Web.FallbackPort || got.Web.Host != "127.0.0.1" {
		t.Fatalf("custom = %+v, %v", got, err)
	}
}

func TestLoadValidation(t *testing.T) {
	for _, tc := range []struct{ name, yaml, want string }{
		{"mode", "web: {mode: invalid}", "web.mode"},
		{"unknown_root", "future_key: yes", "future_key"},
		{"misspelled_mode", "web: {mdoe: manual}", "mdoe"},
		{"misspelled_fallback", "web: {fallback_prot: true}", "fallback_prot"},
		{"public_host", "web: {host: 0.0.0.0}", "web.host"},
		{"public_ipv6", "web: {host: '::'}", "web.host"},
		{"external_host", "web: {host: example.com}", "web.host"},
		{"empty_host", "web: {host: ''}", "web.host"},
		{"ipv4_loopback", "web: {host: 127.0.0.1}", ""},
		{"ipv6_loopback", "web: {host: '::1'}", ""},
		{"localhost", "web: {host: localhost}", ""},
		{"zero", "web: {port: 0}", "web.port"},
		{"negative", "web: {port: -1}", "web.port"},
		{"overflow", "web: {port: 65536}", "web.port"},
		{"syntax", "web: [", "decode config"},
		{"documents", "web: {}\n---\nweb: {}", "single YAML document"},
		{"always", "web: {mode: always, port: 65535}", ""},
		{"minimum", "web: {port: 1}", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("GASACLI_HOME", home)
			if err := os.WriteFile(filepath.Join(home, "config.yaml"), []byte(tc.yaml), 0600); err != nil {
				t.Fatal(err)
			}
			_, err := Load()
			if tc.want == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v; want %q", err, tc.want)
			}
		})
	}
}
