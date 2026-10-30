package service

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopress/internal/backup"
	"gopress/internal/store"
	"gopress/internal/update"
	"gopress/internal/version"
)

type updateTransport func(*http.Request) (*http.Response, error)

func (f updateTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func updateFixture(t *testing.T, mode string) (*Service, UpdateState, update.Release, *[]string) {
	t.Helper()
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	if err = s.Install(context.Background(), store.Site{Title: "Site", BaseURL: "https://example.com", Timezone: "UTC"}, "owner@example.com", []byte("hash")); err != nil {
		t.Fatal(err)
	}
	public, private, _ := ed25519.GenerateKey(rand.Reader)
	t.Setenv("GOPRESS_UPDATE_PUBLIC_KEY", base64.StdEncoding.EncodeToString(public))
	t.Setenv("GOPRESS_UPDATE_FEED", "https://release.test/update.json")
	t.Setenv("GOPRESS_DOKPLOY_URL", "https://dokploy.test")
	t.Setenv("GOPRESS_DOKPLOY_API_KEY", "private-key")
	t.Setenv("GOPRESS_DOKPLOY_APPLICATION_ID", "site-app")
	release := update.Release{Format: 1, Version: "9.0.0", MinimumVersion: "0.1.0", Schema: 16, Image: version.ImageRepository + "@sha256:" + strings.Repeat("a", 64), Notes: "A test release"}
	if mode == "schema" {
		release.Schema = 17
	}
	raw, _ := json.Marshal(release)
	signed := update.Signed{Manifest: raw, Signature: ed25519.Sign(private, raw)}
	state := UpdateState{Phase: "checking", Release: signed, FromVersion: version.Current}
	feedRaw, feedSignature := raw, signed.Signature
	if mode == "changed" {
		changed := release
		changed.Notes = "A replaced release"
		feedRaw, _ = json.Marshal(changed)
		feedSignature = ed25519.Sign(private, feedRaw)
	}
	if mode == "backup" {
		if err = os.WriteFile(filepath.Join(s.Dir, "backups"), []byte("not a directory"), 0600); err != nil {
			t.Fatal(err)
		}
	}

	v := &Service{Store: s}
	calls := []string{}
	previous := updateClient
	t.Cleanup(func() { v.StopUpdates(); v.Background.Wait(); updateClient = previous })
	updateClient = &http.Client{Transport: updateTransport(func(r *http.Request) (*http.Response, error) {
		calls = append(calls, r.Method+" "+r.URL.Host+r.URL.Path)
		code, body := 200, "true"
		switch r.URL.Host {
		case "release.test":
			body = string(feedRaw)
			if strings.HasSuffix(r.URL.Path, ".sig") {
				body = base64.StdEncoding.EncodeToString(feedSignature)
			}
		case "dokploy.test":
			if r.Header.Get("x-api-key") != "private-key" {
				t.Fatal("credential missing")
			}
			if r.Method == "GET" {
				body = `{"applicationId":"site-app","sourceType":"docker","replicas":1,"dockerImage":"` + version.ImageRepository + `:v0.2.0","env":"GOPRESS_DOKPLOY_APPLICATION_ID=site-app","mounts":[{"type":"volume","volumeName":"site-data","mountPath":"/data"}],"placementSwarm":{"Constraints":["node.hostname==site-node"]}}`
				if mode == "preflight" {
					code = 403
				}
			} else {
				saved, err := v.UpdateState()
				if err != nil || saved.Backup == "" {
					t.Fatal("deploy before backup", err)
				}
				path, err := v.BackupPath(saved.Backup)
				if err != nil || backup.Verify(path) != nil {
					t.Fatal("unverified backup")
				}
				if mode == "remote" {
					code = 500
				}
			}
		default:
			t.Fatal("unexpected destination", r.URL.Host)
		}
		return &http.Response{StatusCode: code, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	if err = v.writeUpdate("release.json", signed); err != nil {
		t.Fatal(err)
	}
	return v, state, release, &calls
}
func TestUpdateDeploysOnlyAfterBackupAndCompletesOnActualNewVersion(t *testing.T) {
	for _, wasMaintenance := range []bool{false, true} {
		t.Run(map[bool]string{false: "public", true: "already-maintained"}[wasMaintenance], func(t *testing.T) {
			v, state, release, calls := updateFixture(t, "")
			if wasMaintenance {
				if err := v.SetMaintenance(true); err != nil {
					t.Fatal(err)
				}
			}
			if err := v.writeUpdate("state.json", state); err != nil {
				t.Fatal(err)
			}
			v.installUpdate(context.Background(), &state, release)
			saved, err := v.UpdateState()
			if err != nil || saved.Phase != "requested" || !backupID.MatchString(saved.Backup) {
				t.Fatal(saved, err, *calls)
			}
			if err = v.CompleteUpdate(); err != nil {
				t.Fatal(err)
			}
			if active, _ := v.Maintenance(); !active {
				t.Fatal("old version reopened writes")
			}
			oldVersion := version.Current
			version.Current = release.Version
			t.Cleanup(func() { version.Current = oldVersion })
			if err = v.CompleteUpdate(); err != nil {
				t.Fatal(err)
			}
			saved, _ = v.UpdateState()
			if saved.Phase != "complete" {
				t.Fatal(saved)
			}
			if active, _ := v.Maintenance(); active != wasMaintenance {
				t.Fatal("operator maintenance preference lost")
			}
			if _, err = os.Stat(filepath.Join(v.Store.Dir, "update-in-progress")); !os.IsNotExist(err) {
				t.Fatal(err)
			}
		})
	}
}
func TestUpdateFailuresKeepRemoteOutcomesClosed(t *testing.T) {
	for _, mode := range []string{"preflight", "changed", "backup", "remote", "schema"} {
		t.Run(mode, func(t *testing.T) {
			v, state, release, calls := updateFixture(t, mode)
			v.installUpdate(context.Background(), &state, release)
			saved, err := v.UpdateState()
			if err != nil {
				t.Fatal(err)
			}
			if mode == "preflight" || mode == "changed" || mode == "backup" {
				if saved.Phase != "failed" {
					t.Fatal(saved)
				}
				if active, _ := v.Maintenance(); active {
					t.Fatal("preflight enabled maintenance")
				}
				for _, call := range *calls {
					if strings.HasPrefix(call, "POST") {
						t.Fatal("mutated Dokploy after preflight failed")
					}
				}
			} else if mode == "remote" {
				if saved.Phase != "uncertain" {
					t.Fatal(saved)
				}
				if active, _ := v.Maintenance(); !active {
					t.Fatal("ambiguous deployment reopened writes")
				}
			} else {
				previous := version.Current
				version.Current = release.Version
				t.Cleanup(func() { version.Current = previous })
				if err = v.CompleteUpdate(); err == nil {
					t.Fatal("accepted wrong schema")
				}
				if active, _ := v.Maintenance(); !active {
					t.Fatal("schema mismatch reopened writes")
				}
			}
		})
	}
}
func TestUpdateRejectsUnauthorizedOrNativeInstall(t *testing.T) {
	v, _, _, _ := updateFixture(t, "")
	ctx := store.WithActor(context.Background(), store.User{ID: 2, Role: "author"})
	if err := v.CheckUpdates(ctx); err != ErrForbidden {
		t.Fatal(err)
	}
	if err := v.StartUpdate(ctx, "9.0.0"); err != ErrForbidden {
		t.Fatal(err)
	}
	if err := v.StartUpdate(context.Background(), "9.0.0"); err != ErrUpdateConfig {
		t.Fatal(err)
	}
}

func TestUpdateInterruptedPreparationDoesNotRedeploy(t *testing.T) {
	v, state, _, calls := updateFixture(t, "")
	state.Phase = "backing-up"
	state.WasMaintenance = false
	if err := v.writeUpdate("state.json", state); err != nil {
		t.Fatal(err)
	}
	if err := v.SetMaintenance(true); err != nil {
		t.Fatal(err)
	}
	if err := v.writeUpdateMarker("9.0.0"); err != nil {
		t.Fatal(err)
	}
	// Removing the ordinary flag cannot reopen background writes during an update.
	if err := v.SetMaintenance(false); err != nil {
		t.Fatal(err)
	}
	if active, _ := v.Maintenance(); !active {
		t.Fatal("pending marker did not enforce maintenance")
	}
	if err := v.CompleteUpdate(); err != nil {
		t.Fatal(err)
	}
	saved, _ := v.UpdateState()
	if saved.Phase != "failed" || len(*calls) != 0 {
		t.Fatal(saved, *calls)
	}
	if active, _ := v.Maintenance(); active {
		t.Fatal("safe interrupted preparation remained locked")
	}
}

func TestUpdateQueuePreventsDuplicateDeployments(t *testing.T) {
	v, _, release, calls := updateFixture(t, "")
	if err := v.queueUpdate(context.Background(), release.Version); err != nil {
		t.Fatal(err)
	}
	if err := v.queueUpdate(context.Background(), release.Version); err != store.ErrConflict {
		t.Fatal("accepted duplicate update", err)
	}
	v.Background.Wait()
	state, err := v.UpdateState()
	if err != nil || state.Phase != "requested" {
		t.Fatal(state, err)
	}
	deployments := 0
	for _, call := range *calls {
		if call == "POST dokploy.test/api/application.deploy" {
			deployments++
		}
	}
	if deployments != 1 {
		t.Fatal("deployment repeated", *calls)
	}
}
func TestUpdateShutdownCancelsPreparationAndRejectsNewJobs(t *testing.T) {
	v, _, release, _ := updateFixture(t, "")
	entered := make(chan struct{})
	updateClient.Transport = updateTransport(func(r *http.Request) (*http.Response, error) {
		close(entered)
		<-r.Context().Done()
		return nil, r.Context().Err()
	})
	if err := v.queueUpdate(context.Background(), release.Version); err != nil {
		t.Fatal(err)
	}
	<-entered
	v.StopUpdates()
	v.Background.Wait()
	state, err := v.UpdateState()
	if err != nil || state.Phase != "failed" {
		t.Fatal(state, err)
	}
	if err = v.queueUpdate(context.Background(), release.Version); err != store.ErrConflict {
		t.Fatal("accepted update during shutdown", err)
	}
}

func TestPendingUpdateRecreatesMissingMaintenanceMarkersOnOldVersion(t *testing.T) {
	v, state, release, _ := updateFixture(t, "")
	v.installUpdate(context.Background(), &state, release)
	for _, name := range []string{"maintenance", "update-in-progress"} {
		if err := os.Remove(filepath.Join(v.Store.Dir, name)); err != nil {
			t.Fatal(err)
		}
	}
	if err := v.CompleteUpdate(); err != nil {
		t.Fatal(err)
	}
	if active, _ := v.Maintenance(); !active {
		t.Fatal("old version reopened pending update")
	}
}
