package ginmetrics

import (
	"sync"
	"testing"
)

func TestSingletonRacing(t *testing.T) {
	var wg sync.WaitGroup
	nLoops := 1000
	wg.Add(nLoops)
	for i := 0; i < nLoops; i++ {
		go func() {
			GetMonitor()
			wg.Done()
		}()
	}

	wg.Wait()
}
