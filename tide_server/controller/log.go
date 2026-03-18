package controller

import (
	"net/http"
	"strconv"
	"tide/tide_server/db"
)

func ListItemStatusLogs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	pageNum := uint(1)
	if raw := q.Get("page_num"); raw != "" {
		n, err := strconv.ParseUint(raw, 10, 64)
		if err != nil || n < 1 {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		pageNum = uint(n)
	}
	pageSize := uint(20)
	if raw := q.Get("page_size"); raw != "" {
		n, err := strconv.ParseUint(raw, 10, 64)
		if err != nil || n < 1 || n > 100 {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		pageSize = uint(n)
	}

	ds, err := db.PagedItemStatusLogs(pageNum, pageSize)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, ds)
}
