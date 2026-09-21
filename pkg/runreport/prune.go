package runreport

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Prune removes expired run report files under cfg.Dir for the given entityID.
func Prune(cfg Config, entityID string) error {
	cfg = cfg.Defaults()
	if !cfg.Enabled {
		return nil
	}
	root := filepath.Clean(cfg.Dir)
	if err := walkRunFiles(root, func(path string, info os.FileInfo) error {
		if entityID != "" && !strings.Contains(path, sanitizePathSegment(entityID, "")) {
			return nil
		}
		maxAge := time.Duration(cfg.MaxAgeHours) * time.Hour
		if maxAge > 0 && time.Since(info.ModTime()) > maxAge {
			return os.Remove(path)
		}
		return nil
	}); err != nil {
		return err
	}
	if entityID != "" && cfg.KeepPerEntity > 0 {
		return trimEntityReports(root, entityID, cfg.KeepPerEntity)
	}
	return nil
}

func trimEntityReports(root, entityID string, keep int) error {
	entitySeg := sanitizePathSegment(entityID, "entity")
	var matches []string
	needle := string(filepath.Separator) + entitySeg + string(filepath.Separator)
	err := walkRunFiles(root, func(path string, info os.FileInfo) error {
		if strings.Contains(path, needle) || strings.HasSuffix(filepath.Dir(path), entitySeg) {
			matches = append(matches, path)
		}
		return nil
	})
	if err != nil {
		return err
	}
	if len(matches) <= keep {
		return nil
	}
	type stamped struct {
		path string
		mod  time.Time
	}
	files := make([]stamped, 0, len(matches))
	for _, path := range matches {
		info, statErr := os.Stat(path)
		if statErr != nil {
			return statErr
		}
		files = append(files, stamped{path: path, mod: info.ModTime()})
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].mod.After(files[j].mod)
	})
	for _, file := range files[keep:] {
		if rmErr := os.Remove(file.path); rmErr != nil {
			return rmErr
		}
	}
	return nil
}

func walkRunFiles(root string, fn func(path string, info os.FileInfo) error) error {
	info, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if !info.IsDir() {
		return nil
	}
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(path), ".json") {
			return nil
		}
		fi, err := d.Info()
		if err != nil {
			return err
		}
		return fn(path, fi)
	})
}
