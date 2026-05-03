package config

import (
	"strings"
	"testing"
)

func TestSanitizeName_English(t *testing.T) {
	cases := []struct{ in, want string }{
		{"OpenAI", "openai"},
		{"Hello World", "hello-world"},
		{"foo/bar", "foo-bar"},
		{"  spaced  out  ", "spaced-out"},
		{"already-hyphen", "already-hyphen"},
		{"GPT-4 Model", "gpt-4-model"},
	}
	for _, c := range cases {
		if got := sanitizeName(c.in); got != c.want {
			t.Errorf("sanitizeName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestSanitizeName_Chinese(t *testing.T) {
	cases := []struct{ in, want string }{
		{"OpenAI公司", "openai公司"},
		{"深度学习", "深度学习"},
		{"机器学习 / Machine Learning", "机器学习-machine-learning"},
		{"用户ID", "用户id"},
	}
	for _, c := range cases {
		if got := sanitizeName(c.in); got != c.want {
			t.Errorf("sanitizeName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestSanitizeName_PathUnsafeChars(t *testing.T) {
	// Reserved characters on Windows + path separators must collapse.
	in := `name:with*reserved?<chars>|here`
	got := sanitizeName(in)
	for _, bad := range []string{":", "*", "?", "<", ">", "|", "/", "\\"} {
		if strings.Contains(got, bad) {
			t.Errorf("sanitizeName(%q) = %q still contains %q", in, got, bad)
		}
	}
}

func TestSanitizeName_LengthCap(t *testing.T) {
	long := strings.Repeat("深", 200)
	got := sanitizeName(long)
	if r := []rune(got); len(r) > maxFilenameRunes {
		t.Errorf("expected at most %d runes, got %d", maxFilenameRunes, len(r))
	}
}

func TestSanitizeName_OtherScripts(t *testing.T) {
	cases := []struct{ in, want string }{
		{"café", "café"},
		{"Привет", "привет"},
		{"日本語テスト", "日本語テスト"},
	}
	for _, c := range cases {
		if got := sanitizeName(c.in); got != c.want {
			t.Errorf("sanitizeName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
