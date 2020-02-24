// Package pooler implements a worker-pool paradigm, relying on channels for all goroutine interoperation in order to
// achieve high speed an thread-safety. The only other dependency is the sync/atomic package, but it's kept down to a
// minimum, because we want pooler's operation to be as non-blocking as possible.
package pooler

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

/*
	PUBLIC FUNCS
*/

// New creates a new Pool object without any Callback functions
// routines: maximum number of "worker" goroutines that are allowed to run concurrently
// maxTasks: maximum number of tasks that can be waiting in line to be executed by the next available goroutine
func New(routines int, maxTasks int) (*Pool, error) {
	cfg := NewConfig(routines, maxTasks)
	return NewWithConfig(cfg)
}

// NewWithConfig creates a new Pool object with a user-provided configuration
// config: a pointer to a Config object (see types.go)
func NewWithConfig(config *Config) (*Pool, error) {
	// Check that config is not nil first
	if config == nil {
		return nil, errors.New("cannot create pool with nil config")
	}
	// Create the job pool with its initial configuration
	pool := &Pool{
		cfg:             config,
		shutdownChannel: make(chan struct{}),
		jobChannel:      make(chan *Task, config.MaxTasks),
		capacity:        config.MaxTasks,
	}
	// Start routineNum goroutines to perform the tasks
	for goroutine := 0; goroutine < config.Routines; goroutine++ {
		go pool.worker(goroutine)
	}
	// Return the newly created pool
	return pool, nil
}

// Enqueue adds a task to the queue of tasks waiting to be executed
// task: any object that implements the Runnable interface (see types.go)
func (p *Pool) Enqueue(task Runnable) error {
	defer func() {
		if r := recover(); r != nil {
			// If callback func is not nil, call it and notify panic has been caught
			if p.cfg.TaskQueuingErrorCB != nil {
				switch x := r.(type) {
				case string:
					p.cfg.TaskQueuingErrorCB(&Task{task}, errors.New(x))
				case error:
					p.cfg.TaskQueuingErrorCB(&Task{task}, x)
				default:
					p.cfg.TaskQueuingErrorCB(&Task{task}, errors.New("unknown panic"))
				}
			}
		}
	}()
	if len(p.jobChannel) >= p.capacity {
		return errors.New("cannot add task at this time: maximum capacity of the pool has been reached")
	}
	if task.ID() == "" {
		return errors.New("cannot add a task without an ID")
	}
	// Send the task to the jobChannel so that the first available "worker" goroutine can pick it up
	job := &Task{task}
	p.jobChannel <- job
	// If callback func is not nil, call it and notify that the task has been enqueued
	if p.cfg.TaskQueuedCB != nil {
		p.cfg.TaskQueuedCB(job)
	}
	return nil
}

// QueueLen returns the number of tasks currently queued
func (p *Pool) QueueLen() int {
	return len(p.jobChannel)
}

// ActiveWorkers returns the number of goroutines that are actually busy doing something
func (p *Pool) ActiveWorkers() int {
	n := atomic.LoadInt32(&p.currentLoad)
	return int(n)
}

// Shutdown stops all goroutines running all tasks, and shuts down the entire pool
func (p *Pool) Shutdown() {
	// Atomically set shuttingDown to true
	atomic.StoreInt32(&p.shuttingDown, 1)
	// Close shutdownChannel and jobChannel, and wait for all goroutines to terminate
	close(p.shutdownChannel)
	close(p.jobChannel)
	p.jobChannel = nil
	p.shutdownWG.Wait()
}

// ShutdownWithTimeout stops all goroutines running all tasks, and shuts down the entire pool
// It returns true if it times out, and false if it shuts down regularly (before timeout occurs)
func (p *Pool) ShutdownWithTimeout(timeout time.Duration) bool {
	// Atomically set shuttingDown to true
	atomic.StoreInt32(&p.shuttingDown, 1)
	// Close shutdownChannel and jobChannel, and wait for all goroutines to terminate
	close(p.shutdownChannel)
	close(p.jobChannel)
	p.jobChannel = nil
	return waitTimeout(&p.shutdownWG, timeout)
}

// IsShuttingDown returns false during normal operation and true if the pool is shutting down;
// all tasks should periodically check it inside of their "Run" func.
func (p *Pool) IsShuttingDown() bool {
	return atomic.LoadInt32(&p.shuttingDown) == 1
}

/*
	PRIVATE FUNCS
*/

// waitTimeout waits for the waitgroup for the specified max timeout
// It returns true if it times out, and false if it shuts down regularly (before timeout occurs)
func waitTimeout(wg *sync.WaitGroup, timeout time.Duration) bool {
	c := make(chan struct{})
	go func() {
		defer close(c)
		wg.Wait()
	}()
	select {
	case <-c:
		return false // completed normally
	case <-time.After(timeout):
		return true // timed out
	}
}

// oneUp is used internally to increase the atomic counter of "running" tasks by 1
func (p *Pool) oneUp() {
	atomic.AddInt32(&p.currentLoad, 1)
}

// oneDown is used internally to decrease the atomic counter of "running" tasks by 1
func (p *Pool) oneDown() {
	atomic.AddInt32(&p.currentLoad, -1)
}

// worker is the actual goroutine (of which routineNum will be spawned upon pool creation)
func (p *Pool) worker(goroutine int) {
	// Add self to the shutdown WaitGroup
	p.shutdownWG.Add(1)
	defer p.shutdownWG.Done()
	// If callback func is not nil, call it and notify goroutine has been created
	if p.cfg.WorkerCreatedCB != nil {
		p.cfg.WorkerCreatedCB(goroutine)
	}
	// Wait for something to do (or for shutdown signal)
	for {
		select {
		// Do we need to shutdown the "worker" goroutine?
		case <-p.shutdownChannel:
			// If callback func is not nil, call it and notify goroutine is going down
			if p.cfg.WorkerShutdownCB != nil {
				p.cfg.WorkerShutdownCB(goroutine)
			}
			// The following return will cause the above "defer" to trigger, ensuring consistent decrement of the shutdown WaitGroup
			return
		// Is there a pending task that should be run?
		case task, ok := <-p.jobChannel:
			if ok && !p.IsShuttingDown() {
				p.safeDo(goroutine, task)
			}
		}
	}
}

// safeDo calls the task's "Run" func in a controlled/recoverable way
func (p *Pool) safeDo(routine int, task *Task) {
	// If callback func is not nil, call it and notify task has been started
	if p.cfg.TaskStartedCB != nil {
		p.cfg.TaskStartedCB(routine, task)
	}
	p.oneUp()
	defer p.oneDown()
	defer func() {
		if r := recover(); r != nil {
			// If callback func is not nil, call it and notify panic has been caught
			if p.cfg.TaskCrashedCB != nil {
				switch x := r.(type) {
				case string:
					p.cfg.TaskCrashedCB(routine, task, errors.New(x))
				case error:
					p.cfg.TaskCrashedCB(routine, task, x)
				default:
					p.cfg.TaskCrashedCB(routine, task, errors.New("unknown panic"))
				}
			}
		}
	}()
	// Actually run the task
	err := task.Run(routine)
	// If the Run func returned an error (and callback func is not nil) use callback to report the error
	if err != nil {
		if p.cfg.TaskDoneWithErrorCB != nil {
			p.cfg.TaskDoneWithErrorCB(routine, task, err)
		}
		return
	}
	// Otherwise if callback func is not nil and we get here, report the task completed without errors
	if p.cfg.TaskDoneCB != nil {
		p.cfg.TaskDoneCB(routine, task)
	}
}
