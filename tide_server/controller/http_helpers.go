package controller

import (
	"encoding/json"
	"net/http"
)

func writeJSON(w http.ResponseWriter, code int, obj any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if obj == nil {
		return
	}
	_ = json.NewEncoder(w).Encode(obj)
}

func writeOK(w http.ResponseWriter) {
	_, _ = w.Write([]byte("ok"))
}

func readJSON(r *http.Request, dst any) error {
	decoder := json.NewDecoder(r.Body)
	return decoder.Decode(dst)
}

func readJSONOrBadRequest(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := readJSON(r, dst); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return false
	}
	return true
}
