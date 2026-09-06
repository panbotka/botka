package boxoff

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	// metricsTimeout bounds one scrape of the Kukátko metrics endpoint.
	metricsTimeout = 10 * time.Second
	// metricsBodyLimit caps how much of the response is read. The live
	// endpoint is ~100 kB; anything an order of magnitude larger is a sign
	// something else is answering.
	metricsBodyLimit = 4 << 20
	// bodyExcerptLimit is how much of an unexpected body is kept for the UI.
	bodyExcerptLimit = 200
	// queueDepthPrefix matches only the by-state family. The trailing brace
	// matters: kukatko_jobs_queue_depth_by_type and
	// kukatko_jobs_queue_depth_by_type_state share the metric-name prefix and
	// would otherwise be double-counted.
	queueDepthPrefix = "kukatko_jobs_queue_depth{"
)

// MetricsClient reads Kukátko's job queue depth from its Prometheus endpoint.
// The endpoint is unauthenticated and served outside Kukátko's /api/v1 tree.
type MetricsClient struct {
	url    string
	client *http.Client
}

// NewMetricsClient returns a client scraping the given metrics URL.
func NewMetricsClient(url string) *MetricsClient {
	return &MetricsClient{
		url:    url,
		client: &http.Client{Timeout: metricsTimeout},
	}
}

// Fetch scrapes the endpoint and returns the queue depth.
//
// The returned readings are filled in even when the error is non-nil — status
// code, transport error and a slice of an unexpected body are what the UI
// shows to explain why no shutdown happened. A non-nil error always means
// "the queue state is unknown", which callers must treat as a blocker.
func (c *MetricsClient) Fetch(ctx context.Context) (KukatkoReadings, error) {
	var out KukatkoReadings

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url, nil)
	if err != nil {
		out.Error = err.Error()
		return out, fmt.Errorf("kukatko metrics request: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		out.Error = err.Error()
		return out, fmt.Errorf("kukatko metrics: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	out.StatusCode = resp.StatusCode

	body, err := io.ReadAll(io.LimitReader(resp.Body, metricsBodyLimit))
	if err != nil {
		out.Error = err.Error()
		return out, fmt.Errorf("kukatko metrics body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		out.BodyExcerpt = excerpt(string(body))
		return out, fmt.Errorf("kukatko metrics: HTTP %d", resp.StatusCode)
	}

	queued, running, err := ParseQueueDepth(string(body))
	if err != nil {
		out.Error = err.Error()
		out.BodyExcerpt = excerpt(string(body))
		return out, fmt.Errorf("kukatko metrics: %w", err)
	}

	out.Queued = queued
	out.Running = running
	return out, nil
}

// ParseQueueDepth sums the queued and running job counts out of a Prometheus
// exposition body.
//
// It refuses a body that contains no kukatko_jobs_queue_depth sample at all.
// Without that guard a renamed metric, a captive-portal page or an app that
// never registered its collectors would all parse as "queue empty" and
// license a shutdown on no information.
func ParseQueueDepth(body string) (queued, running int, err error) {
	found := false

	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, queueDepthPrefix) {
			continue
		}
		found = true

		state, value, err := parseQueueDepthSample(line)
		if err != nil {
			return 0, 0, err
		}
		switch state {
		case "queued":
			queued += value
		case "running":
			running += value
		}
	}

	if !found {
		return 0, 0, errors.New("no kukatko_jobs_queue_depth samples in response")
	}
	return queued, running, nil
}

// parseQueueDepthSample splits one exposition line into its state label and
// value: `kukatko_jobs_queue_depth{state="queued"} 12`.
func parseQueueDepthSample(line string) (state string, value int, err error) {
	closing := strings.Index(line, "}")
	if closing < 0 {
		return "", 0, fmt.Errorf("malformed sample %q", line)
	}

	labels := line[len(queueDepthPrefix):closing]
	for _, pair := range strings.Split(labels, ",") {
		name, val, ok := strings.Cut(strings.TrimSpace(pair), "=")
		if ok && name == "state" {
			state = strings.Trim(val, `"`)
		}
	}
	if state == "" {
		return "", 0, fmt.Errorf("sample without a state label: %q", line)
	}

	raw := strings.TrimSpace(line[closing+1:])
	if raw == "" {
		return "", 0, fmt.Errorf("sample without a value: %q", line)
	}
	// Prometheus gauges are floats on the wire ("12" or "12.0"); job counts
	// are whole numbers, so truncation is exact.
	f, err := strconv.ParseFloat(strings.Fields(raw)[0], 64)
	if err != nil {
		return "", 0, fmt.Errorf("sample value %q: %w", raw, err)
	}
	return state, int(f), nil
}

// excerpt shortens an unexpected response body to something a UI can show.
func excerpt(body string) string {
	body = strings.TrimSpace(body)
	if len(body) <= bodyExcerptLimit {
		return body
	}
	return body[:bodyExcerptLimit] + "…"
}
