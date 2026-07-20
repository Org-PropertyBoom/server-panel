package vhostfs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWrite_CreatesAtomicallyAndReportsChange(t *testing.T) {
	d := New(t.TempDir())
	changed, err := d.Write("example.com.caddy", "example.com {\n    reverse_proxy 127.0.0.1:8002\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Error("first write should report changed=true")
	}
	b, err := os.ReadFile(filepath.Join(d.Path(), "example.com.caddy"))
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "example.com {\n    reverse_proxy 127.0.0.1:8002\n}\n" {
		t.Errorf("contents = %q", b)
	}
}

func TestWrite_IdenticalContentIsNoOp(t *testing.T) {
	d := New(t.TempDir())
	body := "a.com {\n    redir https://b.com 301\n}\n"
	if _, err := d.Write("a.com.caddy", body); err != nil {
		t.Fatal(err)
	}
	changed, err := d.Write("a.com.caddy", body)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Error("re-writing identical bytes should report changed=false (no churn)")
	}
}

func TestWrite_OverwriteReportsChange(t *testing.T) {
	d := New(t.TempDir())
	if _, err := d.Write("a.com.caddy", "old"); err != nil {
		t.Fatal(err)
	}
	changed, err := d.Write("a.com.caddy", "new")
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Error("changed content should report changed=true")
	}
}

func TestWrite_LeavesNoTempFiles(t *testing.T) {
	d := New(t.TempDir())
	if _, err := d.Write("a.com.caddy", "x"); err != nil {
		t.Fatal(err)
	}
	names, err := d.ListNames()
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 1 || names[0] != "a.com.caddy" {
		t.Errorf("expected exactly a.com.caddy, got %v (temp file leaked?)", names)
	}
	entries, _ := os.ReadDir(d.Path())
	if len(entries) != 1 {
		t.Errorf("raw dir has %d entries, want 1", len(entries))
	}
}

func TestRemove_Idempotent(t *testing.T) {
	d := New(t.TempDir())
	if _, err := d.Write("a.com.caddy", "x"); err != nil {
		t.Fatal(err)
	}
	removed, err := d.Remove("a.com.caddy")
	if err != nil || !removed {
		t.Fatalf("first remove: removed=%v err=%v", removed, err)
	}
	removed, err = d.Remove("a.com.caddy")
	if err != nil {
		t.Fatalf("second remove errored: %v", err)
	}
	if removed {
		t.Error("removing a missing file should report removed=false, not error")
	}
}

func TestListNames_OnlyCaddyFilesSorted(t *testing.T) {
	dir := t.TempDir()
	d := New(dir)
	for _, n := range []string{"b.com.caddy", "a.com.caddy"} {
		if _, err := d.Write(n, "x"); err != nil {
			t.Fatal(err)
		}
	}
	// Noise that must be ignored: a non-.caddy file and a subdir.
	if err := os.WriteFile(filepath.Join(dir, "README.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "sub.caddy"), 0o755); err != nil {
		t.Fatal(err)
	}
	names, err := d.ListNames()
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 2 || names[0] != "a.com.caddy" || names[1] != "b.com.caddy" {
		t.Errorf("ListNames = %v, want [a.com.caddy b.com.caddy] sorted", names)
	}
}

func TestList_ReturnsContents(t *testing.T) {
	d := New(t.TempDir())
	if _, err := d.Write("a.com.caddy", "body-a"); err != nil {
		t.Fatal(err)
	}
	files, err := d.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0].Name != "a.com.caddy" || files[0].Contents != "body-a" {
		t.Errorf("List = %+v", files)
	}
}

func TestValidName_RejectsTraversal(t *testing.T) {
	d := New(t.TempDir())
	for _, bad := range []string{
		"../escape.caddy", "sub/a.caddy", `sub\a.caddy`, "a.txt", "", ".caddy" + string(rune(0)),
	} {
		if _, err := d.Write(bad, "x"); err == nil {
			t.Errorf("Write(%q) should be rejected", bad)
		}
		if _, err := d.Remove(bad); err == nil {
			t.Errorf("Remove(%q) should be rejected", bad)
		}
	}
}

func TestEnsure(t *testing.T) {
	dir := t.TempDir()
	if err := New(dir).Ensure(); err != nil {
		t.Errorf("Ensure on existing dir: %v", err)
	}
	if err := New(filepath.Join(dir, "nope")).Ensure(); err == nil {
		t.Error("Ensure on missing dir should error")
	}
	// A file where a dir is expected.
	f := filepath.Join(dir, "afile")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := New(f).Ensure(); err == nil {
		t.Error("Ensure on a non-dir path should error")
	}
}
