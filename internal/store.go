package internal

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/andersmyrmel/cold-cli/internal/d1http"
)

type Dialect string

const (
	DialectSQLite   Dialect = "sqlite"
	DialectPostgres Dialect = "postgres"

	// Stable advisory lock key for the global tick loop in Postgres mode.
	postgresTickLockKey int64 = 0x636f6c645f746963
)

type TickLock interface {
	Close() error
}

// Store is a thin wrapper around the database handle plus dialect metadata and
// dialect-specific tick locking.
type Store struct {
	DB      *sql.DB
	Dialect Dialect

	target        string
	displayTarget string
	tickLockPath  string
	tickLockKey   int64
	d1Mode        bool

	acquireSQLiteTickLock   func(path string) (TickLock, error)
	acquirePostgresTickLock func(ctx context.Context, db *sql.DB, key int64) (TickLock, error)
}

type storeOpenConfig struct {
	dialect     Dialect
	sqlitePath  string
	postgresURL string
	d1Proxy     string
	d1Token     string

	openDB            func(driverName, dataSourceName string) (*sql.DB, error)
	bootstrapSQLite   func(db *sql.DB) error
	bootstrapPostgres func(db *sql.DB) error

	acquireSQLiteTickLock   func(path string) (TickLock, error)
	acquirePostgresTickLock func(ctx context.Context, db *sql.DB, key int64) (TickLock, error)
}

func defaultStoreOpenConfig() storeOpenConfig {
	return storeOpenConfig{
		openDB:                  sql.Open,
		bootstrapSQLite:         bootstrapSQLiteSchema,
		bootstrapPostgres:       bootstrapPostgresSchema,
		acquireSQLiteTickLock:   acquireSQLiteTickLock,
		acquirePostgresTickLock: acquirePostgresTickLock,
	}
}

func storeOpenConfigFromEnv() storeOpenConfig {
	cfg := defaultStoreOpenConfig()
	cfg.dialect = CurrentDialect()
	if cfg.dialect == DialectPostgres {
		cfg.postgresURL = strings.TrimSpace(os.Getenv("COLD_CLI_DATABASE_URL"))
	} else {
		cfg.sqlitePath = DBPath()
		cfg.d1Proxy = strings.TrimSpace(os.Getenv("OPENOUTREACH_D1_PROXY"))
		cfg.d1Token = os.Getenv("INTERNAL_CONTAINER_TOKEN")
	}
	return cfg
}

// CurrentDialect returns the active database dialect from environment.
func CurrentDialect() Dialect {
	if strings.TrimSpace(os.Getenv("COLD_CLI_DATABASE_URL")) != "" {
		return DialectPostgres
	}
	return DialectSQLite
}

// DatabaseDisplayTarget returns a user-facing database target description.
func DatabaseDisplayTarget() string {
	if url := strings.TrimSpace(os.Getenv("COLD_CLI_DATABASE_URL")); url != "" {
		return redactDatabaseURL(url)
	}
	if proxy := strings.TrimSpace(os.Getenv("OPENOUTREACH_D1_PROXY")); proxy != "" {
		return "cloudflare D1 via " + proxy
	}
	return DBPath()
}

// OpenStore opens the configured database and bootstraps the current schema.
func OpenStore() (*Store, error) {
	return openStore(storeOpenConfigFromEnv())
}

