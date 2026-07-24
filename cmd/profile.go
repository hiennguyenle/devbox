package cmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/hiennguyenle/devbox/internal/config"
	"github.com/hiennguyenle/devbox/internal/validate"
	"github.com/spf13/cobra"
)

var profileListFlag bool

var profileCmd = &cobra.Command{
	Use:     "profile [NAME]",
	Aliases: []string{"connect"},
	Short:   "Add or update a connection profile",
	Args:    cobra.MaximumNArgs(1),
	RunE:    runProfile,
}

func init() {
	profileCmd.Flags().BoolVar(&profileListFlag, "list", false, "list configured profiles")
	rootCmd.AddCommand(profileCmd)
}

func runProfile(cmd *cobra.Command, args []string) error {
	if profileListFlag {
		return listProfiles()
	}
	return addOrUpdateProfile(args)
}

func listProfiles() error {
	if !config.Exists() {
		return fmt.Errorf("no profiles configured. Run \"devbox profile\" first")
	}
	cfg, err := config.LoadDefault()
	if err != nil {
		return err
	}
	writeProfileList(os.Stdout, cfg)
	return nil
}

// writeProfileList renders the profile table. The default profile is marked
// with "*". Separated from listProfiles so it can be tested against a buffer.
func writeProfileList(out io.Writer, cfg *config.Config) {
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "  PROFILE\tHOST\tUSER\tPORT\tSESSION")
	for _, name := range cfg.ProfileNames() {
		p := cfg.Profiles[name]
		marker := "  "
		if name == cfg.DefaultProfile {
			marker = "* "
		}
		fmt.Fprintf(w, "%s%s\t%s\t%s\t%d\t%s\n", marker, name, p.Host, p.User, p.Port, p.DefaultSession)
	}
	w.Flush()
}

func addOrUpdateProfile(args []string) error {
	reader := bufio.NewReader(os.Stdin)

	cfg := &config.Config{Profiles: map[string]*config.Profile{}}
	if config.Exists() {
		loaded, err := config.LoadDefault()
		if err != nil {
			return err
		}
		cfg = loaded
	}
	if cfg.Profiles == nil {
		cfg.Profiles = map[string]*config.Profile{}
	}

	// Profile name (argument or prompt)
	name := ""
	if len(args) == 1 {
		name = args[0]
	}
	if name == "" {
		fmt.Print("Profile name: ")
		line, _ := reader.ReadString('\n')
		name = strings.TrimSpace(line)
	}
	if err := validate.ProfileName(name); err != nil {
		return err
	}

	existing := cfg.Profiles[name] // nil when adding a new profile

	// Host (required; pre-filled when editing)
	hostDefault := ""
	if existing != nil {
		hostDefault = existing.Host
	}
	host := promptField(reader, "SSH host", hostDefault)
	if host == "" {
		return fmt.Errorf("host is required")
	}
	if err := validate.Host(host); err != nil {
		return err
	}

	// User (defaults to the existing value, else the local username)
	userDefault := "root"
	if cur, err := user.Current(); err == nil {
		userDefault = cur.Username
	}
	if existing != nil && existing.User != "" {
		userDefault = existing.User
	}
	sshUser := promptField(reader, "SSH user", userDefault)
	if err := validate.User(sshUser); err != nil {
		return err
	}

	// Port
	portDefault := 22
	if existing != nil && existing.Port != 0 {
		portDefault = existing.Port
	}
	portStr := promptField(reader, "SSH port", strconv.Itoa(portDefault))
	port, err := strconv.Atoi(portStr)
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("invalid port: %s", portStr)
	}

	// Default session
	sessionDefault := "work"
	if existing != nil && existing.DefaultSession != "" {
		sessionDefault = existing.DefaultSession
	}
	session := promptField(reader, "Default session", sessionDefault)
	if err := validate.SessionName(session); err != nil {
		return err
	}

	// Test the connection; on failure, let the user save anyway.
	if !testConnection(sshUser, host, port) {
		fmt.Print("Save anyway? [y/N]: ")
		answer, _ := reader.ReadString('\n')
		answer = strings.TrimSpace(strings.ToLower(answer))
		if answer != "y" && answer != "yes" {
			return fmt.Errorf("aborted")
		}
	}

	cfg.Profiles[name] = &config.Profile{Host: host, User: sshUser, Port: port, DefaultSession: session}

	// First profile becomes default automatically; otherwise offer to switch.
	optIn := false
	if cfg.DefaultProfile != "" && cfg.DefaultProfile != name {
		fmt.Print("Set as default profile? [y/N]: ")
		answer, _ := reader.ReadString('\n')
		answer = strings.TrimSpace(strings.ToLower(answer))
		optIn = answer == "y" || answer == "yes"
	}
	cfg.DefaultProfile = nextDefaultProfile(cfg.DefaultProfile, name, optIn)

	if err := config.Save(cfg, config.Path()); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Printf("Profile %q saved to %s\n", name, config.Path())
	return nil
}

// nextDefaultProfile decides the default after adding `name`: the first profile
// always wins, and an existing default only changes when the user opts in.
func nextDefaultProfile(priorDefault, name string, optIn bool) string {
	if priorDefault == "" || (priorDefault != name && optIn) {
		return name
	}
	return priorDefault
}

// promptField prints "label [default]: " (or "label: " with no default), reads
// a line, and returns the entered value or the default when the input is blank.
func promptField(reader *bufio.Reader, label, def string) string {
	if def != "" {
		fmt.Printf("%s [%s]: ", label, def)
	} else {
		fmt.Printf("%s: ", label)
	}
	line, _ := reader.ReadString('\n')
	v := strings.TrimSpace(line)
	if v == "" {
		return def
	}
	return v
}

// testConnection runs a quick non-interactive SSH probe and reports success.
func testConnection(sshUser, host string, port int) bool {
	fmt.Printf("Testing connection to %s@%s:%d...\n", sshUser, host, port)
	cmd := exec.Command("ssh",
		"-o", "ConnectTimeout=5",
		"-o", "StrictHostKeyChecking=accept-new",
		"-p", strconv.Itoa(port),
		fmt.Sprintf("%s@%s", sshUser, host),
		"echo", "ok",
	)
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Connection failed: %v\n", err)
		return false
	}
	if strings.TrimSpace(string(out)) == "ok" {
		fmt.Println("Connection successful!")
	}
	return true
}
