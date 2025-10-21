package retention

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"progressdb/pkg/logger"
	"time"

	"progressdb/pkg/timeutil"
)

type fileLease struct {
	path string
}

type leaseFile struct {
	Owner   string `json:"owner"`
	Expires string `json:"expires"`
}

func NewFileLease(auditPath string) *fileLease {
	return &fileLease{path: filepath.Join(auditPath, "retention.lock")}
}

func (l *fileLease) Acquire(owner string, ttl time.Duration) (bool, error) {
	now := timeutil.Now()
	exp := now.Add(ttl)
	lf := leaseFile{Owner: owner, Expires: exp.Format(time.RFC3339)}
	b, _ := json.Marshal(lf)
	tmp := l.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		logger.Error("lease_tmp_write_failed", "path", tmp, "error", err)
		return false, err
	}
	// attempt to create lock atomically if not exists
	if err := os.Link(tmp, l.path); err == nil {
		os.Remove(tmp)
		logger.Info("lease_acquired_via_link", "path", l.path, "owner", owner)
		return true, nil
	}
	// if exists, read and check expiry
	data, err := os.ReadFile(l.path)
	if err != nil {
		return false, err
	}
	var existing leaseFile
	if err := json.Unmarshal(data, &existing); err != nil {
		return false, err
	}
	expT, _ := time.Parse(time.RFC3339, existing.Expires)
	if expT.Before(now) {
		// expired, try to replace
		if err := os.Rename(tmp, l.path); err != nil {
			logger.Error("lease_replace_failed", "error", err)
			return false, err
		}
		logger.Info("lease_acquired_replaced", "path", l.path, "owner", owner)
		return true, nil
	}
	os.Remove(tmp)
	logger.Info("lease_currently_held", "path", l.path, "owner", existing.Owner)
	return false, nil
}

func (l *fileLease) Renew(owner string, ttl time.Duration) error {
	// read existing, ensure owner matches
	data, err := os.ReadFile(l.path)
	if err != nil {
		return err
	}
	var existing leaseFile
	if err := json.Unmarshal(data, &existing); err != nil {
		return err
	}
	if existing.Owner != owner {
		return fmt.Errorf("not owner")
	}
	exp := timeutil.Now().Add(ttl)
	existing.Expires = exp.Format(time.RFC3339)
	b, _ := json.Marshal(existing)
	tmp := l.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		logger.Error("lease_renew_tmp_write_failed", "error", err)
		return err
	}
	if err := os.Rename(tmp, l.path); err != nil {
		logger.Error("lease_renew_rename_failed", "error", err)
		return err
	}
	logger.Info("lease_renewed", "path", l.path, "owner", owner)
	return nil
}

func (l *fileLease) Release(owner string) error {
	data, err := os.ReadFile(l.path)
	if err != nil {
		return err
	}
	var existing leaseFile
	if err := json.Unmarshal(data, &existing); err != nil {
		return err
	}
	if existing.Owner != owner {
		logger.Error("lease_release_not_owner", "owner", owner)
		return fmt.Errorf("not owner")
	}
	if err := os.Remove(l.path); err != nil {
		logger.Error("lease_release_remove_failed", "error", err)
		return err
	}
	logger.Info("lease_released", "path", l.path, "owner", owner)
	return nil
}
