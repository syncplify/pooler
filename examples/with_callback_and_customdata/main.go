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
	_ "go.uber.org/automaxprocs" // automatic
)

var tasks *pooler.Pool

var startTime = time.Now()

var counter int32 // we'll use this to simulate some work

// callMeOnStart is a callback function, we want this to be called when a task is starting
// so we can acquire the task's custom data and change it
func callMeOnStart(routine int, task *pooler.Task) {
	// Get pointer to custom data, then change some value in it
	cd := task.CustomData().(*myCustomData)
	fmt.Printf(">> Goroutine %d is starting task %s with CustomData %d\n", routine, task.ID(), cd.Amount)
	// After displaying amount, change it!
	cd.Amount = 128
}

// callMeWhenDone is our callback function, we want this to be called when a task is done
// so we can show that the task's custom data has, indeed, changed
func callMeWhenDone(routine int, task *pooler.Task) {
	// Get pointer to custom data, then display the value we previously changed
	cd := task.CustomData().(*myCustomData)
	fmt.Printf("<< Goroutine %d is done with task %s with CustomData %d\n", routine, task.ID(), cd.Amount)
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

	// Let's create a pool of 64 "workers" with a queue of up to 1 million tasks to execute, and a few callback functions
	// that change and/or display the task's CustomData
	var err error
	cfg := pooler.NewConfig(64, 1000000).OnTaskStarted(callMeOnStart).OnTaskDone(callMeWhenDone)
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

type myCustomData struct {
	Name   string
	Amount int
	Stamp  time.Time
}

type myTask struct {
	TaskID string
	Custom *myCustomData
}

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
