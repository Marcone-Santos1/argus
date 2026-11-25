package install

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// ---------- themes made from a palette ----------
//
// Dark and light are hand-made files. The other sets are a palette each:
// the panel's colors, the Claude theme and the Fresh theme all come from it,
// so a new set is one entry below.

// Palette is a set's colors, "#rrggbb".
type Palette struct {
	Bg, Main, Card, Need     string // list, sessions/editor, cards, a card that waits for you
	Fg, Soft, Muted, Faint   string // text, from strongest to weakest
	Line, Line2              string // borders and selections
	Green, Amber, Red        string // done, waiting, error
	Claude, OC, Blue         string // accents: Claude's mark, opencode's, information
	Keyword, String, Comment string // code
	Function, Type, Constant string
	Operator, Variable       string
}

// Palettes are the generated sets, by name.
var Palettes = map[string]Palette{
	ThemeNight: { // purple and pink on blue-gray
		Bg: "#21222c", Main: "#282a36", Card: "#343746", Need: "#423432",
		Fg: "#f8f8f2", Soft: "#dcdce4", Muted: "#6272a4", Faint: "#4d5068",
		Line: "#343746", Line2: "#44475a",
		Green: "#50fa7b", Amber: "#ffb86c", Red: "#ff5555",
		Claude: "#ff79c6", OC: "#bd93f9", Blue: "#8be9fd",
		Keyword: "#ff79c6", String: "#f1fa8c", Comment: "#6272a4",
		Function: "#50fa7b", Type: "#8be9fd", Constant: "#bd93f9",
		Operator: "#ff79c6", Variable: "#f8f8f2",
	},
	ThemeSynthwave: { // neon pink and cyan on deep purple
		Bg: "#1a1023", Main: "#241b2f", Card: "#34294f", Need: "#43293a",
		Fg: "#f4eefa", Soft: "#d8cfe6", Muted: "#848bbd", Faint: "#4f4669",
		Line: "#2f2545", Line2: "#463864",
		Green: "#72f1b8", Amber: "#fede5d", Red: "#fe4450",
		Claude: "#ff7edb", OC: "#c77dff", Blue: "#36f9f6",
		Keyword: "#c77dff", String: "#ff8b39", Comment: "#848bbd",
		Function: "#36f9f6", Type: "#ff7edb", Constant: "#f97e72",
		Operator: "#fede5d", Variable: "#f4eefa",
	},
	ThemePink: { // pinks on near-black
		Bg: "#161517", Main: "#1e1d20", Card: "#2c2a2f", Need: "#3a2a2f",
		Fg: "#f2dbe6", Soft: "#d9bccb", Muted: "#8a7a83", Faint: "#4a4248",
		Line: "#2a282c", Line2: "#3d393f",
		Green: "#8be0a4", Amber: "#f6c177", Red: "#ff5c7a",
		Claude: "#ff6bb5", OC: "#f39ac7", Blue: "#7cc7ff",
		Keyword: "#ff6bb5", String: "#f8a5c2", Comment: "#e8e8e8",
		Function: "#f78fb3", Type: "#ff9ed2", Constant: "#ffb3d9",
		Operator: "#ff6bb5", Variable: "#f2dbe6",
	},
	ThemeVibrant: { // the classic warm code colors, saturated
		Bg: "#19181c", Main: "#222128", Card: "#2f2e37", Need: "#3b3226",
		Fg: "#f8f8f2", Soft: "#d6d6d0", Muted: "#8a8a80", Faint: "#4e4d55",
		Line: "#2c2b33", Line2: "#3e3d47",
		Green: "#a6e22e", Amber: "#fd971f", Red: "#f92672",
		Claude: "#f92672", OC: "#ae81ff", Blue: "#66d9ef",
		Keyword: "#66d9ef", String: "#e6db74", Comment: "#75715e",
		Function: "#a6e22e", Type: "#66d9ef", Constant: "#ae81ff",
		Operator: "#f92672", Variable: "#f8f8f2",
	},
	ThemeBordo: { // wine background, pink and teal
		Bg: "#272022", Main: "#322a2d", Card: "#413539", Need: "#4a3a2c",
		Fg: "#cbbec2", Soft: "#b3a5aa", Muted: "#8b747c", Faint: "#5c4c52",
		Line: "#3c3135", Line2: "#4e4045",
		Green: "#49e9a6", Amber: "#e4b781", Red: "#e34e1c",
		Claude: "#df769b", OC: "#7060eb", Blue: "#16a3b6",
		Keyword: "#df769b", String: "#49e9a6", Comment: "#8b747c",
		Function: "#16a3b6", Type: "#e4b781", Constant: "#7060eb",
		Operator: "#df769b", Variable: "#e66533",
	},
}

