package traefikgeoip2

import (
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/IncSW/geoip2"
)

// reloadCheckInterval bounds how often a request may trigger a stat of the
// database file; reloadGracePeriod is how long a superseded database stays
// referenced before its cache is released (in-flight requests hold their own
// pointer, so the reader itself is garbage collected once they finish).
var (
	reloadCheckInterval = 60 * time.Second
	reloadGracePeriod   = 30 * time.Second
)

var errNotLoaded = errors.New("not loaded yet")

// dbState is one loaded database with its result cache. A state is replaced as
// a whole when the file changes and is never modified afterwards; requests take
// a pointer for their duration.
type dbState struct {
	lookup LookupGeoIP2 // nil when the database is unavailable
	cache  *resultCache
	size   int64
	mtime  time.Time
	err    error // why lookup is nil
}

func (s *dbState) sameFile(fi os.FileInfo) bool {
	return s.lookup != nil && s.size == fi.Size() && s.mtime.Equal(fi.ModTime())
}

type cacheSettings struct {
	size int
	ttl  time.Duration
}

// geoDB is the process-wide entry for one database path. Traefik calls New for
// every router on every dynamic configuration reload; all those instances share
// one reader and one cache per path.
type geoDB struct {
	path  string
	kind  dbKind
	cache cacheSettings
	log   *logger

	mu    sync.RWMutex
	state *dbState

	loadMu        sync.Mutex // serializes check()
	checkMu       sync.Mutex // guards the fields below
	nextCheck     time.Time
	checking      bool
	failedVersion string // size:mtime of a file version that failed to open
}

var (
	registryMu sync.Mutex
	registry   = map[string]*geoDB{}
)

// sharedDB returns the entry for dbPath, creating it on first use, and checks
// the file synchronously so that a reload picks up a replaced database.
// The cache settings of the first instance that loads a path apply to it.
func sharedDB(dbPath string, cache cacheSettings) *geoDB {
	registryMu.Lock()
	db, ok := registry[dbPath]
	if !ok {
		db = &geoDB{
			path:  dbPath,
			kind:  kindOf(dbPath),
			cache: cache,
			log:   newLogger(levelInfo, dbPath),
			state: &dbState{err: errNotLoaded},
		}
		registry[dbPath] = db
	}
	registryMu.Unlock()
	db.check(time.Now())
	return db
}

func (db *geoDB) current() *dbState {
	db.mu.RLock()
	defer db.mu.RUnlock()
	return db.state
}

// maybeCheck starts an asynchronous file check unless one ran less than
// reloadCheckInterval ago or is in progress. It returns whether it started one.
func (db *geoDB) maybeCheck(now time.Time) bool {
	db.checkMu.Lock()
	defer db.checkMu.Unlock()
	if db.checking || now.Before(db.nextCheck) {
		return false
	}
	db.checking = true
	go db.check(now)
	return true
}

// check stats the file and swaps in a freshly opened state when the size or
// modification time changed. A replacement that does not open keeps the
// previous database and is reported once per file version.
func (db *geoDB) check(now time.Time) {
	db.loadMu.Lock()
	defer db.loadMu.Unlock()
	defer func() {
		db.checkMu.Lock()
		db.checking = false
		db.nextCheck = now.Add(reloadCheckInterval)
		db.checkMu.Unlock()
	}()

	cur := db.current()
	fi, err := os.Stat(db.path)
	if err != nil {
		if cur.lookup == nil && cur.err != nil && cur.err.Error() == err.Error() {
			return // already reported
		}
		db.log.errorf("database unavailable: %v", err)
		db.swap(&dbState{err: err})
		return
	}
	if cur.sameFile(fi) {
		return
	}
	version := fmt.Sprintf("%d:%d", fi.Size(), fi.ModTime().UnixNano())
	db.checkMu.Lock()
	failed := db.failedVersion == version
	db.checkMu.Unlock()
	if failed {
		return // this exact file version already failed to open and was reported
	}

	st := openState(db.path, db.kind, fi, db.cache)
	if st.lookup == nil {
		db.checkMu.Lock()
		db.failedVersion = version
		db.checkMu.Unlock()
		if cur.lookup != nil {
			db.log.errorf("database not reloaded, keeping the previously loaded version: %v", st.err)
			return
		}
		db.log.errorf("database not loaded: %v", st.err)
		db.swap(st)
		return
	}
	db.log.infof("database loaded: %d bytes, modified %s", fi.Size(), fi.ModTime().UTC().Format(time.RFC3339))
	db.swap(st)
}

// swap publishes st and releases the previous state's cache after the grace period.
func (db *geoDB) swap(st *dbState) {
	db.mu.Lock()
	old := db.state
	db.state = st
	db.mu.Unlock()
	if old != nil && old.cache != nil {
		time.AfterFunc(reloadGracePeriod, old.cache.purge)
	}
}

func openState(path string, kind dbKind, fi os.FileInfo, cache cacheSettings) *dbState {
	st := &dbState{size: fi.Size(), mtime: fi.ModTime()}
	lookup, err := openLookup(path, kind)
	if err != nil {
		st.err = err
		return st
	}
	st.lookup = lookup
	st.cache = newResultCache(cache.size, cache.ttl)
	return st
}

func openLookup(path string, kind dbKind) (LookupGeoIP2, error) {
	switch kind {
	case kindCity:
		rdr, err := geoip2.NewCityReaderFromFile(path)
		if err != nil {
			return nil, err
		}
		return CreateCityDBLookup(rdr), nil
	case kindCountry:
		rdr, err := geoip2.NewCountryReaderFromFile(path)
		if err != nil {
			return nil, err
		}
		return CreateCountryDBLookup(rdr), nil
	case kindASN:
		rdr, err := geoip2.NewASNReaderFromFile(path)
		if err != nil {
			return nil, err
		}
		return CreateASNDBLookup(rdr), nil
	default:
		return nil, fmt.Errorf("cannot tell the database kind from the file name %q: it must contain City, Country or ASN", path)
	}
}
