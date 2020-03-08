package main

import (
	"fmt"
	"math/rand"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/segmentio/ksuid"
	"github.com/syncplify/pooler"
)

var tasks *pooler.Pool

var startTime = time.Now()

// notifyUp is a callback func that will be created when a new goroutine is started
func notifyUp(routine int) {
	fmt.Printf(">> Goroutine %d has been started\n", routine)
}

// notifyDown is a callback func that will be created when a new goroutine is stopped
func notifyDown(routine int) {
	fmt.Printf("<< Goroutine %d has been shut down\n", routine)
}

// stats prints "what's going on" every 0.5 seconds
func stats() {
	for {
		time.Sleep(time.Second * 3)
		aw := tasks.ActiveWorkers()
		at := tasks.ActiveTasks()
		ngr := rand.Intn(16) + 1
		if at == 0 {
			fmt.Printf("#\n# Elapsed: %s - Active Workers/Tasks: %d/%d\n# Hit Ctrl-C to terminate the program\n#\n", time.Now().Sub(startTime).String(), aw, at)
		} else {
			fmt.Printf("#\n# Elapsed: %s - Active Workers/Tasks: %d/%d\n#\n", time.Now().Sub(startTime).String(), aw, at)
			fmt.Println("#===> Attempting to resize pool to:", ngr)
			err := tasks.Resize(int32(ngr))
			if err != nil {
				fmt.Println(err)
			} else {
				fmt.Println("Pool resized successfully!!")
			}
		}
	}
}

func main() {
	// Set GOMAXPROCS = NumCPU to use all available cores
	runtime.GOMAXPROCS(runtime.NumCPU())

	// Let's create a pool of 16 "workers" with a queue of up to 1 million tasks to execute, and a callback function
	var err error
	cfg := pooler.NewConfig(16, 1000000).OnWorkerCreated(notifyUp).OnWorkerShutdown(notifyDown)
	tasks, err = pooler.NewWithConfig(cfg)
	if err != nil {
		panic(err)
	}

	// This goroutine only shows "what's going on" every second
	go stats()

	// Now we spawn a goroutine that will enqueue 200 tasks to the pool
	go func() {
		for i := 0; i < 200; i++ {
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

	// After 8 seconds we reduce the pool size from 16 to 8 routines
	// time.Sleep(time.Second * 4)
	// tasks.Resize(64)

	// After another 8 seconds we increase the size of the pool to 64
	// time.Sleep(time.Second * 4)
	// tasks.Resize(32)

	// Now let's just wait for the user to hit Ctrl-C
	quit := make(chan os.Signal)
	signal.Notify(quit, os.Interrupt, os.Kill, syscall.SIGTERM)
	<-quit

	// Shutdown the pool
	tasks.Shutdown()
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
	// Simulate some work by simply waiting 5 seconds
	time.Sleep(time.Second * 5)
	return nil
}
