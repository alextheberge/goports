package ports

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestAPIErrorJSONBodyWhenUnauthorized(t *testing.T) {
	t.Setenv("GOPORTS_API_TOKEN", "test-secret-token")
	addr, shutdown, err := startTestServer()
	if err != nil {
		t.Fatal(err)
	}
	defer shutdown()

	req, err := http.NewRequest(http.MethodGet, "http://"+addr+"/history", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Accept", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", res.StatusCode)
	}
	var body struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != "unauthorized" || body.Error.Message == "" {
		t.Fatalf("unexpected error body: %+v", body)
	}
}

func TestAPIErrorPlainTextWithoutJSONAccept(t *testing.T) {
	t.Setenv("GOPORTS_API_TOKEN", "test-secret-token")
	addr, shutdown, err := startTestServer()
	if err != nil {
		t.Fatal(err)
	}
	defer shutdown()

	res, err := http.Get("http://" + addr + "/history")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", res.StatusCode)
	}
	if ct := res.Header.Get("Content-Type"); ct != "text/plain; charset=utf-8" {
		t.Fatalf("expected text/plain, got %q", ct)
	}
}
