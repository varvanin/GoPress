package update

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
)

var ErrDokploy = errors.New("Dokploy request failed")
var nodePin = regexp.MustCompile(`^node\.(?:id|hostname)==[a-zA-Z0-9][a-zA-Z0-9._-]{0,127}$`)

type Dokploy struct {
	URL, Key, ApplicationID string
	Client                  *http.Client
}
type Application struct {
	ApplicationID  string   `json:"applicationId"`
	SourceType     string   `json:"sourceType"`
	DockerImage    string   `json:"dockerImage"`
	Command        string   `json:"command"`
	Args           []string `json:"args"`
	Env            string   `json:"env"`
	Replicas       int      `json:"replicas"`
	PlacementSwarm struct {
		Constraints []string `json:"Constraints"`
	} `json:"placementSwarm"`
	ModeSwarm map[string]json.RawMessage `json:"modeSwarm"`
	Mounts    []struct {
		MountPath  string `json:"mountPath"`
		Type       string `json:"type"`
		VolumeName string `json:"volumeName"`
		HostPath   string `json:"hostPath"`
	} `json:"mounts"`
}

func (d Dokploy) Configured() bool {
	u, err := url.Parse(d.URL)
	return err == nil && HTTPS(d.URL) && u.Path == "" && u.RawQuery == "" && d.Key != "" && len(d.Key) <= 4096 && !strings.ContainsAny(d.Key, "\r\n") && len(d.ApplicationID) > 0 && len(d.ApplicationID) <= 128 && !strings.ContainsAny(d.ApplicationID, "\r\n")
}
func (d Dokploy) request(ctx context.Context, method, path string, payload any) ([]byte, error) {
	if !d.Configured() {
		return nil, ErrDokploy
	}
	var body []byte
	if payload != nil {
		var err error
		body, err = json.Marshal(payload)
		if err != nil {
			return nil, ErrDokploy
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, d.URL+"/api/"+path, bytes.NewReader(body))
	if err != nil {
		return nil, ErrDokploy
	}
	req.Header.Set("x-api-key", d.Key)
	req.Header.Set("Content-Type", "application/json")
	// Credentials must never follow redirects to another origin.
	client := *d.Client
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := client.Do(req)
	if err != nil {
		return nil, ErrDokploy
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, ErrDokploy
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil || len(raw) > 1<<20 {
		return nil, ErrDokploy
	}
	return raw, nil
}
func (d Dokploy) Application(ctx context.Context, repository string) (Application, error) {
	var a Application
	raw, err := d.request(ctx, "GET", "application.one?applicationId="+url.QueryEscape(d.ApplicationID), nil)
	if err != nil || json.Unmarshal(raw, &a) != nil {
		return a, ErrDokploy
	}
	if a.ApplicationID != d.ApplicationID || a.SourceType != "docker" || a.Replicas != 1 || !(strings.HasPrefix(a.DockerImage, repository+":") || strings.HasPrefix(a.DockerImage, repository+"@sha256:")) {
		return a, ErrDokploy
	}
	if strings.TrimSpace(a.Command) != "" {
		return a, ErrDokploy
	}
	if len(a.Args) > 0 && !slices.Equal(a.Args, []string{"serve", "--data", "/data", "--addr", "0.0.0.0:8080"}) {
		return a, ErrDokploy
	}
	// Verify this is the application's own configuration, not another site sharing a key.
	found := false
	for _, line := range strings.Split(a.Env, "\n") {
		if strings.TrimSpace(line) == "GOPRESS_DOKPLOY_APPLICATION_ID="+d.ApplicationID {
			found = true
		}
	}
	mounted := false
	for _, m := range a.Mounts {
		if m.MountPath == "/data" && (m.Type == "volume" && m.VolumeName != "" || m.Type == "bind" && m.HostPath != "") {
			mounted = true
		}
	}
	pinned := false
	for _, constraint := range a.PlacementSwarm.Constraints {
		value := strings.ReplaceAll(constraint, " ", "")
		if nodePin.MatchString(value) {
			pinned = true
		}
	}
	if len(a.ModeSwarm) > 0 {
		replicated, ok := a.ModeSwarm["Replicated"]
		var mode struct {
			Replicas *int `json:"Replicas"`
		}
		if !ok || len(a.ModeSwarm) != 1 || json.Unmarshal(replicated, &mode) != nil || mode.Replicas != nil && *mode.Replicas != 1 {
			return a, ErrDokploy
		}
	}
	if !found || !mounted || !pinned {
		return a, ErrDokploy
	}
	return a, nil
}
func (d Dokploy) SetImage(ctx context.Context, image string) error {
	_, err := d.request(ctx, "POST", "application.update", map[string]any{
		"applicationId": d.ApplicationID, "dockerImage": image, "replicas": 1, "modeSwarm": map[string]any{"Replicated": map[string]int{"Replicas": 1}},
		"rollbackActive": false, "autoDeploy": false,
		"updateConfigSwarm": map[string]any{"Parallelism": 1, "Order": "stop-first", "FailureAction": "pause"},
		"healthCheckSwarm":  map[string]any{"Test": []string{"CMD", "/gopress", "healthcheck", "--addr", "127.0.0.1:8080", "--installed"}, "Interval": 10000000000, "Timeout": 3000000000, "StartPeriod": 300000000000, "Retries": 3},
	})
	return err
}
func (d Dokploy) Deploy(ctx context.Context, version string) error {
	_, err := d.request(ctx, "POST", "application.deploy", map[string]string{"applicationId": d.ApplicationID, "title": "GoPress " + version})
	return err
}
