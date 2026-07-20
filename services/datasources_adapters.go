package services

import (
	"errors"
	"net"
	"net/url"
	"strings"

	"github.com/go-sql-driver/mysql"
)

// DBAdapter maps an engine-agnostic DataSource onto a concrete database/sql
// driver: the driver name to open, the DSN to build, and an engine-agnostic
// liveness probe. Adding an engine later = one new adapter registered below;
// nothing else changes.
type DBAdapter interface {
	Driver() string                // database/sql driver name, e.g. "mysql"
	BuildDSN(ds DataSource) string // driver-specific connection string
	ProbeQuery() string            // engine-agnostic liveness check, e.g. "SELECT 1"
}

// dbAdapters is the engine -> adapter registry. mysql and sqlite drivers are
// compiled in (go-sql-driver/mysql, go-sqlite3). postgres is registered so the
// UI/model support it, but the lib/pq driver is not imported yet — a Test of a
// postgres source returns a clear "driver not built in" message until it is added.
var dbAdapters = map[string]DBAdapter{
	"mysql":    mysqlAdapter{},
	"postgres": postgresAdapter{},
	"sqlite":   sqliteAdapter{},
}

// adapterFor returns the adapter for a (normalized) engine name.
func adapterFor(engine string) (DBAdapter, bool) {
	a, ok := dbAdapters[normalizeEngine(engine)]
	return a, ok
}

// normalizeEngine folds common aliases onto the canonical adapter keys.
func normalizeEngine(engine string) string {
	switch strings.ToLower(strings.TrimSpace(engine)) {
	case "mysql", "mariadb":
		return "mysql"
	case "postgres", "postgresql", "pg":
		return "postgres"
	case "sqlite", "sqlite3":
		return "sqlite"
	default:
		return strings.ToLower(strings.TrimSpace(engine))
	}
}

type mysqlAdapter struct{}

func (mysqlAdapter) Driver() string { return "mysql" }

func (mysqlAdapter) BuildDSN(ds DataSource) string {
	cfg := mysql.NewConfig()
	cfg.Net = "tcp"
	cfg.Addr = net.JoinHostPort(ds.Host, ds.Port)
	cfg.DBName = ds.Database
	cfg.User = ds.User
	cfg.Passwd = ds.Password
	cfg.ParseTime = true
	return cfg.FormatDSN()
}

func (mysqlAdapter) ProbeQuery() string { return "SELECT 1" }

type postgresAdapter struct{}

func (postgresAdapter) Driver() string { return "postgres" }

func (postgresAdapter) BuildDSN(ds DataSource) string {
	u := url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(ds.User, ds.Password),
		Host:   net.JoinHostPort(ds.Host, ds.Port),
		Path:   "/" + ds.Database,
	}
	q := u.Query()
	q.Set("sslmode", "disable")
	u.RawQuery = q.Encode()
	return u.String()
}

func (postgresAdapter) ProbeQuery() string { return "SELECT 1" }

type sqliteAdapter struct{}

func (sqliteAdapter) Driver() string { return "sqlite3" }

// BuildDSN for sqlite is just the database file path (Host/Port/User/Password do
// not apply).
func (sqliteAdapter) BuildDSN(ds DataSource) string { return ds.Database }

func (sqliteAdapter) ProbeQuery() string { return "SELECT 1" }

// friendlyDBError maps the common connection failures to clear, user-facing
// messages (used by the per-source Test).
func friendlyDBError(err error) string {
	var me *mysql.MySQLError
	if errors.As(err, &me) {
		switch me.Number {
		case 1045:
			return "access denied — check the username and password"
		case 1049:
			return "unknown database — check the database name"
		}
		return me.Message
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "unknown driver"):
		return "that database engine is not built into this server-panel build yet"
	case strings.Contains(msg, "connection refused"),
		strings.Contains(msg, "i/o timeout"),
		strings.Contains(msg, "dial tcp"),
		strings.Contains(msg, "no such host"):
		return "cannot reach the database at that host:port — check host, port, and firewall"
	}
	return msg
}
