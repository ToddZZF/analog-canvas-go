package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// postQuit sends a /desktop/quit request with the given method and origin
// and reports the response status without reading a body.
func postQuit(handler *quitHandler, method string, origin string) int {
	request, err := http.NewRequest(method, "http://desktop/quit", bytes.NewReader(nil))
	if err != nil {
		panic(err)
	}
	request.Header.Set("origin", origin)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder.Code
}

func TestQuitHandler(t *testing.T) {
	handler := newQuitHandler([]string{"http://127.0.0.1:4173"})

	// GET is not a quit.
	if status := postQuit(handler, http.MethodGet, "http://127.0.0.1:4173"); status != http.StatusMethodNotAllowed {
		t.Fatalf("GET status %d, want 405", status)
	}

	// Other origins cannot stop the server.
	if status := postQuit(handler, http.MethodPost, "https://evil.example"); status != http.StatusForbidden {
		t.Fatalf("wrong origin status %d, want 403", status)
	}
	select {
	case <-handler.request:
		t.Fatal("a rejected request must not quit the server")
	default:
	}

	// The served origin quits and answers before shutdown.
	if status := postQuit(handler, http.MethodPost, "http://127.0.0.1:4173"); status != http.StatusOK {
		t.Fatalf("quit status %d, want 200", status)
	}
	select {
	case <-handler.request:
	case <-time.After(time.Second):
		t.Fatal("an accepted request must quit the server")
	}
}

func TestQuitRouteReachesHandler(t *testing.T) {
	test := newTestServer(t)
	quit := newQuitHandler(nil)
	mux := test.server.Config.Handler.(*localServer)
	mux.quit = quit
	quit.origins[test.server.URL] = true

	outcome := test.post(t, "/desktop/quit", nil)
	if outcome["status"] != "quitting" {
		t.Fatalf("quit outcome %v, want quitting", outcome["status"])
	}
	select {
	case <-quit.request:
	default:
		t.Fatal("the routed quit did not reach its handler")
	}

	// Nearby unknown /desktop routes stay not found.
	response, err := test.client.Post(test.server.URL+"/desktop/quit-all", "application/json", bytes.NewReader(nil))
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("/desktop/quit-all status %d, want 404", response.StatusCode)
	}
}
