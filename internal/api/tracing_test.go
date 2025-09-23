package api

import (
	"net/http/httptest"
	"testing"
)

func TestRespondErrorAddsEvent(t *testing.T){
	// build a request with a trace in context
	r := httptest.NewRequest("GET", "/x", nil)
	tc := &Trace{ID: "t1"}
	r = r.WithContext(withTraceCtx(r.Context(), tc))
	rw := httptest.NewRecorder()
	respondError(rw, r, 418, "teapot")
	if rw.Code != 418 { t.Fatalf("expected 418, got %d", rw.Code) }
	if len(tc.Events) == 0 { t.Fatalf("expected an error event") }
	found := false
	for _, ev := range tc.Events {
		if ev.Name == "error" { found = true }
	}
	if !found { t.Fatalf("error event not recorded") }
}
