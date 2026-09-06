package boxoff

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// realSample is trimmed from the live https://fotky.kotrzina.cz/metrics
// response, including the neighbouring families whose names share a prefix
// with the one we read.
const realSample = `# HELP kukatko_jobs_queue_depth Number of jobs in the queue, partitioned by state.
# TYPE kukatko_jobs_queue_depth gauge
kukatko_jobs_queue_depth{state="done"} 155145
kukatko_jobs_queue_depth{state="queued"} 12
kukatko_jobs_queue_depth{state="running"} 1
kukatko_jobs_queue_depth_by_type{type="image_embed"} 41640
kukatko_jobs_queue_depth_by_type_state{state="queued",type="image_embed"} 12
kukatko_embedding_service_up 0
`

func TestParseQueueDepth_ReadsQueuedAndRunning(t *testing.T) {
	queued, running, err := ParseQueueDepth(realSample)
	if err != nil {
		t.Fatalf("ParseQueueDepth() error = %v", err)
	}
	if queued != 12 {
		t.Errorf("queued = %d, want 12 (the by_type_state family must not be counted)", queued)
	}
	if running != 1 {
		t.Errorf("running = %d, want 1", running)
	}
}

func TestParseQueueDepth_DoneOnlyIsEmptyQueue(t *testing.T) {
	body := `kukatko_jobs_queue_depth{state="done"} 155145` + "\n"

	queued, running, err := ParseQueueDepth(body)
	if err != nil {
		t.Fatalf("ParseQueueDepth() error = %v", err)
	}
	if queued != 0 || running != 0 {
		t.Errorf("queued=%d running=%d, want 0/0", queued, running)
	}
}

func TestParseQueueDepth_RejectsBodyWithoutTheFamily(t *testing.T) {
	// A renamed metric, an HTML error page or an app that never registered its
	// collectors must not read as "queue empty" — that would license a
	// shutdown on no information at all.
	tests := []struct {
		name string
		body string
	}{
		{name: "empty body", body: ""},
		{name: "html error page", body: "<html><body>502 Bad Gateway</body></html>"},
		{name: "other metrics only", body: "kukatko_http_requests_total{code=\"200\"} 5\n"},
		{name: "only the by_type families", body: "kukatko_jobs_queue_depth_by_type{type=\"ocr\"} 3\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, _, err := ParseQueueDepth(tt.body); err == nil {
				t.Fatal("ParseQueueDepth() = nil error, want a rejection")
			}
		})
	}
}

func TestMetricsClient_Fetch_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte(realSample))
	}))
	defer srv.Close()

	got, err := NewMetricsClient(srv.URL).Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if got.StatusCode != http.StatusOK || got.Queued != 12 || got.Running != 1 {
		t.Errorf("got %+v, want 200/12/1", got)
	}
	if got.Error != "" {
		t.Errorf("Error = %q, want empty", got.Error)
	}
}

func TestMetricsClient_Fetch_HTTPErrorKeepsDetail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("upstream is down"))
	}))
	defer srv.Close()

	got, err := NewMetricsClient(srv.URL).Fetch(context.Background())
	if err == nil {
		t.Fatal("Fetch() = nil error, want failure on 502")
	}
	if got.StatusCode != http.StatusBadGateway {
		t.Errorf("StatusCode = %d, want 502", got.StatusCode)
	}
	if !strings.Contains(got.BodyExcerpt, "upstream is down") {
		t.Errorf("BodyExcerpt = %q, want the response body for the UI", got.BodyExcerpt)
	}
}

func TestMetricsClient_Fetch_TransportErrorKeepsDetail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close() // nothing is listening any more

	got, err := NewMetricsClient(url).Fetch(context.Background())
	if err == nil {
		t.Fatal("Fetch() = nil error, want failure")
	}
	if got.Error == "" {
		t.Error("Error is empty; the transport failure must be reported to the UI")
	}
}

func TestMetricsClient_Fetch_UntrustworthyBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("<html>Just a moment...</html>"))
	}))
	defer srv.Close()

	got, err := NewMetricsClient(srv.URL).Fetch(context.Background())
	if err == nil {
		t.Fatal("Fetch() = nil error, want rejection of a 200 with no metrics in it")
	}
	if got.BodyExcerpt == "" {
		t.Error("BodyExcerpt is empty; the unexpected body must reach the UI")
	}
}
