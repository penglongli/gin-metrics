package ginmetrics

import (
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/penglongli/gin-metrics/bloom"
)

var (
	metricRequestTotal    = "gin_request_total"
	metricRequestUVTotal  = "gin_request_uv_total"
	metricURIRequestTotal = "gin_uri_request_total"
	metricRequestBody     = "gin_request_body_total"
	metricResponseBody    = "gin_response_body_total"
	metricRequestDuration = "gin_request_duration"
	metricSlowRequest     = "gin_slow_request_total"

	bloomFilter *bloom.BloomFilter
)

// Use set gin metrics middleware
func (m *Monitor) Use(r gin.IRoutes) {
	m.initGinMetrics()

	r.Use(m.monitorInterceptor)
	r.GET(m.metricPath, func(ctx *gin.Context) {
		promhttp.Handler().ServeHTTP(ctx.Writer, ctx.Request)
	})
}

// UseWithoutExposingEndpoint is used to add monitor interceptor to gin router
// It can be called multiple times to intercept from multiple gin.IRoutes
// http path is not set, to do that use Expose function
func (m *Monitor) UseWithoutExposingEndpoint(r gin.IRoutes) {
	m.initGinMetrics()
	r.Use(m.monitorInterceptor)
}

// Expose adds metric path to a given router.
// The router can be different with the one passed to UseWithoutExposingEndpoint.
// This allows to expose metrics on different port.
func (m *Monitor) Expose(r gin.IRoutes) {
	r.GET(m.metricPath, func(ctx *gin.Context) {
		promhttp.Handler().ServeHTTP(ctx.Writer, ctx.Request)
	})
}

// initGinMetrics used to init gin metrics
func (m *Monitor) initGinMetrics() {
	bloomFilter = bloom.NewBloomFilter()

	_ = monitor.AddMetric(&Metric{
		Type:        Counter,
		Name:        metricRequestTotal,
		Description: "all the server received request num.",
		Labels:      m.getMetricLabelsIncludingMetadata(metricRequestTotal),
	})
	_ = monitor.AddMetric(&Metric{
		Type:        Counter,
		Name:        metricRequestUVTotal,
		Description: "all the server received ip num.",
		Labels:      m.getMetricLabelsIncludingMetadata(metricRequestUVTotal),
	})
	_ = monitor.AddMetric(&Metric{
		Type:        Counter,
		Name:        metricURIRequestTotal,
		Description: "all the server received request num with every uri.",
		Labels:      m.getMetricLabelsIncludingMetadata(metricURIRequestTotal),
	})
	_ = monitor.AddMetric(&Metric{
		Type:        Counter,
		Name:        metricRequestBody,
		Description: "the server received request body size, unit byte",
		Labels:      m.getMetricLabelsIncludingMetadata(metricRequestBody),
	})
	_ = monitor.AddMetric(&Metric{
		Type:        Counter,
		Name:        metricResponseBody,
		Description: "the server send response body size, unit byte",
		Labels:      m.getMetricLabelsIncludingMetadata(metricResponseBody),
	})
	_ = monitor.AddMetric(&Metric{
		Type:        Histogram,
		Name:        metricRequestDuration,
		Description: "the time server took to handle the request.",
		Labels:      m.getMetricLabelsIncludingMetadata(metricRequestDuration),
		Buckets:     m.reqDuration,
	})
	_ = monitor.AddMetric(&Metric{
		Type:        Counter,
		Name:        metricSlowRequest,
		Description: fmt.Sprintf("the server handled slow requests counter, t=%d.", m.slowTime),
		Labels:      m.getMetricLabelsIncludingMetadata(metricSlowRequest),
	})
}

func (m *Monitor) includesMetadata() bool {
	return len(m.metadata) > 0
}

func (m *Monitor) getMetadata() ([]string, []string) {
	metadata_labels := []string{}
	metadata_values := []string{}

	for v := range m.metadata {
		metadata_labels = append(metadata_labels, v)
		metadata_values = append(metadata_values, m.metadata[v])
	}

	return metadata_labels, metadata_values
}

func (m *Monitor) getMetricLabelsIncludingMetadata(metricName string) []string {
	includes_metadata := m.includesMetadata()
	metadata_labels, _ := m.getMetadata()

	switch metricName {
	case metricRequestDuration:
		metric_labels := []string{"uri"}
		if includes_metadata {
			metric_labels = append(metric_labels, metadata_labels...)
		}
		return metric_labels

	case metricURIRequestTotal:
		metric_labels := []string{"uri", "method", "code"}
		if includes_metadata {
			metric_labels = append(metric_labels, metadata_labels...)
		}
		return metric_labels

	case metricSlowRequest:
		metric_labels := []string{"uri", "method", "code"}
		if includes_metadata {
			metric_labels = append(metric_labels, metadata_labels...)
		}
		return metric_labels

	default:
		var metric_labels []string = nil
		if includes_metadata {
			metric_labels = metadata_labels
		}
		return metric_labels
	}
}

// monitorInterceptor as gin monitor middleware.
func (m *Monitor) monitorInterceptor(ctx *gin.Context) {
	// some paths should not be reported
	if ctx.Request.URL.Path == m.metricPath ||
		slices.Contains(m.excludePaths, ctx.Request.URL.Path) {
		ctx.Next()
		return
	}
	startTime := time.Now()

	// execute normal process.
	ctx.Next()

	// after request
	m.ginMetricHandle(ctx, startTime)
}

func (m *Monitor) ginMetricHandle(ctx *gin.Context, start time.Time) {
	r := ctx.Request
	w := ctx.Writer

	// set request total
	var metric_values []string = nil
	_ = m.GetMetric(metricRequestTotal).Inc(m.getMetricValues(metric_values))

	// set uv
	if clientIP := ctx.ClientIP(); !bloomFilter.Contains(clientIP) {
		bloomFilter.Add(clientIP)
		metric_values = nil
		_ = m.GetMetric(metricRequestUVTotal).Inc(m.getMetricValues(metric_values))
	}

	// set uri request total
	metric_values = []string{ctx.FullPath(), r.Method, strconv.Itoa(w.Status())}
	_ = m.GetMetric(metricURIRequestTotal).Inc(m.getMetricValues(metric_values))

	// set request body size
	// since r.ContentLength can be negative (in some occasions) guard the operation
	if r.ContentLength >= 0 {
		metric_values = nil
		_ = m.GetMetric(metricRequestBody).Add(m.getMetricValues(metric_values), float64(r.ContentLength))
	}

	// set slow request
	latency := time.Since(start)
	if int32(latency.Seconds()) > m.slowTime {
		metric_values = []string{ctx.FullPath(), r.Method, strconv.Itoa(w.Status())}
		_ = m.GetMetric(metricSlowRequest).Inc(m.getMetricValues(metric_values))
	}

	// set request duration
	metric_values = []string{ctx.FullPath()}
	_ = m.GetMetric(metricRequestDuration).Observe(m.getMetricValues(metric_values), latency.Seconds())

	// set response size
	if w.Size() > 0 {
		metric_values = nil
		_ = m.GetMetric(metricResponseBody).Add(m.getMetricValues(metric_values), float64(w.Size()))
	}
}

func (m *Monitor) getMetricValues(metric_values []string) []string {
	includes_metadata := m.includesMetadata()
	_, metadata_values := m.getMetadata()
	if includes_metadata {
		metric_values = append(metric_values, metadata_values...)
	}
	return metric_values
}
