package services

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestUpdateRunningImage(t *testing.T) {
	if os.Getenv("UNITYPERF_TEST_UPDATE_CHILD") == "1" {
		fmt.Print("R")
		io.Copy(io.Discard, os.Stdin)
		return
	}
	if runtime.GOOS != "windows" {
		t.Skip("Windows executable replacement")
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	old, err := os.ReadFile(self)
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "unityperf.exe")
	if err = os.WriteFile(target, old, 0755); err != nil {
		t.Fatal(err)
	}
	child := exec.Command(target, "-test.run=^TestUpdateRunningImage$")
	child.Env = append(os.Environ(), "UNITYPERF_TEST_UPDATE_CHILD=1")
	input, _ := child.StdinPipe()
	out, _ := child.StdoutPipe()
	if err = child.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { input.Close(); child.Wait() }()
	if _, err = io.ReadFull(out, make([]byte, 1)); err != nil {
		t.Fatal(err)
	}
	next := append(bytes.Clone(old), 1)
	digest := fmt.Sprintf("%x", sha256.Sum256(next))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/main/update.json":
			fmt.Fprintf(w, `{"version":"0.2.0","sha256":"%s"}`, digest)
		case "/v0.2.0/bin/unityperf.exe":
			w.Write(next)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	if _, err = updateExecutable(srv.Client(), srv.URL+"/", target, "0.2.0"); err != nil {
		t.Fatal(err)
	}
	digest = fmt.Sprintf("%064d", 0)
	if _, err = updateExecutable(srv.Client(), srv.URL+"/", target, "0.1.0"); err == nil {
		t.Fatal("accepted digest mismatch")
	}
	got, _ := os.ReadFile(target)
	if !bytes.Equal(got, old) {
		t.Fatal("old image changed on failure")
	}
	digest = fmt.Sprintf("%x", sha256.Sum256(next))
	if _, err = updateExecutable(srv.Client(), srv.URL+"/", target, "0.1.0"); err != nil {
		t.Fatal(err)
	}
	got, _ = os.ReadFile(target)
	if !bytes.Equal(got, next) {
		t.Fatal("image not replaced")
	}
	if _, err = updateExecutable(srv.Client(), srv.URL+"/", target, "0.1.0"); err != nil {
		t.Fatal(err)
	}
}
