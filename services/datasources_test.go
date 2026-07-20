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
	created, _ := svc.Save(DataSource{Name: "x", Engine: "mysql", Host: "h", Port: "3306", Database: "d", User: "u"})
	if err := svc.Delete(created.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, ok := svc.Resolve(created.ID); ok {
		t.Fatalf("source still present after delete")
	}
	if err := svc.Delete("nope"); err != ErrDataSourceNotFound {
		t.Fatalf("expected ErrDataSourceNotFound, got %v", err)
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
