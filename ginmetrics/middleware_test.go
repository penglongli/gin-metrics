package ginmetrics_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/penglongli/gin-metrics/ginmetrics"
)

func init() {
	gin.SetMode(gin.ReleaseMode)
}

func TestMiddlewareSamePort(t *testing.T) {
	r := newRouter()

	ts := httptest.NewServer(r)
	defer ts.Close()

	var wg sync.WaitGroup
	nLoops := 1000
	wg.Add(nLoops)

	for i := 0; i < nLoops; i++ {
		go func(t *testing.T, i int) {
			res, err := http.Get(ts.URL + fmt.Sprintf("/product/%d", i))
			if err != nil {
				t.Errorf("Expected nil, received %s", err.Error())
			}

			if res.StatusCode != http.StatusOK {
				t.Errorf("Expected %d, received %d", http.StatusOK, res.StatusCode)
			}
			wg.Done()
		}(t, i)
	}

	wg.Wait()
}

func TestMiddlewareDifferentPort(t *testing.T) {
	appRouter, metricsRouter := newRouterSeparateMetrics()

	ts := httptest.NewServer(appRouter)
	tms := httptest.NewServer(metricsRouter)

	defer ts.Close()
	defer tms.Close()

	var wg sync.WaitGroup
	nLoops := 1000
	wg.Add(nLoops)

	for i := 0; i < nLoops; i++ {
		go func(t *testing.T, i int) {
			res, err := http.Get(ts.URL + fmt.Sprintf("/product/%d", i))
			if err != nil {
				t.Errorf("Expected nil, received %s", err.Error())
			}

			if res.StatusCode != http.StatusOK {
				t.Errorf("Expected %d, received %d", http.StatusOK, res.StatusCode)
			}
			wg.Done()
		}(t, i)
	}

	wg.Wait()
}

func newRouterSeparateMetrics() (*gin.Engine, *gin.Engine) {
	appRouter := gin.New()
	metricRouter := gin.Default()

	m := ginmetrics.GetMonitor()
	m.UseWithoutExposingEndpoint(appRouter)
	m.Expose(metricRouter)

	appRouter.GET("/product/:id", func(ctx *gin.Context) {
		ctx.JSON(200, map[string]string{
			"productId": ctx.Param("id"),
		})
	})

	return appRouter, metricRouter
}

func newRouter() *gin.Engine {
	r := gin.New()

	m := ginmetrics.GetMonitor()
	m.SetMetricPath("/metrics")
	m.SetSlowTime(10)
	m.SetDuration([]float64{0.1, 0.3, 1.2, 5, 10})
	m.Use(r)

	r.GET("/product/:id", func(ctx *gin.Context) {
		ctx.JSON(200, map[string]string{
			"productId": ctx.Param("id"),
		})
	})

	return r
}