// rgb reads "#rrggbb".
func rgb(h string) [3]int {
	v, _ := strconv.ParseUint(strings.TrimPrefix(h, "#"), 16, 32)
	return [3]int{int(v >> 16), int(v >> 8 & 0xff), int(v & 0xff)}
}

func hex(c [3]int) string { return fmt.Sprintf("#%02x%02x%02x", c[0], c[1], c[2]) }

// Mix is a between b and c: t=0 is b, t=1 is c.
func Mix(b, c string, t float64) string {
	x, y := rgb(b), rgb(c)
	var o [3]int
	for i := range o {
		o[i] = int(float64(x[i]) + (float64(y[i])-float64(x[i]))*t + 0.5)
	}
	return hex(o)
}

// claudeFromPalette is a Claude Code theme file for a palette.
func claudeFromPalette(p Palette) string {
	lift := func(c string) string { return Mix(c, "#ffffff", 0.25) }
	o := map[string]string{
		"claude": p.Claude, "claudeShimmer": lift(p.Claude),
		"text": p.Fg, "inverseText": p.Main,
		"inactive": p.Muted, "inactiveShimmer": lift(p.Muted), "subtle": p.Faint,
		"suggestion": p.OC, "permission": p.OC, "permissionShimmer": lift(p.OC),
		"remember": p.Claude, "success": p.Green, "error": p.Red,
		"warning": p.Amber, "warningShimmer": lift(p.Amber), "merged": p.OC,
		"promptBorder": p.Line2, "promptBorderShimmer": lift(p.Line2),
		"planMode": p.Blue, "autoAccept": p.OC, "bashBorder": p.Amber, "ide": p.Blue,
		"fastMode": p.Amber, "fastModeShimmer": lift(p.Amber),
		"diffAdded": Mix(p.Main, p.Green, 0.18), "diffRemoved": Mix(p.Main, p.Red, 0.18),
		"diffAddedDimmed": Mix(p.Main, p.Green, 0.09), "diffRemovedDimmed": Mix(p.Main, p.Red, 0.09),
		"diffAddedWord": Mix(p.Main, p.Green, 0.38), "diffRemovedWord": Mix(p.Main, p.Red, 0.38),
		"userMessageBackground": p.Card, "userMessageBackgroundHover": Mix(p.Card, p.Fg, 0.06),
		"bashMessageBackgroundColor": Mix(p.Main, p.Card, 0.6), "memoryBackgroundColor": Mix(p.Main, p.Card, 0.6),
		"selectionBg": p.Line2, "composerSidebarBackground": p.Bg,
		"rate_limit_fill": p.Claude, "rate_limit_empty": p.Line2,
	}
	out, _ := json.MarshalIndent(map[string]any{"name": "Argus", "base": "dark", "overrides": o}, "", "  ")
	return string(out) + "\n"
}

