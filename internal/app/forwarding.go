package app

import (
	"net/http"
)

func pausedRequest(w http.ResponseWriter, r *http.Request) (bool, bool) {
	var p struct {
		Paused *bool `json:"paused"`
	}
	if !decode(w, r, &p) {
		return false, false
	}
	if p.Paused == nil {
		fail(w, 400, "请指定 paused: true 或 false")
		return false, false
	}
	return *p.Paused, true
}
