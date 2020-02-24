package pooler

import (
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/segmentio/ksuid"
)

var pool *Pool

var (
	counter int32 // we'll use this to simulate some work
	// How many goroutines/maxTasks
	routines = 10
	maxTasks = 1000
)

func init() {
	runtime.GOMAXPROCS(runtime.NumCPU() / 2)

	// Create the pool
	pool, _ = New(routines, maxTasks)
}

// ****************************
// * TASK TO BE EXECUTED      *
// ****************************

type myTask struct {
	TaskID string
}

func (t *myTask) ID() string {
	return t.TaskID
}

func (t *myTask) CustomData() interface{} {
	return nil // in this basic example, our task has no custom data, so we simply return nil
}

func (t *myTask) Run(routine int) error {
	atomic.AddInt32(&counter, 1)
	return nil
}

// ****************************
// ****************************

func TestPooler_Easy(t *testing.T) {
	if pool == nil {
		t.Fatal("1. Could not create pool: expected pointer, got nil")
	}
	// Enqueue jobs
	for i := 0; i < maxTasks; i++ {
		job := &myTask{TaskID: ksuid.New().String()}
		err := pool.Enqueue(job)
		if err != nil {
			t.Fatalf("2. Could not enqueue job: %s", err)
		}
	}
	// Allow some time for all jobs to finish
	time.Sleep(3 * time.Second)
	// Shutdown the pool
	pool.Shutdown()
	//Check results
	if int(counter) != maxTasks {
		t.Fatalf("3. Counter (%d) and maxTasks (%d) are not the same", counter, maxTasks)
	}
}

func BenchmarkPooler_Easy(b *testing.B) {
	for i := 0; i < b.N; i++ {
		job := &myTask{TaskID: ksuid.New().String()}
		pool.Enqueue(job)
	}
	for {
		if int(atomic.LoadInt32(&counter)) >= b.N {
			break
		}
	}
}