// freshFromPalette is a Fresh theme for a palette: the dark file with its
// grays moved onto the palette's shades and its accents replaced.
func freshFromPalette(name string, p Palette) string {
	var t map[string]any
	if json.Unmarshal([]byte(freshThemeDark), &t) != nil {
		return ""
	}
	// The dark file's gray levels and where each lands.
	ramp := []struct {
		v int
		c string
	}{{0, p.Bg}, {15, p.Main}, {38, p.Card}, {55, p.Line2}, {80, p.Faint}, {125, p.Muted}, {180, p.Soft}, {214, p.Fg}, {255, p.Fg}}
	accents := map[[3]int]string{
		{217, 119, 87}: p.Claude, {74, 222, 128}: p.Green, {245, 177, 76}: p.Amber,
		{248, 113, 113}: p.Red, {124, 179, 255}: p.Blue, {154, 166, 255}: p.OC,
	}
	conv := func(v any) any {
		a, ok := v.([]any)
		if !ok || len(a) != 3 {
			return v
		}
		var c [3]int
		for i := range c {
			f, _ := a[i].(float64)
			c[i] = int(f)
		}
		out := ""
		if s, ok := accents[c]; ok {
			out = s
		} else if max(c[0], c[1], c[2])-min(c[0], c[1], c[2]) < 12 { // a gray
			g := (c[0] + c[1] + c[2]) / 3
			for i := 1; i < len(ramp); i++ {
				if g <= ramp[i].v {
					lo, hi := ramp[i-1], ramp[i]
					out = Mix(lo.c, hi.c, float64(g-lo.v)/float64(hi.v-lo.v))
					break
				}
			}
		}
		if out == "" {
			return v
		}
		return arr(out)
	}
	for _, sec := range t {
		if m, ok := sec.(map[string]any); ok {
			for k, v := range m {
				m[k] = conv(v)
			}
		}
	}
	t["name"] = name
	set := func(sec string, kv map[string]string) {
		m, _ := t[sec].(map[string]any)
		if m == nil {
			return
		}
		for k, c := range kv {
			m[k] = arr(c)
		}
	}
	set("editor", map[string]string{
		"bg": p.Main, "fg": p.Fg, "cursor": p.Claude, "bracket_match_fg": p.Claude,
		"line_number_bg": p.Main, "line_number_fg": p.Faint, "current_line_bg": Mix(p.Main, p.Card, 0.5),
		"selection_bg": p.Line2,
		"diff_add_bg":  Mix(p.Main, p.Green, 0.18), "diff_remove_bg": Mix(p.Main, p.Red, 0.18),
		"diff_add_highlight_bg": Mix(p.Main, p.Green, 0.35), "diff_remove_highlight_bg": Mix(p.Main, p.Red, 0.35),
		"diff_modify_bg": Mix(p.Main, p.Amber, 0.15),
	})
	rainbow := map[string]string{}
	for i, c := range []string{p.OC, p.Claude, p.Blue, p.Green, p.Amber, p.Function} {
		rainbow[fmt.Sprintf("indent_rainbow_%d", i+1)] = c
		rainbow[fmt.Sprintf("bracket_rainbow_%d", i+1)] = c
	}
	set("editor", rainbow)
	set("syntax", map[string]string{
		"keyword": p.Keyword, "string": p.String, "comment": p.Comment, "function": p.Function,
		"type": p.Type, "variable": p.Variable, "variable_builtin": p.Constant,
		"constant": p.Constant, "operator": p.Operator,
	})
	set("diagnostic", map[string]string{"error_fg": p.Red, "warning_fg": p.Amber, "info_fg": p.Blue, "hint_fg": p.Muted})
	set("ui", map[string]string{"tab_active_bg": p.Main, "tab_active_fg": p.Fg, "help_key_fg": p.Claude, "file_status_added_fg": p.Green})
	set("search", map[string]string{"current_match_bg": p.Amber, "current_match_fg": p.Main})
	out, _ := json.MarshalIndent(t, "", "  ")
	return string(out) + "\n"
}

func arr(h string) []int { c := rgb(h); return c[:] }
