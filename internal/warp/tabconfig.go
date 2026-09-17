package warp

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hiennguyenle/devbox/internal/config"
)

// Marker is the first line of every file devbox writes into Warp's
// tab_configs directory. Sync only ever overwrites or removes files that
// carry it, so hand-written configs are never touched.
const Marker = "# managed by devbox warp sync — edits will be overwritten"

const (
	ActionWrote   = "wrote"
	ActionRemoved = "removed"
	ActionSkipped = "skipped"
)

// Change is one thing Sync did (or refused to do) to one file.
type Change struct {
	Action string
	Path   string
}

const (
	filePrefix = "devbox_"
	fileSuffix = ".toml"
	localKey   = "local"
	localColor = "blue"
)

// profileColors are cycled in sorted profile order. Blue is reserved for
// local; black and white make poor tab markers.
var profileColors = []string{"green", "yellow", "magenta", "cyan", "red"}

// ColorFor returns the tab color for the i-th profile in sorted order.
func ColorFor(i int) string {
	return profileColors[i%len(profileColors)]
}

// RenderProfile renders the tab config for one profile. Profile names, hosts
// and session names are validated to a character set that needs no TOML
// escaping. The session parameter is filled in Warp's modal on open, since
// warp://tab_config URIs cannot carry parameters.
func RenderProfile(name string, p *config.Profile, color string) string {
	return fmt.Sprintf(`%s
name = "devbox: %s"
color = "%s"

[[panes]]
id = "main"
type = "terminal"
is_focused = true
commands = ["devbox resume {{session}} -p %s"]

[params.session]
description = "tmux session on %s"
default = "%s"
`, Marker, name, color, name, p.Host, p.DefaultSession)
}

// RenderLocal renders the tab config for the built-in local profile.
func RenderLocal() string {
	return fmt.Sprintf(`%s
name = "devbox: local"
color = "%s"

[[panes]]
id = "main"
type = "terminal"
is_focused = true
commands = ["devbox resume {{session}} --local"]

[params.session]
description = "tmux session on this machine"
default = "work"
`, Marker, localColor)
}

// Sync makes dir hold exactly one managed file per configured profile plus
// the local one. cfg may be nil (no config yet): then only the local file is
// wanted and every managed profile file is stale. Returns the changes in
// order: writes (profiles sorted, then local), then removals.
func Sync(dir string, cfg *config.Config) ([]Change, error) {
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return nil, fmt.Errorf("warp tab configs directory not found: %s", dir)
	}

	var order []string
	want := map[string]string{}
	if cfg != nil {
		for i, name := range cfg.ProfileNames() {
			order = append(order, name)
			want[name] = RenderProfile(name, cfg.Profiles[name], ColorFor(i))
		}
	}
	order = append(order, localKey)
	want[localKey] = RenderLocal()

	var changes []Change
	for _, key := range order {
		path := filepath.Join(dir, filePrefix+key+fileSuffix)
		if managed, exists := isManaged(path); exists && !managed {
			changes = append(changes, Change{ActionSkipped, path})
			continue
		}
		if err := os.WriteFile(path, []byte(want[key]), 0644); err != nil {
			return changes, err
		}
		changes = append(changes, Change{ActionWrote, path})
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return changes, err
	}
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() || !strings.HasPrefix(n, filePrefix) || !strings.HasSuffix(n, fileSuffix) {
			continue
		}
		key := strings.TrimSuffix(strings.TrimPrefix(n, filePrefix), fileSuffix)
		if _, wanted := want[key]; wanted {
			continue
		}
		path := filepath.Join(dir, n)
		if managed, _ := isManaged(path); !managed {
			continue
		}
		if err := os.Remove(path); err != nil {
			return changes, err
		}
		changes = append(changes, Change{ActionRemoved, path})
	}
	return changes, nil
}

// isManaged reports whether path exists and, if so, whether its first line
// is the Marker.
func isManaged(path string) (managed, exists bool) {
	f, err := os.Open(path)
	if err != nil {
		return false, false
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	if sc.Scan() {
		return sc.Text() == Marker, true
	}
	return false, true
}
