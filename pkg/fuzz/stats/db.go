package stats

import (
	"database/sql"
	_ "embed"
	"fmt"
	"os"
	"sync"

	_ "github.com/mattn/go-sqlite3"
	"github.com/pkg/errors"
)

type StatsDatabase interface {
	Close()

	InsertRecord(event FuzzingEvent) error
}

var (
	//go:embed schema.sql
	dbSchemaCreateStatement string
)

type sqliteStatsDatabase struct {
	db       *sql.DB
	scanName string

	siteIDCache      map[string]int
	templateIDCache  map[string]int
	componentIDCache map[string]int
	cacheMutex       *sync.Mutex
}

func newSqliteStatsDatabase(scanName string) (*sqliteStatsDatabase, error) {
	filename := fmt.Sprintf("%s.stats.db", scanName)

	connectionString := fmt.Sprintf("./%s?_journal_mode=WAL&_synchronous=NORMAL", filename)
	db, err := sql.Open("sqlite3", connectionString)
	if err != nil {
		return nil, errors.Wrap(err, "could not open database")
	}

	_, err = db.Exec(dbSchemaCreateStatement)
	if err != nil {
		return nil, errors.Wrap(err, "could not create schema")
	}

	database := &sqliteStatsDatabase{
		scanName:         scanName,
		db:               db,
		siteIDCache:      make(map[string]int),
		templateIDCache:  make(map[string]int),
		componentIDCache: make(map[string]int),
		cacheMutex:       &sync.Mutex{},
	}
	return database, nil
}

func (s *sqliteStatsDatabase) Close() {
	// Disable WAL mode and switch back to DELETE mode
	_ = s.db.Close()
	os.Remove(fmt.Sprintf("%s.stats.db-wal", s.scanName))
	os.Remove(fmt.Sprintf("%s.stats.db-shm", s.scanName))
}

func (s *sqliteStatsDatabase) InsertRecord(event FuzzingEvent) error {
	tx, err := s.db.Begin()
	if err != nil {
		return errors.Wrap(err, "could not begin transaction")
	}

	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	siteID, err := s.getSiteID(tx, event.SiteName)
	if err != nil {
		return errors.Wrap(err, "could not get site_id")
	}

	templateID, err := s.getTemplateID(tx, event.TemplateID)
	if err != nil {
		return errors.Wrap(err, "could not get template_id")
	}

	componentID, err := s.getComponentID(tx, siteID, event.ComponentType, event.ComponentName)
	if err != nil {
		return errors.Wrap(err, "could not get component_id")
	}

	err = s.insertFuzzingResult(tx, componentID, templateID, event.PayloadSent, event.StatusCode)
	if err != nil {
		return errors.Wrap(err, "could not insert fuzzing result")
	}

	if err := tx.Commit(); err != nil {
		return errors.Wrap(err, "could not commit transaction")
	}

	return nil
}

func (s *sqliteStatsDatabase) getSiteID(tx *sql.Tx, siteName string) (int, error) {
	var siteID int

	s.cacheMutex.Lock()
	if id, ok := s.siteIDCache[siteName]; ok {
		s.cacheMutex.Unlock()
		return id, nil
	}
	s.cacheMutex.Unlock()

	err := tx.QueryRow(
		`INSERT OR IGNORE INTO sites (site_name)
		VALUES (?) RETURNING site_id
	`, siteName).Scan(&siteID)
	if err != nil {
		return 0, err
	}

	// Cache the site_id
	s.cacheMutex.Lock()
	s.siteIDCache[siteName] = siteID
	s.cacheMutex.Unlock()

	return siteID, nil
}

func (s *sqliteStatsDatabase) getTemplateID(tx *sql.Tx, templateName string) (int, error) {
	var templateID int

	s.cacheMutex.Lock()
	if id, ok := s.templateIDCache[templateName]; ok {
		s.cacheMutex.Unlock()
		return id, nil
	}
	s.cacheMutex.Unlock()

	err := tx.QueryRow(`
        INSERT OR IGNORE INTO templates (template_name)
        VALUES (?) RETURNING template_id
    `, templateName).Scan(&templateID)
	if err != nil {
		return 0, err
	}

	s.cacheMutex.Lock()
	s.templateIDCache[templateName] = templateID
	s.cacheMutex.Unlock()

	return templateID, nil
}

func (s *sqliteStatsDatabase) getComponentID(tx *sql.Tx, siteID int, componentType, componentName string) (int, error) {
	key := fmt.Sprintf("%d:%s:%s", siteID, componentType, componentName)
	var componentID int

	s.cacheMutex.Lock()
	if id, ok := s.componentIDCache[key]; ok {
		s.cacheMutex.Unlock()
		return id, nil
	}
	s.cacheMutex.Unlock()

	err := tx.QueryRow(`
        INSERT OR IGNORE INTO components (site_id, component_type, component_name)
        VALUES (?, ?, ?) RETURNING component_id
    `, siteID, componentType, componentName).Scan(&componentID)
	if err != nil {
		return 0, err
	}

	s.cacheMutex.Lock()
	s.componentIDCache[key] = componentID
	s.cacheMutex.Unlock()

	return componentID, nil
}

func (s *sqliteStatsDatabase) insertFuzzingResult(tx *sql.Tx, componentID, templateID int, payloadSent string, statusCode int) error {
	_, err := tx.Exec(`
        INSERT INTO fuzzing_results (component_id, template_id, payload_sent, status_code_received)
        VALUES (?, ?, ?, ?)
    `, componentID, templateID, payloadSent, statusCode)
	return err
}
