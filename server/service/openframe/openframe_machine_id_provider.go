package openframe

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/rs/zerolog/log"
)

// OpenFrameMachineIdProvider reads the shared machine ID written by openframe-client.
type OpenFrameMachineIdProvider struct {
	machineId   string
	initialized bool
	mu          sync.RWMutex
}

func NewOpenFrameMachineIdProvider() *OpenFrameMachineIdProvider {
	return &OpenFrameMachineIdProvider{}
}

// GetMachineId returns the machine ID, reading from file if not cached
func (p *OpenFrameMachineIdProvider) GetMachineId() string {
	p.mu.RLock()
	if p.initialized {
		machineId := p.machineId
		p.mu.RUnlock()
		return machineId
	}
	p.mu.RUnlock()

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.initialized {
		return p.machineId
	}

	p.machineId = p.readFromFile()
	// Don't cache an empty read: the file may not be written yet, retry on the next call
	p.initialized = p.machineId != ""
	return p.machineId
}

// Refresh forces a re-read of machine ID from file
func (p *OpenFrameMachineIdProvider) Refresh() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.machineId = p.readFromFile()
	p.initialized = true
}

func (p *OpenFrameMachineIdProvider) getFilePath() string {
	switch runtime.GOOS {
	case "windows":
		programData := os.Getenv("ProgramData")
		if programData == "" {
			return ""
		}
		return filepath.Join(programData, "OpenFrame", "machine_id")
	case "darwin":
		return "/Library/Application Support/OpenFrame/machine_id"
	default:
		return "/var/lib/openframe/machine_id"
	}
}

func (p *OpenFrameMachineIdProvider) readFromFile() string {
	path := p.getFilePath()
	if path == "" {
		log.Warn().Msg("Could not determine machine_id file path")
		return ""
	}

	data, err := os.ReadFile(path)
	if err != nil {
		log.Debug().Str("path", path).Err(err).Msg("Could not read machine_id file")
		return ""
	}

	machineId := strings.TrimSpace(string(data))
	if machineId == "" {
		log.Warn().Str("path", path).Msg("Machine ID file is empty")
		return ""
	}

	log.Debug().Str("path", path).Msg("Read machine ID from file")
	return machineId
}
