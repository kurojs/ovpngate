package favstore

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAddAndGet(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.json")
	s := New(path)

	s.Add("1.2.3.4", "server-01", "US", "United States")
	if !s.IsFavorite("1.2.3.4") {
		t.Fatal("expected favorite to exist")
	}

	f := s.Get("1.2.3.4")
	if f == nil || f.HostName != "server-01" {
		t.Fatalf("unexpected favorite: %+v", f)
	}
}

func TestRemove(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.json")
	s := New(path)

	s.Add("1.2.3.4", "server-01", "US", "United States")
	s.Remove("1.2.3.4")
	if s.IsFavorite("1.2.3.4") {
		t.Fatal("expected favorite to be removed")
	}
}

func TestRename(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.json")
	s := New(path)

	s.Add("1.2.3.4", "server-01", "US", "United States")
	s.Rename("1.2.3.4", "my server")
	f := s.Get("1.2.3.4")
	if f == nil || f.Alias != "my server" {
		t.Fatalf("unexpected alias: %q", f.Alias)
	}
}

func TestPersist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.json")

	s1 := New(path)
	s1.Add("1.2.3.4", "server-01", "US", "United States")
	s1.Add("5.6.7.8", "server-02", "JP", "Japan")
	if err := s1.Save(); err != nil {
		t.Fatal(err)
	}

	s2 := New(path)
	if err := s2.Load(); err != nil {
		t.Fatal(err)
	}

	if !s2.IsFavorite("1.2.3.4") || !s2.IsFavorite("5.6.7.8") {
		t.Fatal("expected both favorites after load")
	}
	if s2.IsFavorite("9.9.9.9") {
		t.Fatal("expected non-existent favorite to be absent")
	}
}

func TestLoadNonexistent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nope.json")
	s := New(path)
	if err := s.Load(); err != nil {
		t.Fatal("loading non-existent file should not error")
	}
}

func TestDefaultPath(t *testing.T) {
	path, err := DefaultPath()
	if err != nil {
		t.Fatal(err)
	}
	if path == "" {
		t.Fatal("expected non-empty path")
	}
	dir := filepath.Dir(path)
	os.RemoveAll(dir)
}
