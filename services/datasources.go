package services

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// A Data Source is a named, engine-agnostic database connection that features
// (the Caddy vhost engine, and anything later) consume BY NAME. The list is
// persisted server-side in a root-owned JSON file (0640); passwords live only in
// that file and are NEVER returned to the client (the client sees passwordSet).
const (
	datasourcesPathVar  = "PPT_DATASOURCES_PATH"
	defaultDatasources  = "/etc/ppt-server-panel/datasources.json"
	datasourceIDByteLen = 8
)

// ErrDataSourceNotFound is returned for an update/delete/test of an unknown id.
var ErrDataSourceNotFound = errors.New("data source not found")

// ErrDuplicateName is returned when a create/update would collide with another
// source's name (sources are consumed by name, so names are unique).
var ErrDuplicateName = errors.New("a data source with that name already exists")

// ErrCannotDeleteOnlyActive blocks deleting the single remaining source (it's the
// active one by definition). Add + activate a replacement first, then delete this.
var ErrCannotDeleteOnlyActive = errors.New("cannot delete the only data source — add and activate another first")

// DataSource is one saved connection, as persisted (Password included on disk
// only). Engine is one of the registered adapter keys (mysql|postgres|sqlite).
type DataSource struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Engine   string `json:"engine"`
	Host     string `json:"host"`
	Port     string `json:"port"`
	Database string `json:"database"`
	User     string `json:"user"`
	Password string `json:"password"`
	// Active marks the SINGLE source every feature reads. Exactly one active while
	// any source exists (radio semantics — set-active clears the others).
	Active bool `json:"active"`
}

// DataSourceView is the client-facing shape — identical to DataSource but with
// the password replaced by a boolean so the secret never leaves the server.
type DataSourceView struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Engine      string `json:"engine"`
	Host        string `json:"host"`
	Port        string `json:"port"`
	Database    string `json:"database"`
	User        string `json:"user"`
	PasswordSet bool   `json:"passwordSet"`
	Active      bool   `json:"active"`
}

// DataSourceTestResult is the truthful outcome of a per-source liveness Test
// (ping + the adapter's engine-agnostic ProbeQuery). Feature-specific schema
// checks (e.g. the vhost engine's three host tables) live in that feature, not here.
type DataSourceTestResult struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

func (d DataSource) view() DataSourceView {
	return DataSourceView{
		ID: d.ID, Name: d.Name, Engine: d.Engine, Host: d.Host, Port: d.Port,
		Database: d.Database, User: d.User, PasswordSet: d.Password != "", Active: d.Active,
	}
}

// DataSourceService persists and tests the data-source list. The mutex serializes
// read-modify-write of the JSON file.
type DataSourceService struct {
	path string
	mu   sync.Mutex
}

// NewDataSourceService points at the root-owned JSON store (overridable via
// PPT_DATASOURCES_PATH for tests/dev).
func NewDataSourceService() *DataSourceService {
	path := os.Getenv(datasourcesPathVar)
	if path == "" {
		path = defaultDatasources
	}
	return &DataSourceService{path: path}
}

// List returns every saved source as a client-safe view (no passwords).
func (s *DataSourceService) List() ([]DataSourceView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	list, err := s.loadNormalized()
	if err != nil {
		return nil, err
	}
	views := make([]DataSourceView, 0, len(list))
	for _, d := range list {
		views = append(views, d.view())
	}
	return views, nil
}

// Save creates (blank ID) or updates a source and returns its view. A blank
// password on update keeps the stored one, so the secret never round-trips
// through the client.
func (s *DataSourceService) Save(in DataSource) (DataSourceView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	in.Name = strings.TrimSpace(in.Name)
	in.Engine = normalizeEngine(in.Engine)
	in.Host = strings.TrimSpace(in.Host)
	in.Port = strings.TrimSpace(in.Port)
	in.Database = strings.TrimSpace(in.Database)
	in.User = strings.TrimSpace(in.User)

	if in.Name == "" {
		return DataSourceView{}, errors.New("name is required")
	}
	if _, ok := adapterFor(in.Engine); !ok {
		return DataSourceView{}, fmt.Errorf("unknown engine %q", in.Engine)
	}

	list, err := s.load()
	if err != nil {
		return DataSourceView{}, err
	}

	// Name must be unique across all OTHER sources.
	for _, d := range list {
		if d.ID != in.ID && strings.EqualFold(d.Name, in.Name) {
			return DataSourceView{}, ErrDuplicateName
		}
	}

	if in.ID == "" {
		in.ID = newDataSourceID()
		in.Active = len(list) == 0 // the first source is auto-active; never zero active
		list = append(list, in)
	} else {
		idx := indexByID(list, in.ID)
		if idx < 0 {
			return DataSourceView{}, ErrDataSourceNotFound
		}
		if in.Password == "" {
			in.Password = list[idx].Password // keep existing secret
		}
		in.Active = list[idx].Active // active is changed only via SetActive, never a plain Save
		list[idx] = in
	}

	if err := s.persist(list); err != nil {
		return DataSourceView{}, err
	}
	return in.view(), nil
}

// Delete removes a source by id. Deleting the ONLY source is blocked (it's the
// active one — add + activate a replacement first). Deleting the active source when
// others remain promotes another to active, so there is never zero active.
func (s *DataSourceService) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	list, err := s.load()
	if err != nil {
		return err
	}
	idx := indexByID(list, id)
	if idx < 0 {
		return ErrDataSourceNotFound
	}
	if len(list) == 1 {
		return ErrCannotDeleteOnlyActive
	}
	wasActive := list[idx].Active
	list = append(list[:idx], list[idx+1:]...)
	if wasActive && activeIndex(list) < 0 && len(list) > 0 {
		list[0].Active = true // promote another — never zero active while a source exists
	}
	return s.persist(list)
}

