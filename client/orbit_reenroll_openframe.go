package client

import (
	"errors"
	"io/fs"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

// unauthenticatedStreakArmed marks a saved 401 streak whose re-enroll has been armed.
const unauthenticatedStreakArmed = "armed"

// unauthenticatedStreakFilePath is where the 401 streak is kept between orbit runs, next to the node key file. It is empty
// when the client has no node key file.
func (oc *OrbitClient) unauthenticatedStreakFilePath() string {
	if oc.nodeKeyFilePath == "" {
		return ""
	}
	return oc.nodeKeyFilePath + ".unauthenticated"
}

// loadUnauthenticatedStreakLocked reads the 401 streak an earlier orbit run saved, once per client. A start time in the
// future is ignored. The caller must hold reenrollMu.
func (oc *OrbitClient) loadUnauthenticatedStreakLocked(now time.Time) {
	if oc.unauthenticatedStreakLoaded {
		return
	}
	oc.unauthenticatedStreakLoaded = true
	path := oc.unauthenticatedStreakFilePath()
	if path == "" || !oc.unauthenticatedSince.IsZero() {
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			log.Info().Err(err).Msg("read orbit 401 streak file")
		}
		return
	}
	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return
	}
	since, err := time.Parse(time.RFC3339Nano, fields[0])
	if err != nil || since.After(now) {
		return
	}
	oc.unauthenticatedSince = since
	oc.forceReenroll = len(fields) > 1 && fields[1] == unauthenticatedStreakArmed
}

// saveUnauthenticatedStreakLocked writes the 401 streak when it differs from prevSince and prevArmed. The caller must hold
// reenrollMu.
func (oc *OrbitClient) saveUnauthenticatedStreakLocked(prevSince time.Time, prevArmed bool) {
	path := oc.unauthenticatedStreakFilePath()
	if path == "" || oc.unauthenticatedSince.IsZero() {
		return
	}
	if oc.unauthenticatedSince.Equal(prevSince) && oc.forceReenroll == prevArmed {
		return
	}
	content := oc.unauthenticatedSince.UTC().Format(time.RFC3339Nano)
	if oc.forceReenroll {
		content += " " + unauthenticatedStreakArmed
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		log.Info().Err(err).Msg("write orbit 401 streak file")
	}
}

// removeUnauthenticatedStreakLocked deletes the saved 401 streak when there is one. The caller must hold reenrollMu.
func (oc *OrbitClient) removeUnauthenticatedStreakLocked() {
	oc.loadUnauthenticatedStreakLocked(time.Now())
	path := oc.unauthenticatedStreakFilePath()
	if path == "" || (oc.unauthenticatedSince.IsZero() && !oc.forceReenroll) {
		return
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		log.Info().Err(err).Msg("remove orbit 401 streak file")
	}
}
