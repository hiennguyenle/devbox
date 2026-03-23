package cmd

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"strings"

	"github.com/hien/devbox/internal/config"
	"github.com/spf13/cobra"
)

var connectCmd = &cobra.Command{
	Use:   "connect",
	Short: "Configure SSH host for devbox",
	RunE:  runConnect,
}

func init() {
	rootCmd.AddCommand(connectCmd)
}

func runConnect(cmd *cobra.Command, args []string) error {
	reader := bufio.NewReader(os.Stdin)

	// Host (required)
	fmt.Print("SSH host: ")
	host, _ := reader.ReadString('\n')
	host = strings.TrimSpace(host)
	if host == "" {
		return fmt.Errorf("host is required")
	}

	// User (optional)
	currentUser, _ := user.Current()
	defaultUser := currentUser.Username
	fmt.Printf("SSH user [%s]: ", defaultUser)
	sshUser, _ := reader.ReadString('\n')
	sshUser = strings.TrimSpace(sshUser)
	if sshUser == "" {
		sshUser = defaultUser
	}

	// Port (optional)
	fmt.Print("SSH port [22]: ")
	portStr, _ := reader.ReadString('\n')
	portStr = strings.TrimSpace(portStr)
	port := 22
	if portStr != "" {
		p, err := strconv.Atoi(portStr)
		if err != nil || p < 1 || p > 65535 {
			return fmt.Errorf("invalid port: %s", portStr)
		}
		port = p
	}

	// Test connection
	fmt.Printf("Testing connection to %s@%s:%d...\n", sshUser, host, port)
	testCmd := exec.Command("ssh",
		"-o", "ConnectTimeout=5",
		"-o", "StrictHostKeyChecking=accept-new",
		"-p", fmt.Sprintf("%d", port),
		fmt.Sprintf("%s@%s", sshUser, host),
		"echo", "ok",
	)
	testCmd.Stderr = os.Stderr
	out, err := testCmd.Output()

	if err != nil {
		fmt.Fprintf(os.Stderr, "Connection failed: %v\n", err)
		fmt.Print("Save anyway? [y/N]: ")
		answer, _ := reader.ReadString('\n')
		answer = strings.TrimSpace(strings.ToLower(answer))
		if answer != "y" && answer != "yes" {
			return fmt.Errorf("aborted")
		}
	} else {
		if strings.TrimSpace(string(out)) == "ok" {
			fmt.Println("Connection successful!")
		}
	}

	cfg := &config.Config{
		Host:           host,
		User:           sshUser,
		Port:           port,
		DefaultSession: "work",
	}

	if err := config.Save(cfg, config.Path()); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Printf("Config saved to %s\n", config.Path())
	return nil
}
