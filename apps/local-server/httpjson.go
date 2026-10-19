package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

// writeJSON emits the one JSON shape every /desktop route answers with.
func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("content-type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(body)
}

// readStrictJSON parses a bounded JSON object body or reports why it cannot.
func readStrictJSON(r *http.Request, target any) error {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1024*1024+1))
	if err != nil || len(body) == 0 || len(body) > 1024*1024 {
		return errors.New("invalid body")
	}
	return json.Unmarshal(body, target)
}
