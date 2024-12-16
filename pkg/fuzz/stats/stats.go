// Package stats implements a statistics recording module for
// nuclei fuzzing.
package stats

import (
	"fmt"
	"net/url"

	"github.com/pkg/errors"
)

// Tracker is a stats tracker module for fuzzing server
type Tracker struct {
	database *simpleStats
}

// NewTracker creates a new tracker instance
func NewTracker() (*Tracker, error) {
	db, err := NewSimpleStats()
	if err != nil {
		return nil, errors.Wrap(err, "could not create new tracker")
	}

	tracker := &Tracker{
		database: db,
	}
	return tracker, nil
}

func (t *Tracker) GetStats() SimpleStatsResponse {
	return t.database.GetStatistics()
}

// Close closes the tracker
func (t *Tracker) Close() {
	t.database.Close()
}

// FuzzingEvent is a fuzzing event
type FuzzingEvent struct {
	URL           string
	ComponentType string
	ComponentName string
	TemplateID    string
	PayloadSent   string
	StatusCode    int
	Matched       bool
	RawRequest    string
	RawResponse   string
	Severity      string

	siteName string
}

func (t *Tracker) RecordResultEvent(event FuzzingEvent) {
	event.siteName = getCorrectSiteName(event.URL)
	t.database.InsertMatchedRecord(event)
}

type ComponentEvent struct {
	URL           string
	ComponentType string
	ComponentName string

	siteName string
}

func (t *Tracker) RecordComponentEvent(event ComponentEvent) {
	event.siteName = getCorrectSiteName(event.URL)
	t.database.InsertComponent(event)
}

type ErrorEvent struct {
	TemplateID string
	URL        string
	Error      string
}

func (t *Tracker) RecordErrorEvent(event ErrorEvent) {
	t.database.InsertError(event)
}

func getCorrectSiteName(originalURL string) string {
	parsed, err := url.Parse(originalURL)
	if err != nil {
		return ""
	}

	// Site is the host:port combo
	siteName := parsed.Host
	if parsed.Port() == "" {
		if parsed.Scheme == "https" {
			siteName = fmt.Sprintf("%s:443", siteName)
		} else if parsed.Scheme == "http" {
			siteName = fmt.Sprintf("%s:80", siteName)
		}
	}
	return siteName
}