package pooler

import "sync"

// CallbackFuncWrk is the prototype of a function that will be called by the pool to notify when workers are created or shutdown.
type CallbackFuncWrk func(routine int)

// CallbackFuncTask is the prototype of a function that will be called by the pool to notify when workers start/stop tasks.
type CallbackFuncTask func(routine int, task *Task)

// CallbackFuncTaskErr is the prototype of a function that will be called by the pool to notify of errors pertaining tasks (typically runtime errors).
type CallbackFuncTaskErr func(routine int, task *Task, err error)

// CallbackFuncQueue is the prototype of a function that will be called by the pool to notify of successful events pertaining the queue (like a task successfully enqueued, for example).
type CallbackFuncQueue func(task *Task)

// CallbackFuncQueueErr is the prototype of a function that will be called by the pool to notify of errors pertaining the queue (queuing errors).
type CallbackFuncQueueErr func(task *Task, err error)

// Config is the global pool configuration struct.
type Config struct {
	// Routines is the desired number of "worker" goroutines
	Routines int32
	// MaxTasks is the maximum number of tasks that can be in this pool's queue at any given time
	MaxTasks int64
	// WorkerCreatedCB is an optional callback func that will be called every time a "worker" goroutine is created
	WorkerCreatedCB CallbackFuncWrk
	// WorkerShutdownCB is an optional callback func that will be called every time a "worker" goroutine is shutdown
	WorkerShutdownCB CallbackFuncWrk
	// TaskQueuedCB is an optional callback func that will be called every time a task is successfully added to the pool's queue
	TaskQueuedCB CallbackFuncQueue
	// TaskQueuingErrorCB is an optional callback func that will be called every time there's a problem adding a task to the pool's queue
	TaskQueuingErrorCB CallbackFuncQueueErr
	// TaskStartedCB is an optional callback func that will be called every time a task is picked up by a "worker" routine and its execution begins
	TaskStartedCB CallbackFuncTask
	// TaskDoneCB is an optional callback func that will be called every time a task is done running without errors
	TaskDoneCB CallbackFuncTask
	// TaskDoneWithErrorCB is an optional callback func that will be called every time a task is done running but has returned an error
	TaskDoneWithErrorCB CallbackFuncTaskErr
	// TaskCrashedCB is an optional callback func that will be called every time a `panic` has occurred within the Run() method while a task was running
	TaskCrashedCB CallbackFuncTaskErr
}

// Runnable is the interface that all "runnable" tasks must implement.
type Runnable interface {
	ID() string
	Run(routine int) error
	CustomData() interface{}
}

// Task encapsulates a base struct for objects that implement the Runnable interface.
type Task struct {
	Runnable
}

// Pool is a container for a pool of goroutines that will run the queued tasks.
type Pool struct {
	cfg               *Config        // A user-provided configuration for this pool
	shutdownChannel   chan struct{}  // Channel used to shutdown the pool
	shutdownWG        sync.WaitGroup // WaitGroup used to wait for "worker" goroutines to shut down
	shuttingDown      int32          // starts as 0, will be atomically set to 1 when the pool is shutting down (so that tasks can check it)
	jobChannel        chan *Task     // Buffered channel used to dispatch tasks to the goroutines that run them
	shrinkChannel     chan struct{}  // Buffered channel that will be used to shrink the pool during a call to the .Resize method
	capacity          int64          // Maximum capacity of the pool
	currentLoad       int32          // Current number of goroutines actually doing something
	currentGoroutines int32          // Current number of running (including idle) goroutines
	nextGoroutine     int64          // Incremental (atomic) number to be assigned to the next goroutine
}
