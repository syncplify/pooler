package main

import (
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/segmentio/ksuid"
	"github.com/syncplify/pooler"
)

var tasks *pooler.Pool

var startTime = time.Now()

var counter int32 // we'll use this to simulate some work

// callMe is our callback function, we want this to be called when a task is done
func callMe(routine int, task *pooler.Task) {
	fmt.Printf("<< Goroutine %d is done with task %s\n", routine, task.ID())
}

// stats prints "what's going on" every 0.5 seconds
func stats() {
	for {
		cnt := atomic.LoadInt32(&counter)
		if cnt < 1000000 {
			fmt.Printf("#\n# Elapsed: %s - Counter: %d\n#\n", time.Now().Sub(startTime).String(), cnt)
		} else {
			fmt.Printf("#\n# Elapsed: %s - Counter: %d\n# Hit Ctrl-C to terminate the program\n#\n", time.Now().Sub(startTime).String(), cnt)
		}
		time.Sleep(time.Second)
	}
}

func main() {
	// Set GOMAXPROCS = NumCPU to use all available cores
	runtime.GOMAXPROCS(runtime.NumCPU())

	// Let's create a pool of 64 "workers" with a queue of up to 1 million tasks to execute, and a callback function
	var err error
	cfg := pooler.NewConfig(64, 1000000).OnTaskDone(callMe)
	tasks, err = pooler.NewWithConfig(cfg)
	if err != nil {
		panic(err)
	}

	// This goroutine only shows "what's going on" every second
	go stats()

	// Now we spawn 10 goroutines that simultaneouly enqueue 100,000 tasks each to the pool (tot: 1 million tasks)
	for k := 0; k < 10; k++ {
		go func() {
			for i := 0; i < 100000; i++ {
				// Each task *must* have a unique ID, we use the excellent segmentio/ksuid package for this purpose
				// In this extended example we also add some custom data to the task
				job := &myTask{TaskID: ksuid.New().String(), Custom: &myCustomData{Name: "Test", Amount: 42, Stamp: time.Now()}}
				// Let's add the task to the queue of tasks to be executed (enqueue)
				err := tasks.Enqueue(job)
				if err != nil {
					fmt.Println(err)
				}
			}
		}()
	}

	// Now let's just wait for the user to hit Ctrl-C
	quit := make(chan os.Signal)
	signal.Notify(quit, os.Interrupt, os.Kill, syscall.SIGTERM)
	<-quit

	// Shutdown the pool
	tasks.Shutdown()

	// Check the counter
	fmt.Println("Final value of counter:", counter)
}

// ****************************
// * TASK TO BE EXECUTED      *
// ****************************

type myTask struct {
	TaskID string
	Custom *myCustomData
}

type myCustomData struct {
	Name   string
	Amount int
	Stamp  time.Time
}

// In order to be a valid "pooler task" our struct needs to implement the pooler.Runnable interface,
// which means that we need (mandatory) to implement three methods:
// 1. ID() to return the task's unique ID
// 2. CustomData() to return the task's custom data, or nil in case this task has no need for custom data
// 3. Run(routine id) which is the actual func that runs the task

func (t *myTask) ID() string {
	return t.TaskID
}

func (t *myTask) CustomData() interface{} {
	return t.Custom
}

func (t *myTask) Run(routine int) error {
	atomic.AddInt32(&counter, 1)
	return nil
}
