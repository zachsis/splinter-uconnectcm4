package dashboard

import (
	"fmt"
	"os"
)

// Theme holds 256-color SGR foreground codes for the dashboard's visual roles.
// An empty code means "no color" — the mono theme (and any color-disabled
// fallback) leaves every field empty, so output is byte-identical to plain text.
type Theme struct {
	Name   string
	Header string
	Label  string
	Value  string
	Warn   string
	Spark  string
	Dim    string
}

const reset = "\x1b[0m"

func fg256(n int) string { return fmt.Sprintf("\x1b[38;5;%dm", n) }

// monoTheme has all-empty codes (no color).
var monoTheme = Theme{Name: "mono"}

var themes = map[string]Theme{
	"matrix": {Name: "matrix", Header: fg256(46), Label: fg256(34), Value: fg256(48), Warn: fg256(196), Spark: fg256(40), Dim: fg256(238)},
	"amber":  {Name: "amber", Header: fg256(214), Label: fg256(136), Value: fg256(220), Warn: fg256(202), Spark: fg256(214), Dim: fg256(94)},
	"neon":   {Name: "neon", Header: fg256(51), Label: fg256(45), Value: fg256(201), Warn: fg256(197), Spark: fg256(51), Dim: fg256(240)},
	"mono":   monoTheme,
}

// themeOrder is the cycle order for the `t` hotkey.
var themeOrder = []string{"matrix", "amber", "neon", "mono"}

// ThemeNames returns the valid theme names (for --theme validation/help).
func ThemeNames() []string { return append([]string(nil), themeOrder...) }

// ValidTheme reports whether name is a known theme.
func ValidTheme(name string) bool { _, ok := themes[name]; return ok }

// ColorEnabled reports whether ANSI color should be used, honoring NO_COLOR and
// a non-color/dumb TERM.
func ColorEnabled() bool {
	if _, ok := os.LookupEnv("NO_COLOR"); ok {
		return false
	}
	switch os.Getenv("TERM") {
	case "", "dumb":
		return false
	}
	return true
}

// paint wraps s in a color code + reset, or returns s unchanged when the code is
// empty (mono / color disabled).
func paint(code, s string) string {
	if code == "" {
		return s
	}
	return code + s + reset
}
