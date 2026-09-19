package cmd

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const releaseBaseURL = "https://nexus.mijo.services/repository/public/devbox"

var releaseVersionPattern = regexp.MustCompile(`^v([0-9]+)\.([0-9]+)\.([0-9]+)(?:-([0-9A-Za-z.-]+))?$`)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Check for and install the latest devbox release",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		target, err := os.Executable()
		if err != nil {
			return fmt.Errorf("find devbox executable: %w", err)
		}
		target, err = filepath.EvalSymlinks(target)
		if err != nil {
			return fmt.Errorf("resolve devbox executable: %w", err)
		}
		updated, latest, err := updateSelf(version, target, releaseBaseURL, &http.Client{Timeout: 30 * time.Second})
		if err != nil {
			return err
		}
		if updated {
			fmt.Fprintf(cmd.OutOrStdout(), "Updated devbox %s to %s\n", version, latest)
			return nil
		}
		if compareReleaseVersions(version, latest) > 0 {
			fmt.Fprintf(cmd.OutOrStdout(), "devbox %s is newer than the latest published release (%s)\n", version, latest)
			return nil
		}
		fmt.Fprintf(cmd.OutOrStdout(), "devbox is already up to date (%s)\n", version)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(updateCmd)
}

func updateSelf(current, target, baseURL string, client *http.Client) (bool, string, error) {
	if !releaseVersionPattern.MatchString(current) {
		return false, "", fmt.Errorf("devbox update requires a released version; reinstall with: curl -fsSL %s/install.sh | bash", baseURL)
	}
	latestBytes, err := getReleaseFile(client, baseURL+"/latest/version.txt")
	if err != nil {
		return false, "", fmt.Errorf("check latest devbox release: %w", err)
	}
	latest := strings.TrimSpace(string(latestBytes))
	if !releaseVersionPattern.MatchString(latest) {
		return false, "", fmt.Errorf("invalid latest devbox release version: %q", latest)
	}
	if compareReleaseVersions(current, latest) >= 0 {
		return false, latest, nil
	}

	osName, arch, err := releasePlatform()
	if err != nil {
		return false, latest, err
	}
	archiveName := fmt.Sprintf("devbox_%s_%s_%s.tar.gz", latest, osName, arch)
	archive, err := getReleaseFile(client, fmt.Sprintf("%s/%s/%s", baseURL, latest, archiveName))
	if err != nil {
		return false, latest, fmt.Errorf("download devbox %s: %w", latest, err)
	}
	checksums, err := getReleaseFile(client, fmt.Sprintf("%s/%s/checksums.txt", baseURL, latest))
	if err != nil {
		return false, latest, fmt.Errorf("download checksum for devbox %s: %w", latest, err)
	}
	if err := verifyChecksum(archiveName, archive, checksums); err != nil {
		return false, latest, err
	}
	if err := replaceExecutable(target, archive); err != nil {
		return false, latest, err
	}
	return true, latest, nil
}

func getReleaseFile(client *http.Client, url string) ([]byte, error) {
	response, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected HTTP status %s", response.Status)
	}
	const maxReleaseFileSize = 128 << 20
	data, err := io.ReadAll(io.LimitReader(response.Body, maxReleaseFileSize+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxReleaseFileSize {
		return nil, fmt.Errorf("release file exceeds %d MiB", maxReleaseFileSize>>20)
	}
	return data, nil
}

func releasePlatform() (string, string, error) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		return "", "", fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}
	if runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
		return "", "", fmt.Errorf("unsupported architecture: %s", runtime.GOARCH)
	}
	return runtime.GOOS, runtime.GOARCH, nil
}

func verifyChecksum(archiveName string, archive, checksums []byte) error {
	for _, line := range strings.Split(string(checksums), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || strings.TrimPrefix(fields[1], "*") != archiveName {
			continue
		}
		expected, err := hex.DecodeString(fields[0])
		if err != nil || len(expected) != sha256.Size {
			return fmt.Errorf("invalid checksum for %s", archiveName)
		}
		actual := sha256.Sum256(archive)
		if subtle.ConstantTimeCompare(expected, actual[:]) != 1 {
			return fmt.Errorf("checksum verification failed for %s", archiveName)
		}
		return nil
	}
	return fmt.Errorf("checksum not found for %s", archiveName)
}

func replaceExecutable(target string, archive []byte) error {
	reader, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return fmt.Errorf("read release archive: %w", err)
	}
	defer reader.Close()

	entries := tar.NewReader(reader)
	for {
		header, err := entries.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read release archive: %w", err)
		}
		if header.Name != "devbox" || !header.FileInfo().Mode().IsRegular() {
			continue
		}
		info, err := os.Stat(target)
		if err != nil {
			return fmt.Errorf("stat devbox executable: %w", err)
		}
		temp, err := os.CreateTemp(filepath.Dir(target), ".devbox-update-*")
		if err != nil {
			return fmt.Errorf("create temporary executable: %w", err)
		}
		tempName := temp.Name()
		defer os.Remove(tempName)
		if _, err := io.Copy(temp, entries); err != nil {
			temp.Close()
			return fmt.Errorf("extract devbox executable: %w", err)
		}
		if err := temp.Chmod(info.Mode().Perm()); err != nil {
			temp.Close()
			return fmt.Errorf("set devbox executable permissions: %w", err)
		}
		if err := temp.Close(); err != nil {
			return fmt.Errorf("close temporary executable: %w", err)
		}
		if err := os.Rename(tempName, target); err != nil {
			return fmt.Errorf("install devbox update: %w", err)
		}
		return nil
	}
	return fmt.Errorf("release archive does not contain devbox")
}

func compareReleaseVersions(left, right string) int {
	leftParts := releaseVersionPattern.FindStringSubmatch(left)
	rightParts := releaseVersionPattern.FindStringSubmatch(right)
	for i := 1; i <= 3; i++ {
		leftNumber, _ := strconv.ParseUint(leftParts[i], 10, 64)
		rightNumber, _ := strconv.ParseUint(rightParts[i], 10, 64)
		if leftNumber < rightNumber {
			return -1
		}
		if leftNumber > rightNumber {
			return 1
		}
	}
	return comparePrerelease(leftParts[4], rightParts[4])
}

func comparePrerelease(left, right string) int {
	if left == right {
		return 0
	}
	if left == "" {
		return 1
	}
	if right == "" {
		return -1
	}
	leftParts, rightParts := strings.Split(left, "."), strings.Split(right, ".")
	for i := 0; i < len(leftParts) && i < len(rightParts); i++ {
		leftNumber, leftErr := strconv.ParseUint(leftParts[i], 10, 64)
		rightNumber, rightErr := strconv.ParseUint(rightParts[i], 10, 64)
		if leftErr == nil && rightErr == nil {
			if leftNumber < rightNumber {
				return -1
			}
			if leftNumber > rightNumber {
				return 1
			}
			continue
		}
		if leftErr == nil {
			return -1
		}
		if rightErr == nil {
			return 1
		}
		if leftParts[i] < rightParts[i] {
			return -1
		}
		if leftParts[i] > rightParts[i] {
			return 1
		}
	}
	if len(leftParts) < len(rightParts) {
		return -1
	}
	return 1
}
