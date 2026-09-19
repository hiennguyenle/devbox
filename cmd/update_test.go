package cmd

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestUpdateCommandIsRegistered(t *testing.T) {
	command, _, err := rootCmd.Find([]string{"update"})
	if err != nil {
		t.Fatalf("find update command: %v", err)
	}
	if command == rootCmd {
		t.Fatal("update command is not registered")
	}
}

func TestUpdateSelfReplacesExecutableWithNewerRelease(t *testing.T) {
	target := writeExecutable(t, "old")
	server := updateServer(t, "v0.5.0", "new", false)

	updated, latest, err := updateSelf("v0.4.0", target, server.URL, server.Client())
	if err != nil {
		t.Fatalf("update self: %v", err)
	}
	if !updated || latest != "v0.5.0" {
		t.Fatalf("updated, latest = %v, %q; want true, v0.5.0", updated, latest)
	}
	if got, err := os.ReadFile(target); err != nil || string(got) != "new" {
		t.Fatalf("updated executable = %q, %v; want new", got, err)
	}
}

func TestUpdateSelfSkipsCurrentRelease(t *testing.T) {
	target := writeExecutable(t, "old")
	server := updateServer(t, "v0.5.0", "new", false)

	updated, latest, err := updateSelf("v0.5.0", target, server.URL, server.Client())
	if err != nil {
		t.Fatalf("update self: %v", err)
	}
	if updated || latest != "v0.5.0" {
		t.Fatalf("updated, latest = %v, %q; want false, v0.5.0", updated, latest)
	}
	if got, err := os.ReadFile(target); err != nil || string(got) != "old" {
		t.Fatalf("executable = %q, %v; want unchanged old", got, err)
	}
}

func TestUpdateSelfRejectsBadChecksumWithoutReplacingExecutable(t *testing.T) {
	target := writeExecutable(t, "old")
	server := updateServer(t, "v0.5.0", "new", true)

	if _, _, err := updateSelf("v0.4.0", target, server.URL, server.Client()); err == nil {
		t.Fatal("update self succeeded with a bad checksum")
	}
	if got, err := os.ReadFile(target); err != nil || string(got) != "old" {
		t.Fatalf("executable = %q, %v; want unchanged old", got, err)
	}
}

func TestUpdateSelfRejectsMalformedLatestVersion(t *testing.T) {
	target := writeExecutable(t, "old")
	server := updateServer(t, "not-a-version", "new", false)

	if _, _, err := updateSelf("v0.4.0", target, server.URL, server.Client()); err == nil {
		t.Fatal("update self accepted malformed latest version")
	}
}

func TestUpdateSelfRejectsUnreleasedBuild(t *testing.T) {
	target := writeExecutable(t, "old")

	if _, _, err := updateSelf("dev", target, "https://example.invalid/devbox", http.DefaultClient); err == nil {
		t.Fatal("update self accepted unreleased build")
	}
}

func writeExecutable(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "devbox")
	if err := os.WriteFile(path, []byte(contents), 0755); err != nil {
		t.Fatalf("write executable: %v", err)
	}
	return path
}

func updateServer(t *testing.T, version, binary string, badChecksum bool) *httptest.Server {
	t.Helper()
	archiveName := fmt.Sprintf("devbox_%s_%s_%s.tar.gz", version, runtime.GOOS, runtime.GOARCH)
	archive := archiveForTest(t, binary)
	sum := fmt.Sprintf("%x", sha256.Sum256(archive))
	if badChecksum {
		sum = "bad" + sum[3:]
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/latest/version.txt":
			fmt.Fprintln(w, version)
		case "/" + version + "/" + archiveName:
			w.Write(archive)
		case "/" + version + "/checksums.txt":
			fmt.Fprintf(w, "%s  %s\n", sum, archiveName)
		default:
			http.NotFound(w, r)
		}
	}))
}

func archiveForTest(t *testing.T, binary string) []byte {
	t.Helper()
	var archive bytes.Buffer
	gz := gzip.NewWriter(&archive)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: "devbox", Mode: 0755, Size: int64(len(binary))}); err != nil {
		t.Fatalf("tar header: %v", err)
	}
	if _, err := tw.Write([]byte(binary)); err != nil {
		t.Fatalf("tar body: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	return archive.Bytes()
}
