package token

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"token-router/app/logic/allocator"
	"token-router/repository/store"
)

func newTestServer(n int, m int64) (*httptest.Server, allocator.Allocator) {
	gin.SetMode(gin.TestMode)
	a := allocator.New(store.NewMemoryStore(n, m))
	r := gin.New()
	h := New(a)
	r.POST("/alloc", h.AllocHandler)
	r.POST("/free", h.FreeHandler)
	r.GET("/debug/status", h.StatusHandler)
	return httptest.NewServer(r), a
}

func doJSON(t *testing.T, srv *httptest.Server, method, path string, body any) (*http.Response, map[string]any) {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	req, _ := http.NewRequest(method, srv.URL+path, &buf)
	req.Header.Set("Content-Type", "application/json")
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp, out
}

// TestHTTP_ExampleScript runs the exact 10-step example from the prompt and
// asserts the response shape matches the spec verbatim.
func TestHTTP_ExampleScript(t *testing.T) {
	srv, a := newTestServer(2, 300)
	defer srv.Close()

	type want struct {
		status       int
		hasNodeID    bool
		hasRemaining bool
		errorBody    string
	}
	steps := []struct {
		desc, method, path string
		body               any
		want               want
	}{
		{"alloc req-1 80", "POST", "/alloc", map[string]any{"request_id": "req-1", "token_count": 80}, want{200, true, true, ""}},
		{"alloc req-2 120", "POST", "/alloc", map[string]any{"request_id": "req-2", "token_count": 120}, want{200, true, true, ""}},
		{"free req-1", "POST", "/free", map[string]any{"request_id": "req-1"}, want{200, true, false, ""}},
		{"alloc req-3 200", "POST", "/alloc", map[string]any{"request_id": "req-3", "token_count": 200}, want{200, true, true, ""}},
		{"free req-2", "POST", "/free", map[string]any{"request_id": "req-2"}, want{200, true, false, ""}},
		{"alloc req-4 300", "POST", "/alloc", map[string]any{"request_id": "req-4", "token_count": 300}, want{200, true, true, ""}},
		{"free req-3", "POST", "/free", map[string]any{"request_id": "req-3"}, want{200, true, false, ""}},
		{"alloc req-5 250", "POST", "/alloc", map[string]any{"request_id": "req-5", "token_count": 250}, want{200, true, true, ""}},
		{"free req-4", "POST", "/free", map[string]any{"request_id": "req-4"}, want{200, true, false, ""}},
		{"free req-5", "POST", "/free", map[string]any{"request_id": "req-5"}, want{200, true, false, ""}},
	}
	for _, s := range steps {
		resp, body := doJSON(t, srv, s.method, s.path, s.body)
		if resp.StatusCode != s.want.status {
			t.Errorf("%s: status=%d want=%d body=%v", s.desc, resp.StatusCode, s.want.status, body)
		}
		if s.want.errorBody == "" {
			if _, ok := body["node_id"]; ok != s.want.hasNodeID {
				t.Errorf("%s: node_id presence=%v want=%v body=%v", s.desc, ok, s.want.hasNodeID, body)
			}
			if _, ok := body["remaining_quota"]; ok != s.want.hasRemaining {
				t.Errorf("%s: remaining_quota presence=%v want=%v body=%v", s.desc, ok, s.want.hasRemaining, body)
			}
			// No wrapper fields should leak into the prompt contract.
			for _, k := range []string{"errno", "errmsg", "data", "duplicate"} {
				if _, ok := body[k]; ok {
					t.Errorf("%s: response contains forbidden key %q (body=%v)", s.desc, k, body)
				}
			}
		}
	}
	st := a.Status()
	for _, n := range st.Nodes {
		if n.Remaining != 300 {
			t.Errorf("final node %d remaining %d, want 300", n.ID, n.Remaining)
		}
	}
	if st.InFlight != 0 {
		t.Errorf("final in-flight %d, want 0", st.InFlight)
	}
}

func TestHTTP_AllocSuccessShape(t *testing.T) {
	srv, _ := newTestServer(1, 100)
	defer srv.Close()

	resp, body := doJSON(t, srv, "POST", "/alloc", map[string]any{"request_id": "r1", "token_count": 50})
	if resp.StatusCode != 200 {
		t.Fatalf("status=%d body=%v", resp.StatusCode, body)
	}
	if int(body["node_id"].(float64)) != 0 {
		t.Errorf("node_id=%v", body["node_id"])
	}
	if int(body["remaining_quota"].(float64)) != 50 {
		t.Errorf("remaining_quota=%v", body["remaining_quota"])
	}
}

