package pythoninspector

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"gopkg.in/yaml.v3"
)

// ScriptManager manages inspection scripts
type ScriptManager struct {
	scriptsDir string
	scripts    map[string]*InspectionScript // name -> script
	mu         sync.RWMutex
}

// NewScriptManager creates a new script manager
func NewScriptManager(scriptsDir string) *ScriptManager {
	return &ScriptManager{
		scriptsDir: scriptsDir,
		scripts:    make(map[string]*InspectionScript),
	}
}

// LoadScripts loads all scripts from the scripts directory
func (sm *ScriptManager) LoadScripts() error {
	log.Infof("Loading inspection scripts from %s", sm.scriptsDir)

	// Check if scripts directory exists
	if _, err := os.Stat(sm.scriptsDir); os.IsNotExist(err) {
		log.Warnf("Scripts directory does not exist: %s", sm.scriptsDir)
		return nil
	}

	// Scan scripts directory
	entries, err := os.ReadDir(sm.scriptsDir)
	if err != nil {
		return fmt.Errorf("failed to read scripts directory: %w", err)
	}

	loadedCount := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		scriptName := entry.Name()
		scriptDir := filepath.Join(sm.scriptsDir, scriptName)

		// Load single script
		if err := sm.loadScript(scriptName, scriptDir); err != nil {
			log.Warnf("Failed to load script %s: %v", scriptName, err)
			continue
		}

		loadedCount++
	}

	log.Infof("Successfully loaded %d inspection scripts", loadedCount)
	return nil
}

// loadScript loads a single script
func (sm *ScriptManager) loadScript(name, dir string) error {
	// Read metadata
	metadataPath := filepath.Join(dir, "metadata.yaml")
	metadataData, err := os.ReadFile(metadataPath)
	if err != nil {
		return fmt.Errorf("metadata not found: %w", err)
	}

	var metadata ScriptMetadata
	if err := yaml.Unmarshal(metadataData, &metadata); err != nil {
		return fmt.Errorf("failed to parse metadata: %w", err)
	}

	// Verify script file exists
	scriptPath := filepath.Join(dir, "inspect.py")
	if _, err := os.Stat(scriptPath); err != nil {
		return fmt.Errorf("inspect.py not found: %w", err)
	}

	// Validate and set defaults
	if metadata.Name == "" {
		metadata.Name = name
	}
	if metadata.Timeout == 0 {
		metadata.Timeout = 30
	}
	if metadata.SafetyLevel == "" {
		metadata.SafetyLevel = "safe"
	}

	script := &InspectionScript{
		Metadata:   metadata,
		ScriptPath: scriptPath,
	}

	sm.mu.Lock()
	sm.scripts[metadata.Name] = script
	sm.mu.Unlock()

	log.Infof("Loaded script: %s (v%s) - %s",
		metadata.Name, metadata.Version, metadata.Description)

	return nil
}

// GetScript retrieves a script by name
func (sm *ScriptManager) GetScript(name string) (*InspectionScript, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	script, ok := sm.scripts[name]
	if !ok {
		return nil, fmt.Errorf("script not found: %s", name)
	}

	if !script.Metadata.Enabled {
		return nil, fmt.Errorf("script disabled: %s", name)
	}

	return script, nil
}

// ListScripts returns all scripts
func (sm *ScriptManager) ListScripts() []*InspectionScript {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	scripts := make([]*InspectionScript, 0, len(sm.scripts))
	for _, script := range sm.scripts {
		scripts = append(scripts, script)
	}

	return scripts
}

// ListEnabledScripts returns all enabled scripts
func (sm *ScriptManager) ListEnabledScripts() []*InspectionScript {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	scripts := make([]*InspectionScript, 0, len(sm.scripts))
	for _, script := range sm.scripts {
		if script.Metadata.Enabled {
			scripts = append(scripts, script)
		}
	}

	return scripts
}

// SearchScripts searches scripts by query string
func (sm *ScriptManager) SearchScripts(query string) []*InspectionScript {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	query = strings.ToLower(query)
	scripts := make([]*InspectionScript, 0)

	for _, script := range sm.scripts {
		// Search in name, description, and tags
		if strings.Contains(strings.ToLower(script.Metadata.Name), query) ||
			strings.Contains(strings.ToLower(script.Metadata.Description), query) ||
			containsTag(script.Metadata.Tags, query) {
			scripts = append(scripts, script)
		}
	}

	return scripts
}

// GetScriptsByTag returns scripts with a specific tag
func (sm *ScriptManager) GetScriptsByTag(tag string) []*InspectionScript {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	scripts := make([]*InspectionScript, 0)
	tag = strings.ToLower(tag)

	for _, script := range sm.scripts {
		if containsTag(script.Metadata.Tags, tag) {
			scripts = append(scripts, script)
		}
	}

	return scripts
}

// containsTag checks if tags contain the query
func containsTag(tags []string, query string) bool {
	for _, tag := range tags {
		if strings.Contains(strings.ToLower(tag), query) {
			return true
		}
	}
	return false
}

// ValidateScript validates a script
func (sm *ScriptManager) ValidateScript(name string) error {
	script, err := sm.GetScript(name)
	if err != nil {
		return err
	}

	// Check if script file is accessible
	if _, err := os.Stat(script.ScriptPath); err != nil {
		return fmt.Errorf("script file not accessible: %w", err)
	}

	return nil
}

