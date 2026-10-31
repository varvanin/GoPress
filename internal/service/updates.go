package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopress/internal/backup"
	"gopress/internal/store"
	"gopress/internal/update"
	"gopress/internal/version"
)

var ErrUpdateConfig = errors.New("update configuration is incomplete")
var updateClient = update.Client(20 * time.Second)

type UpdateState struct {
	Phase          string        `json:"phase"`
	Release        update.Signed `json:"release"`
	FromVersion    string        `json:"from_version"`
	PreviousImage  string        `json:"previous_image"`
	Backup         string        `json:"backup"`
	Started        int64         `json:"started"`
	WasMaintenance bool          `json:"was_maintenance"`
}

func (s UpdateState) Pending() bool {
	return s.Phase == "checking" || s.Phase == "backing-up" || s.Phase == "deploying" || s.Phase == "requested" || s.Phase == "uncertain"
}
func updateValue(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
func updateTrust() (string, string) {
	return updateValue("GOPRESS_UPDATE_FEED", version.UpdateFeed), updateValue("GOPRESS_UPDATE_PUBLIC_KEY", version.UpdatePublicKey)
}
func dokploySettings() update.Dokploy {
	return update.Dokploy{URL: strings.TrimRight(os.Getenv("GOPRESS_DOKPLOY_URL"), "/"), Key: os.Getenv("GOPRESS_DOKPLOY_API_KEY"), ApplicationID: os.Getenv("GOPRESS_DOKPLOY_APPLICATION_ID"), Client: updateClient}
}
func UpdatesConfigured() bool {
	feed, key := updateTrust()
	return version.Deployment == "docker" && feed != "" && key != "" && dokploySettings().Configured()
}
func (v *Service) writeUpdate(name string, value any) error {
	root := filepath.Join(v.Store.Dir, "updates")
	if err := os.MkdirAll(root, 0700); err != nil {
		return err
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(root, ".update-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err = file.Write(raw); err != nil {
		file.Close()
		return err
	}
	if err = file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	if err = os.Rename(file.Name(), filepath.Join(root, name)); err != nil {
		return err
	}
	if err = syncDirectory(root); err != nil {
		return err
	}
	return syncDirectory(v.Store.Dir)
}
func (v *Service) readUpdate(name string, value any) error {
	raw, err := os.ReadFile(filepath.Join(v.Store.Dir, "updates", name))
	if err != nil {
		return err
	}
	if len(raw) > 64000 {
		return ErrInvalid
	}
	return json.Unmarshal(raw, value)
}
func (v *Service) UpdateState() (UpdateState, error) {
	var state UpdateState
	err := v.readUpdate("state.json", &state)
	if errors.Is(err, os.ErrNotExist) {
		err = nil
	}
	return state, err
}
func (v *Service) CheckedRelease() (update.Release, error) {
	var signed update.Signed
	if err := v.readUpdate("release.json", &signed); err != nil {
		return update.Release{}, err
	}
	_, key := updateTrust()
	return signed.Verify(key, version.ImageRepository)
}
func canUpdate(release update.Release) bool {
	current := update.CurrentBase(version.Current)
	minimum, err := update.Compare(current, release.MinimumVersion)
	if err != nil || minimum < 0 {
		return false
	}
	newer, err := update.Compare(release.Version, current)
	return err == nil && (newer > 0 || newer == 0 && strings.Contains(version.Current, "-"))
}
func CanInstallRelease(release update.Release) bool { return canUpdate(release) && UpdatesConfigured() }
func (v *Service) CheckUpdates(ctx context.Context) error {
	if err := RequireAdmin(ctx); err != nil {
		return err
	}
	if !v.updateMu.TryLock() {
		return store.ErrConflict
	}
	defer v.updateMu.Unlock()
	feed, key := updateTrust()
	if key == "" {
		return ErrUpdateConfig
	}
	checkCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	signed, _, err := update.Fetch(checkCtx, updateClient, feed, key, version.ImageRepository)
	if err != nil {
		return err
	}
	return v.writeUpdate("release.json", signed)
}
func (v *Service) StartUpdate(ctx context.Context, expected string) error {
	if err := RequireAdmin(ctx); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if !UpdatesConfigured() {
		return ErrUpdateConfig
	}
	relative, err := filepath.Rel("/data", v.Store.Dir)
	if err != nil || relative == ".." || strings.HasPrefix(relative, "../") {
		return ErrUpdateConfig
	}
	return v.queueUpdate(ctx, expected)
}

func (v *Service) queueUpdate(ctx context.Context, expected string) error {
	if !v.updateMu.TryLock() {
		return store.ErrConflict
	}
	release, err := v.CheckedRelease()
	if err != nil || release.Version != expected || !canUpdate(release) {
		v.updateMu.Unlock()
		return ErrInvalid
	}
	old, err := v.UpdateState()
	if err != nil || old.Pending() {
		v.updateMu.Unlock()
		return store.ErrConflict
	}
	var signed update.Signed
	if err = v.readUpdate("release.json", &signed); err != nil {
		v.updateMu.Unlock()
		return err
	}
	state := UpdateState{Phase: "checking", Release: signed, FromVersion: version.Current, Started: time.Now().Unix()}
	if err = v.writeUpdate("state.json", state); err != nil {
		v.updateMu.Unlock()
		return err
	}
	// The job outlives the initiating HTTP request, but all network calls remain bounded.
	jobCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Minute)
	v.updateLifecycleMu.Lock()
	if v.updateStopping || ctx.Err() != nil {
		v.updateLifecycleMu.Unlock()
		cancel()
		state.Phase = "failed"
		_ = v.writeUpdate("state.json", state)
		v.updateMu.Unlock()
		return store.ErrConflict
	}
	v.updateCancel = cancel
	v.Background.Add(1)
	go func() {
		defer v.Background.Done()
		defer cancel()
		defer v.updateMu.Unlock()
		defer func() { v.updateLifecycleMu.Lock(); v.updateCancel = nil; v.updateLifecycleMu.Unlock() }()
		v.installUpdate(jobCtx, &state, release)
	}()
	v.updateLifecycleMu.Unlock()
	return nil
}
func (v *Service) installUpdate(ctx context.Context, state *UpdateState, release update.Release) {
	maintenanceSet, remote := false, false
	err := func() error {
		feed, key := updateTrust()
		fresh, _, err := update.Fetch(ctx, updateClient, feed, key, version.ImageRepository)
		if err != nil {
			return err
		}
		if !bytes.Equal(fresh.Manifest, state.Release.Manifest) || !bytes.Equal(fresh.Signature, state.Release.Signature) {
			return store.ErrConflict
		}
		dokploy := dokploySettings()
		app, err := dokploy.Application(ctx, version.ImageRepository)
		if err != nil {
			return err
		}
		state.PreviousImage = app.DockerImage
		v.Operations.Lock()
		err = func() error {
			state.WasMaintenance, err = v.Maintenance()
			if err != nil {
				return err
			}
			if err = v.SetMaintenance(true); err != nil {
				return err
			}
			maintenanceSet = true
			state.Phase = "backing-up"
			if err = v.writeUpdate("state.json", state); err != nil {
				return err
			}
			state.Backup, err = v.CreateBackup()
			if err != nil {
				return err
			}
			path, err := v.BackupPath(state.Backup)
			if err != nil {
				return err
			}
			if err = backup.Verify(path); err != nil {
				return err
			}
			if err = ctx.Err(); err != nil {
				return err
			}
			if err = v.Store.Audit(ctx, "updates.request", release.Version); err != nil {
				return err
			}
			if err = v.writeUpdateMarker(release.Version); err != nil {
				return err
			}
			state.Phase = "deploying"
			return v.writeUpdate("state.json", state)
		}()
		v.Operations.Unlock()
		if err != nil {
			return err
		}
		// Once a remote mutation starts its outcome can be ambiguous; do not reopen writes.
		remote = true
		if err = dokploy.SetImage(ctx, release.Image); err != nil {
			return err
		}
		if err = dokploy.Deploy(ctx, release.Version); err != nil {
			return err
		}
		state.Phase = "requested"
		return v.writeUpdate("state.json", state)
	}()
	if err == nil {
		return
	}
	if remote {
		state.Phase = "uncertain"
	} else {
		state.Phase = "failed"
		if maintenanceSet {
			v.Operations.Lock()
			// Never clear a maintenance state that was already enabled by the operator.
			cleanupErr := v.SetMaintenance(state.WasMaintenance)
			if cleanupErr == nil {
				cleanupErr = os.Remove(filepath.Join(v.Store.Dir, "update-in-progress"))
			}
			if cleanupErr != nil && !errors.Is(cleanupErr, os.ErrNotExist) {
				state.Phase = "uncertain"
			}
			v.Operations.Unlock()
		}
	}
	// The last durable phase and maintenance marker remain authoritative if disk writes fail.
	_ = v.writeUpdate("state.json", state)
}

// CompleteUpdate runs after binding the new listener and before accepting requests or jobs.
func (v *Service) CompleteUpdate() error {
	state, err := v.UpdateState()
	if err != nil {
		return err
	}
	if !state.Pending() {
		return nil
	}
	if state.Phase == "checking" || state.Phase == "backing-up" {
		if version.Current != state.FromVersion {
			return errors.New("unexpected version during interrupted update preparation")
		}
		if state.Phase == "backing-up" {
			if err = v.SetMaintenance(state.WasMaintenance); err != nil {
				return err
			}
			if err = os.Remove(filepath.Join(v.Store.Dir, "update-in-progress")); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
		state.Phase = "failed"
		return v.writeUpdate("state.json", state)
	}
	if !backupID.MatchString(state.Backup) {
		return errors.New("update backup is missing")
	}

	_, key := updateTrust()
	release, err := state.Release.Verify(key, version.ImageRepository)
	if err != nil {
		return err
	}
	if release.Version != version.Current {
		if err = v.SetMaintenance(true); err != nil {
			return err
		}
		return v.writeUpdateMarker(release.Version)
	}
	diagnostics, err := v.Diagnose()
	if err != nil {
		return err
	}
	if diagnostics.Schema != release.Schema {
		return fmt.Errorf("release schema mismatch")
	}
	if err = v.SetMaintenance(state.WasMaintenance); err != nil {
		return err
	}
	if err = os.Remove(filepath.Join(v.Store.Dir, "update-in-progress")); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err = syncDirectory(v.Store.Dir); err != nil {
		return err
	}
	state.Phase = "complete"
	return v.writeUpdate("state.json", state)
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
func (v *Service) writeUpdateMarker(target string) error {
	file, err := os.OpenFile(filepath.Join(v.Store.Dir, "update-in-progress"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	if _, err = file.WriteString(target + "\n"); err != nil {
		file.Close()
		return err
	}
	if err = file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	return syncDirectory(v.Store.Dir)
}

// StopUpdates prevents shutdown from racing a newly accepted installation job.
func (v *Service) StopUpdates() {
	v.updateLifecycleMu.Lock()
	v.updateStopping = true
	cancel := v.updateCancel
	v.updateLifecycleMu.Unlock()
	if cancel != nil {
		cancel()
	}
}
