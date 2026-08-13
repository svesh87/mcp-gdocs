package google

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// TestRetriesQuota is the case that turns up the moment a whole deck is built: Google
// meters writes per minute, the twentieth slide hits the limit, and without a retry the
// document is left half-built.
func TestRetriesQuota(t *testing.T) {
	var calls atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error": {"code": 429, "status": "RESOURCE_EXHAUSTED",
			                        "message": "Quota exceeded for quota metric 'Write requests'"}}`))
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"presentationId": "deck", "replies": [{}]}`))
	}))
	t.Cleanup(server.Close)

	client := New(server.Client(), WithBaseURL(server.URL))

	// The first attempt is refused on quota, the second goes through, and the caller sees
	// a success rather than a half-written deck.
	response, err := client.SlidesBatchUpdate(context.Background(), "deck", []Request{{
		InsertText: &InsertTextRequest{ObjectID: "body1", Text: "Текст"},
	}})
	if err != nil {
		t.Fatalf("a quota refusal should be retried: %v", err)
	}
	if len(response.Replies) != 1 {
		t.Errorf("the answer came back as %+v", response)
	}
	if calls.Load() != 2 {
		t.Errorf("expected one retry, got %d calls", calls.Load())
	}
}

func TestDoesNotRetryRefusals(t *testing.T) {
	var calls atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error": {"code": 403, "status": "PERMISSION_DENIED", "message": "no"}}`))
	}))
	t.Cleanup(server.Close)

	client := New(server.Client(), WithBaseURL(server.URL))

	// A permission refusal will be refused again: repeating it wastes the caller's time
	// and Google's quota.
	if _, err := client.Presentation(context.Background(), "deck", ""); err == nil {
		t.Fatal("a 403 should be an error")
	}
	if calls.Load() != 1 {
		t.Errorf("a 403 should not be retried, got %d calls", calls.Load())
	}
}

func TestRetryGivesUpWithTheReason(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error": {"code": 429, "status": "RESOURCE_EXHAUSTED", "message": "slow down"}}`))
	}))
	t.Cleanup(server.Close)

	client := New(server.Client(), WithBaseURL(server.URL))

	ctx, cancel := context.WithCancel(context.Background())
	// Cancelling stands in for the caller giving up: the wait between attempts has to
	// respect it rather than sleeping through a cancelled call.
	cancel()

	_, err := client.Presentation(ctx, "deck", "")
	if err == nil {
		t.Fatal("a cancelled call should fail")
	}
	if !strings.Contains(err.Error(), "context canceled") && !strings.Contains(err.Error(), "429") {
		t.Errorf("the error should say what happened: %v", err)
	}
}