func TestHTTP_Overloaded(t *testing.T) {
	srv, _ := newTestServer(1, 100)
	defer srv.Close()

	if resp, _ := doJSON(t, srv, "POST", "/alloc", map[string]any{"request_id": "a", "token_count": 100}); resp.StatusCode != 200 {
		t.Fatalf("alloc1: %d", resp.StatusCode)
	}
	resp, body := doJSON(t, srv, "POST", "/alloc", map[string]any{"request_id": "b", "token_count": 1})
	if resp.StatusCode != 429 {
		t.Fatalf("expected 429, got %d body=%v", resp.StatusCode, body)
	}
	if body["error"] != "overloaded" {
		t.Errorf("body.error=%v want=overloaded", body["error"])
	}
}

func TestHTTP_FreeSuccessAndNotFound(t *testing.T) {
	srv, _ := newTestServer(2, 100)
	defer srv.Close()

	_, _ = doJSON(t, srv, "POST", "/alloc", map[string]any{"request_id": "x", "token_count": 30})
	resp, body := doJSON(t, srv, "POST", "/free", map[string]any{"request_id": "x"})
	if resp.StatusCode != 200 {
		t.Fatalf("free status=%d body=%v", resp.StatusCode, body)
	}
	if _, ok := body["node_id"]; !ok {
		t.Errorf("free body missing node_id: %v", body)
	}

	resp, body = doJSON(t, srv, "POST", "/free", map[string]any{"request_id": "ghost"})
	if resp.StatusCode != 404 || body["error"] != "not_found" {
		t.Fatalf("ghost free: status=%d body=%v", resp.StatusCode, body)
	}
}

func TestHTTP_DuplicateViaHeader(t *testing.T) {
	srv, _ := newTestServer(2, 100)
	defer srv.Close()

	resp, body := doJSON(t, srv, "POST", "/alloc", map[string]any{"request_id": "x", "token_count": 30})
	if resp.StatusCode != 200 {
		t.Fatalf("first alloc: %d body=%v", resp.StatusCode, body)
	}
	if got := resp.Header.Get(HeaderDuplicate); got != "" {
		t.Errorf("first alloc should not set duplicate header, got %q", got)
	}

	resp, body = doJSON(t, srv, "POST", "/alloc", map[string]any{"request_id": "x", "token_count": 30})
	if resp.StatusCode != 200 {
		t.Fatalf("dup alloc: %d body=%v", resp.StatusCode, body)
	}
	if got := resp.Header.Get(HeaderDuplicate); got != "true" {
		t.Errorf("dup alloc missing %s header, got %q", HeaderDuplicate, got)
	}
	// Body must remain strictly compliant — no `duplicate` field leaking.
	if _, ok := body["duplicate"]; ok {
		t.Errorf("dup alloc body should not expose duplicate field: %v", body)
	}
}

func TestHTTP_InvalidPayload(t *testing.T) {
	srv, _ := newTestServer(1, 100)
	defer srv.Close()

	cases := []map[string]any{
		{"request_id": "", "token_count": 10},   // empty id
		{"request_id": "x"},                     // missing token_count
		{"request_id": "x", "token_count": 200}, // > budget
		{"request_id": "x", "token_count": -1},  // negative
	}
	for i, c := range cases {
		resp, body := doJSON(t, srv, "POST", "/alloc", c)
		if resp.StatusCode != 400 {
			t.Errorf("case %d: status=%d want=400 body=%v", i, resp.StatusCode, body)
		}
		if body["error"] != "invalid_request" {
			t.Errorf("case %d: body.error=%v want=invalid_request", i, body["error"])
		}
	}
}

func TestHTTP_DebugStatus(t *testing.T) {
	srv, _ := newTestServer(2, 100)
	defer srv.Close()

	_, _ = doJSON(t, srv, "POST", "/alloc", map[string]any{"request_id": "s1", "token_count": 40})
	resp, body := doJSON(t, srv, "GET", "/debug/status", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("status code=%d", resp.StatusCode)
	}
	if int(body["in_flight"].(float64)) != 1 {
		t.Errorf("in_flight=%v body=%v", body["in_flight"], body)
	}
}