// SetActive makes id the single active source and clears the rest (radio).
func (s *DataSourceService) SetActive(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	list, err := s.load()
	if err != nil {
		return err
	}
	if indexByID(list, id) < 0 {
		return ErrDataSourceNotFound
	}
	for i := range list {
		list[i].Active = list[i].ID == id
	}
	return s.persist(list)
}

// ActiveSource returns the single active source (full, incl. password) for a
// feature to open a connection. Second return is false when no source exists.
func (s *DataSourceService) ActiveSource() (DataSource, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	list, err := s.loadNormalized()
	if err != nil || len(list) == 0 {
		return DataSource{}, false
	}
	if i := activeIndex(list); i >= 0 {
		return list[i], true
	}
	return DataSource{}, false
}

// Test opens the source via its adapter, pings, and runs the engine-agnostic
// ProbeQuery. Returns a truthful ok/error.
func (s *DataSourceService) Test(ctx context.Context, id string) DataSourceTestResult {
	s.mu.Lock()
	ds, ok := s.get(id)
	s.mu.Unlock()
	if !ok {
		return DataSourceTestResult{Error: "data source not found"}
	}

	adapter, ok := adapterFor(ds.Engine)
	if !ok {
		return DataSourceTestResult{Error: "unsupported engine: " + ds.Engine}
	}

	db, err := sql.Open(adapter.Driver(), adapter.BuildDSN(ds))
	if err != nil {
		return DataSourceTestResult{Error: friendlyDBError(err)}
	}
	defer db.Close()
	db.SetMaxOpenConns(2)

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		return DataSourceTestResult{Error: friendlyDBError(err)}
	}

	probeCtx, probeCancel := context.WithTimeout(ctx, 5*time.Second)
	defer probeCancel()
	if _, err := db.ExecContext(probeCtx, adapter.ProbeQuery()); err != nil {
		return DataSourceTestResult{Error: friendlyDBError(err)}
	}
	return DataSourceTestResult{OK: true}
}

// Resolve returns the full stored source (incl. password) for a feature that
// needs to open a connection — e.g. the vhost engine selecting its host-source by
// name. Not exposed over the client API.
func (s *DataSourceService) Resolve(id string) (DataSource, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.get(id)
}

// ResolveByName is the "consume by name" lookup features use.
func (s *DataSourceService) ResolveByName(name string) (DataSource, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	list, err := s.load()
	if err != nil {
		return DataSource{}, false
	}
	for _, d := range list {
		if strings.EqualFold(d.Name, name) {
			return d, true
		}
	}
	return DataSource{}, false
}

// DataSourceHealth is the ACTIVE source's live connection status — derived from a
// real ping, so "active + healthy" is visible without clicking Test.
type DataSourceHealth struct {
	SourceID    string `json:"sourceId,omitempty"`
	Name        string `json:"name,omitempty"`
	OK          bool   `json:"ok"`
	Error       string `json:"error,omitempty"`
	CheckedAtMs int64  `json:"checkedAtMs"`
}

// ActiveHealth pings the active source and returns its live status. Present=false
// when there is no active source to check.
func (s *DataSourceService) ActiveHealth(ctx context.Context) (DataSourceHealth, bool) {
	ds, ok := s.ActiveSource()
	if !ok {
		return DataSourceHealth{}, false
	}
	h := DataSourceHealth{SourceID: ds.ID, Name: ds.Name, CheckedAtMs: time.Now().UnixMilli()}
	adapter, ok := adapterFor(ds.Engine)
	if !ok {
		h.Error = "unsupported engine: " + ds.Engine
		return h, true
	}
	db, err := sql.Open(adapter.Driver(), adapter.BuildDSN(ds))
	if err != nil {
		h.Error = friendlyDBError(err)
		return h, true
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	pingCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		h.Error = friendlyDBError(err)
		return h, true
	}
	h.OK = true
	return h, true
}

// loadNormalized loads the list and guarantees exactly one active source when any
// exist (migrates legacy data with no active flag by promoting the first). Persists
// the promotion so the invariant is durable. Caller holds s.mu.
func (s *DataSourceService) loadNormalized() ([]DataSource, error) {
	list, err := s.load()
	if err != nil {
		return nil, err
	}
	if len(list) > 0 && activeIndex(list) < 0 {
		list[0].Active = true
		_ = s.persist(list)
	}
	return list, nil
}

func activeIndex(list []DataSource) int {
	for i := range list {
		if list[i].Active {
			return i
		}
	}
	return -1
}

func (s *DataSourceService) get(id string) (DataSource, bool) {
	list, err := s.load()
	if err != nil {
		return DataSource{}, false
	}
	if idx := indexByID(list, id); idx >= 0 {
		return list[idx], true
	}
	return DataSource{}, false
}

func (s *DataSourceService) load() ([]DataSource, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, nil
	}
	var list []DataSource
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, fmt.Errorf("parse data sources: %w", err)
	}
	return list, nil
}

func (s *DataSourceService) persist(list []DataSource) error {
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	return atomicWriteFile(s.path, append(data, '\n'), 0640)
}

func indexByID(list []DataSource, id string) int {
	for i := range list {
		if list[i].ID == id {
			return i
		}
	}
	return -1
}

func newDataSourceID() string {
	b := make([]byte, datasourceIDByteLen)
	if _, err := rand.Read(b); err != nil {
		return "ds" + fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

// atomicWriteFile writes data to a temp file in the same dir, then renames over
// the target so a concurrent reader never sees a partial file. Sets mode on the
// temp before the rename (root:root 0640, since the panel runs as root).
func atomicWriteFile(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".mthan-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }() // no-op after a successful rename

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