func openStore(cfg storeOpenConfig) (*Store, error) {
	defaults := defaultStoreOpenConfig()
	if cfg.openDB == nil {
		cfg.openDB = defaults.openDB
	}
	if cfg.bootstrapSQLite == nil {
		cfg.bootstrapSQLite = defaults.bootstrapSQLite
	}
	if cfg.bootstrapPostgres == nil {
		cfg.bootstrapPostgres = defaults.bootstrapPostgres
	}
	if cfg.acquireSQLiteTickLock == nil {
		cfg.acquireSQLiteTickLock = defaults.acquireSQLiteTickLock
	}
	if cfg.acquirePostgresTickLock == nil {
		cfg.acquirePostgresTickLock = defaults.acquirePostgresTickLock
	}

	dialect := cfg.dialect
	if dialect == "" {
		if strings.TrimSpace(cfg.postgresURL) != "" {
			dialect = DialectPostgres
		} else {
			dialect = DialectSQLite
		}
	}

	store := &Store{
		Dialect:                 dialect,
		tickLockPath:            filepath.Join(DataDir(), "tick.lock"),
		tickLockKey:             postgresTickLockKey,
		acquireSQLiteTickLock:   cfg.acquireSQLiteTickLock,
		acquirePostgresTickLock: cfg.acquirePostgresTickLock,
	}

	switch dialect {
	case DialectSQLite:
		if strings.TrimSpace(cfg.d1Proxy) != "" {
			db, err := cfg.openDB("d1http", d1http.FormatDSN(cfg.d1Proxy, cfg.d1Token))
			if err != nil {
				return nil, fmt.Errorf("opening D1 proxy: %w", err)
			}
			// D1 is remote HTTP; allow >1 conn so open Rows do not block other queries
			// (sqlite file mode keeps MaxOpenConns(1) for local write locking).
			db.SetMaxOpenConns(8)
			db.SetMaxIdleConns(4)

			registerDBDialect(db, dialect)
			if err := cfg.bootstrapSQLite(db); err != nil {
				unregisterDBDialect(db)
				db.Close()
				return nil, err
			}

			store.DB = db
			store.d1Mode = true
			store.target = cfg.d1Proxy
			store.displayTarget = "cloudflare D1 via " + cfg.d1Proxy
			return store, nil
		}

		if cfg.sqlitePath == "" {
			cfg.sqlitePath = DBPath()
		}

		db, err := cfg.openDB("sqlite", sqliteDSN(cfg.sqlitePath))
		if err != nil {
			return nil, fmt.Errorf("opening database: %w", err)
		}
		db.SetMaxOpenConns(1)
		db.SetMaxIdleConns(1)

		registerDBDialect(db, dialect)
		if err := cfg.bootstrapSQLite(db); err != nil {
			unregisterDBDialect(db)
			db.Close()
			return nil, err
		}

		store.DB = db
		store.target = cfg.sqlitePath
		store.displayTarget = cfg.sqlitePath
		return store, nil

	case DialectPostgres:
		if strings.TrimSpace(cfg.postgresURL) == "" {
			return nil, fmt.Errorf("COLD_CLI_DATABASE_URL is empty")
		}

		db, err := cfg.openDB("pgx", cfg.postgresURL)
		if err != nil {
			return nil, fmt.Errorf("opening database: %w", err)
		}

		registerDBDialect(db, dialect)
		if err := cfg.bootstrapPostgres(db); err != nil {
			unregisterDBDialect(db)
			db.Close()
			return nil, err
		}

		store.DB = db
		store.target = cfg.postgresURL
		store.displayTarget = redactDatabaseURL(cfg.postgresURL)
		return store, nil

	default:
		return nil, fmt.Errorf("unsupported database dialect %q", dialect)
	}
}

// Close closes the underlying database handle.
func (s *Store) Close() error {
	if s == nil || s.DB == nil {
		return nil
	}
	unregisterDBDialect(s.DB)
	return s.DB.Close()
}

// DisplayTarget returns a user-facing database target description.
func (s *Store) DisplayTarget() string {
	if s == nil {
		return ""
	}
	return s.displayTarget
}

// AcquireTickLock acquires the dialect-specific tick lock and returns a handle
// that must be closed to release it.
func (s *Store) AcquireTickLock(ctx context.Context) (TickLock, error) {
	if s == nil {
		return nil, fmt.Errorf("store is nil")
	}

	switch s.Dialect {
	case DialectSQLite:
		if s.d1Mode {
			if s.DB == nil {
				return nil, fmt.Errorf("database handle is nil")
			}
			return acquireD1TickLock(ctx, s.DB)
		}
		return s.acquireSQLiteTickLock(s.tickLockPath)
	case DialectPostgres:
		if s.DB == nil {
			return nil, fmt.Errorf("database handle is nil")
		}
		return s.acquirePostgresTickLock(ctx, s.DB, s.tickLockKey)
	default:
		return nil, fmt.Errorf("unsupported database dialect %q", s.Dialect)
	}
}

func redactDatabaseURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "postgres via COLD_CLI_DATABASE_URL"
	}
	return parsed.Redacted()
}

func acquireSQLiteTickLock(lockPath string) (TickLock, error) {
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, fmt.Errorf("opening lock file: %w", err)
	}

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, fmt.Errorf("tick already running")
	}

	return f, nil
}

const d1TickLockStaleSeconds = 180

type d1TickLock struct {
	db     *sql.DB
	holder string
}

func acquireD1TickLock(ctx context.Context, db *sql.DB) (TickLock, error) {
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS tick_lock (
		id INTEGER PRIMARY KEY CHECK (id = 1),
		holder TEXT NOT NULL,
		locked_at INTEGER NOT NULL
	)`); err != nil {
		return nil, fmt.Errorf("creating tick_lock: %w", err)
	}

	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return nil, fmt.Errorf("tick lock holder: %w", err)
	}
	holder := fmt.Sprintf("%d-%s", time.Now().UnixNano(), hex.EncodeToString(buf[:]))

	res, err := db.ExecContext(ctx, `
		INSERT INTO tick_lock (id, holder, locked_at)
		VALUES (1, ?, CAST(strftime('%s','now') AS INTEGER))
		ON CONFLICT(id) DO UPDATE SET
			holder = excluded.holder,
			locked_at = excluded.locked_at
		WHERE tick_lock.locked_at < CAST(strftime('%s','now') AS INTEGER) - ?`,
		holder, d1TickLockStaleSeconds)
	if err != nil {
		return nil, fmt.Errorf("acquiring D1 tick lock: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, fmt.Errorf("tick already running")
	}
	return &d1TickLock{db: db, holder: holder}, nil
}

func (l *d1TickLock) Close() error {
	if l == nil || l.db == nil {
		return nil
	}
	_, err := l.db.Exec(`DELETE FROM tick_lock WHERE id = 1 AND holder = ?`, l.holder)
	return err
}

type postgresAdvisoryLocker interface {
	TryLock(ctx context.Context, key int64) (bool, error)
	Unlock(ctx context.Context, key int64) error
	Close() error
}

type sqlPostgresAdvisoryLocker struct {
	conn *sql.Conn
}

func (l *sqlPostgresAdvisoryLocker) TryLock(ctx context.Context, key int64) (bool, error) {
	var ok bool
	if err := l.conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock($1)", key).Scan(&ok); err != nil {
		return false, err
	}
	return ok, nil
}

func (l *sqlPostgresAdvisoryLocker) Unlock(ctx context.Context, key int64) error {
	var ok bool
	if err := l.conn.QueryRowContext(ctx, "SELECT pg_advisory_unlock($1)", key).Scan(&ok); err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("tick advisory lock was not held")
	}
	return nil
}

func (l *sqlPostgresAdvisoryLocker) Close() error {
	return l.conn.Close()
}

type postgresTickLock struct {
	locker postgresAdvisoryLocker
	key    int64
}

func acquirePostgresTickLock(ctx context.Context, db *sql.DB, key int64) (TickLock, error) {
	conn, err := db.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("opening advisory lock connection: %w", err)
	}

	lock, err := acquirePostgresTickLockWithLocker(ctx, &sqlPostgresAdvisoryLocker{conn: conn}, key)
	if err != nil {
		conn.Close()
		return nil, err
	}

	return lock, nil
}

func acquirePostgresTickLockWithLocker(ctx context.Context, locker postgresAdvisoryLocker, key int64) (TickLock, error) {
	ok, err := locker.TryLock(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("acquiring advisory tick lock: %w", err)
	}
	if !ok {
		return nil, fmt.Errorf("tick already running")
	}
	return &postgresTickLock{locker: locker, key: key}, nil
}

func (l *postgresTickLock) Close() error {
	if l == nil || l.locker == nil {
		return nil
	}

	ctx := context.Background()
	unlockErr := l.locker.Unlock(ctx, l.key)
	closeErr := l.locker.Close()

	if unlockErr != nil {
		return unlockErr
	}
	return closeErr
}
