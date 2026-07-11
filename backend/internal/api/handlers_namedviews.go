package api

import (
	"net/http"
)

// makeNamedViewsHandler handles GET /named-views.
// It proxies the Onshape namedViews API for the given document/element.
func makeNamedViewsHandler(svc Services) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		docID := q.Get("documentId")
		elemID := q.Get("elementId")

		if docID == "" || elemID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "documentId and elementId are required"})
			return
		}

		client := onshapeClientForReq(svc, r)

		namedViews, err := client.GetNamedViews(r.Context(), docID, elemID, true, false)
		if err != nil {
			svc.Log.Error("failed to fetch named views", "err", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, namedViews)
	}
}
