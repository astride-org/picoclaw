package tasks

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/lucas-stellet/playbookd"
	"github.com/lucas-stellet/playbookd/embed"
)

// initPlaybookManager creates a PlaybookManager using .playbookd.toml from the
// workspace if available, falling back to BM25-only (Noop embeddings) otherwise.
func initPlaybookManager(workspace string) (*playbookd.PlaybookManager, error) {
	playbooksDir := filepath.Join(workspace, "playbooks")
	os.MkdirAll(playbooksDir, 0o755)

	configPath := filepath.Join(workspace, ".playbookd.toml")
	cfg, err := playbookd.LoadConfig(configPath)
	if err == nil {
		mgrCfg, err := cfg.BuildManagerConfig()
		if err != nil {
			return nil, fmt.Errorf("build playbook config: %w", err)
		}
		mgrCfg.DataDir = playbooksDir
		return playbookd.NewPlaybookManager(mgrCfg)
	}

	// No config file → BM25 only (current behavior)
	return playbookd.NewPlaybookManager(playbookd.ManagerConfig{
		DataDir:   playbooksDir,
		EmbedFunc: embed.Noop(),
	})
}
