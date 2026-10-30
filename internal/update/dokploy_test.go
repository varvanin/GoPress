package update

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func applicationJSON() map[string]any {
	return map[string]any{"applicationId": "site-app", "sourceType": "docker", "replicas": 1, "dockerImage": "ghcr.io/varvanin/gopress:v0.2.0", "env": "GOPRESS_DOKPLOY_APPLICATION_ID=site-app\nGOPRESS_DOKPLOY_API_KEY=secret", "mounts": []map[string]string{{"type": "volume", "volumeName": "site-data", "mountPath": "/data"}}, "placementSwarm": map[string]any{"Constraints": []string{"node.hostname==site-node"}}}
}
func TestDokployPinsDigestAndSafeDeploymentPolicy(t *testing.T) {
	calls := []string{}
	client := Client(time.Second)
	client.Transport = transport(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("x-api-key") != "secret" {
			t.Fatal("credential missing")
		}
		calls = append(calls, r.URL.Path)
		response := "true"
		if r.Method == "GET" {
			body, _ := json.Marshal(applicationJSON())
			response = string(body)
		} else {
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if r.URL.Path == "/api/application.update" {
				policy := body["updateConfigSwarm"].(map[string]any)
				if policy["Order"] != "stop-first" || policy["FailureAction"] != "pause" || body["rollbackActive"] != false || body["replicas"] != float64(1) || !strings.Contains(body["dockerImage"].(string), "@sha256:") {
					t.Fatal(body)
				}
			}
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(response))}, nil
	})
	d := Dokploy{URL: "https://dokploy.test", Key: "secret", ApplicationID: "site-app", Client: client}
	if _, err := d.Application(context.Background(), "ghcr.io/varvanin/gopress"); err != nil {
		t.Fatal(err)
	}
	if err := d.SetImage(context.Background(), "ghcr.io/varvanin/gopress@sha256:"+strings.Repeat("a", 64)); err != nil {
		t.Fatal(err)
	}
	if err := d.Deploy(context.Background(), "1.0.0"); err != nil {
		t.Fatal(err)
	}
	if strings.Join(calls, ",") != "/api/application.one,/api/application.update,/api/application.deploy" {
		t.Fatal(calls)
	}
}
func TestDokployRejectsWrongApplicationAndCredentialRedirect(t *testing.T) {
	for _, mutation := range []func(map[string]any){func(a map[string]any) { a["sourceType"] = "github" }, func(a map[string]any) { a["command"] = "/other" }, func(a map[string]any) { a["args"] = []string{"serve", "--addr", "0.0.0.0:8081"} }, func(a map[string]any) { a["applicationId"] = "other" }, func(a map[string]any) { a["env"] = "GOPRESS_DOKPLOY_APPLICATION_ID=other" }, func(a map[string]any) { a["replicas"] = 2 }, func(a map[string]any) { a["mounts"] = nil }, func(a map[string]any) { a["placementSwarm"] = nil }, func(a map[string]any) { a["modeSwarm"] = map[string]any{"Global": map[string]any{}} }} {
		a := applicationJSON()
		mutation(a)
		body, _ := json.Marshal(a)
		client := Client(time.Second)
		client.Transport = transport(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(body)))}, nil
		})
		d := Dokploy{URL: "https://dokploy.test", Key: "secret", ApplicationID: "site-app", Client: client}
		if _, err := d.Application(context.Background(), "ghcr.io/varvanin/gopress"); err == nil {
			t.Fatal("accepted unsafe target", a)
		}
	}
	client := Client(time.Second)
	calls := 0
	client.Transport = transport(func(*http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 302, Header: http.Header{"Location": []string{"https://other.test/secret"}}, Body: io.NopCloser(strings.NewReader("secret provider body"))}, nil
	})
	d := Dokploy{URL: "https://dokploy.test", Key: "secret", ApplicationID: "site-app", Client: client}
	if err := d.Deploy(context.Background(), "1.0.0"); err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatal("followed credential redirect")
	}
}
