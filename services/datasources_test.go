package services

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func newTestDataSources(t *testing.T) *DataSourceService {
	t.Helper()
	return &DataSourceService{path: filepath.Join(t.TempDir(), "datasources.json")}
}

func TestDataSourcesEmptyWhenMissing(t *testing.T) {
	svc := newTestDataSources(t)
	list, err := svc.List()
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected empty list, got %d", len(list))
	}
}

func TestDataSourcesCreateNeverLeaksPassword(t *testing.T) {
	svc := newTestDataSources(t)
	view, err := svc.Save(DataSource{
		Name: "propertyteam", Engine: "mysql", Host: "127.0.0.1", Port: "3306",
		Database: "propertyteam", User: "root", Password: "s3cr3t",
	})
	if err != nil {
		t.Fatalf("Save() error: %v", err)
	}
	if view.ID == "" {
		t.Fatalf("expected a generated id")
	}
	if !view.PasswordSet {
		t.Fatalf("expected passwordSet true")
	}

	list, err := svc.List()
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(list) != 1 || list[0].Name != "propertyteam" || !list[0].PasswordSet {
		t.Fatalf("unexpected list: %+v", list)
	}
	// The view type has no password field at all — assert the resolved (server-side)
	// source still holds the secret while the view does not.
	full, ok := svc.Resolve(view.ID)
	if !ok || full.Password != "s3cr3t" {
		t.Fatalf("stored password missing/incorrect: %+v ok=%v", full, ok)
	}
}

func TestDataSourcesBlankPasswordKeepsExisting(t *testing.T) {
	svc := newTestDataSources(t)
	created, err := svc.Save(DataSource{Name: "db", Engine: "mysql", Host: "h", Port: "3306", Database: "d", User: "u", Password: "keepme"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	// Update host with a blank password.
	if _, err := svc.Save(DataSource{ID: created.ID, Name: "db", Engine: "mysql", Host: "h2", Port: "3306", Database: "d", User: "u", Password: ""}); err != nil {
		t.Fatalf("update: %v", err)
	}
	full, ok := svc.Resolve(created.ID)
	if !ok {
		t.Fatalf("resolve after update failed")
	}
	if full.Password != "keepme" {
		t.Fatalf("blank password did not preserve secret: %q", full.Password)
	}
	if full.Host != "h2" {
		t.Fatalf("host not updated: %q", full.Host)
	}
}

func TestDataSourcesDuplicateNameRejected(t *testing.T) {
	svc := newTestDataSources(t)
	if _, err := svc.Save(DataSource{Name: "same", Engine: "mysql", Host: "h", Port: "3306", Database: "d", User: "u"}); err != nil {
		t.Fatalf("first: %v", err)
	}
	if _, err := svc.Save(DataSource{Name: "SAME", Engine: "mysql", Host: "h", Port: "3306", Database: "d", User: "u"}); err != ErrDuplicateName {
		t.Fatalf("expected ErrDuplicateName, got %v", err)
	}
}

func TestDataSourcesDelete(t *testing.T) {
	svc := newTestDataSources(t)
	first, _ := svc.Save(DataSource{Name: "x", Engine: "mysql", Host: "h", Port: "3306", Database: "d", User: "u"})
	// The first source is auto-active; deleting the ONLY source is blocked.
	if err := svc.Delete(first.ID); err != ErrCannotDeleteOnlyActive {
		t.Fatalf("deleting the only source should be blocked; got %v", err)
	}
	// Add a second, then deleting the non-active one succeeds.
	second, _ := svc.Save(DataSource{Name: "y", Engine: "mysql", Host: "h", Port: "3306", Database: "d", User: "u"})
	if err := svc.Delete(second.ID); err != nil {
		t.Fatalf("delete non-active: %v", err)
	}
	if _, ok := svc.Resolve(second.ID); ok {
		t.Fatalf("source still present after delete")
	}
	if err := svc.Delete("nope"); err != ErrDataSourceNotFound {
		t.Fatalf("expected ErrDataSourceNotFound, got %v", err)
	}
}

func TestDataSources_SingleActiveModel(t *testing.T) {
	svc := newTestDataSources(t)
	a, _ := svc.Save(DataSource{Name: "a", Engine: "mysql", Host: "h", Port: "3306", Database: "d", User: "u"})
	// First source is auto-active.
	if act, ok := svc.ActiveSource(); !ok || act.ID != a.ID {
		t.Fatalf("first source should be auto-active; got %+v ok=%v", act, ok)
	}
	b, _ := svc.Save(DataSource{Name: "b", Engine: "mysql", Host: "h", Port: "3306", Database: "d", User: "u"})
	// Second is NOT auto-active.
	if act, _ := svc.ActiveSource(); act.ID != a.ID {
		t.Fatalf("active should still be a; got %s", act.ID)
	}
	// Switching active is radio — exactly one active.
	if err := svc.SetActive(b.ID); err != nil {
		t.Fatal(err)
	}
	list, _ := svc.List()
	activeCount := 0
	for _, d := range list {
		if d.Active {
			activeCount++
		}
	}
	if activeCount != 1 {
		t.Fatalf("exactly one active expected, got %d", activeCount)
	}
	if act, _ := svc.ActiveSource(); act.ID != b.ID {
		t.Fatalf("active should be b after SetActive; got %s", act.ID)
	}
	// Deleting the active source (b) promotes another (a) — never zero active.
	if err := svc.Delete(b.ID); err != nil {
		t.Fatal(err)
	}
	if act, ok := svc.ActiveSource(); !ok || act.ID != a.ID {
		t.Fatalf("deleting active should promote a; got %+v ok=%v", act, ok)
	}
}

func TestDataSourcesUnknownEngineRejected(t *testing.T) {
	svc := newTestDataSources(t)
	if _, err := svc.Save(DataSource{Name: "x", Engine: "oracle", Host: "h", Port: "1521", Database: "d", User: "u"}); err == nil {
		t.Fatalf("expected error for unknown engine")
	}
}

func TestDataSourceTestNotFound(t *testing.T) {
	svc := newTestDataSources(t)
	res := svc.Test(context.Background(), "missing")
	if res.OK || res.Error == "" {
		t.Fatalf("expected not-found error, got %+v", res)
	}
}

func TestDBAdapterDSNs(t *testing.T) {
	ds := DataSource{Host: "127.0.0.1", Port: "3306", Database: "propertyteam", User: "root", Password: "p@ss word"}

	my, _ := adapterFor("mysql")
	if got := my.BuildDSN(ds); !strings.Contains(got, "@tcp(127.0.0.1:3306)/propertyteam") || !strings.Contains(got, "parseTime=true") {
		t.Fatalf("mysql DSN unexpected: %q", got)
	}

	pg, _ := adapterFor("postgresql") // alias
	if pg.Driver() != "postgres" {
		t.Fatalf("postgres alias not normalized")
	}
	if got := pg.BuildDSN(ds); !strings.HasPrefix(got, "postgres://root:") || !strings.Contains(got, "@127.0.0.1:3306/propertyteam") || !strings.Contains(got, "sslmode=disable") {
		t.Fatalf("postgres DSN unexpected: %q", got)
	}

	lite, ok := adapterFor("sqlite3") // alias
	if !ok || lite.Driver() != "sqlite3" {
		t.Fatalf("sqlite alias/driver wrong")
	}
	if got := lite.BuildDSN(DataSource{Database: "/var/data/app.db"}); got != "/var/data/app.db" {
		t.Fatalf("sqlite DSN should be the file path, got %q", got)
	}
}
