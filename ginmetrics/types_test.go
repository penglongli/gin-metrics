package ginmetrics

import "testing"

func TestSingletonRacing(t *testing.T) {
	for i := 0; i < 10; i++ {
		go GetMonitor()
	}
}
