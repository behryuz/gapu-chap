package http

import (
	"encoding/json"
	"fmt"
	"net/http"
)

const maxBodyBytes = 1 << 20 // 1MB

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	if err := dec.Decode(dst); err != nil {
		return fmt.Errorf("decoding request: %w", err)
	}
	return nil
}
