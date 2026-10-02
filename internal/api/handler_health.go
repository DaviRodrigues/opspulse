package api

import (
	"net/http"

)

func HandleHealth(w http.ResponseWriter, r *http.Request) {
	SendJSON(w, http.StatusOK, map[string]string{
		"status": "OK",
	})
}
