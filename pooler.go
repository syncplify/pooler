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

// New creates a new pooler.Pool object without any Callback functions.
// `routines` is the maximum number of "worker" goroutines that are allowed to run concurrently.
// `maxTasks` is the maximum number of tasks that can be waiting in line to be executed by the next available goroutine.
func New(routines int32, maxTasks int64) (*Pool, error) {
	cfg := NewConfig(routines, maxTasks)
	return NewWithConfig(cfg)
}

// NewWithConfig creates a new pooler.Pool object with a user-provided configuration.
// `config` is a pointer to a pooler.Config object (see types.go).
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
		shrinkChannel:   make(chan bool, config.MaxTasks),
		capacity:        config.MaxTasks,
	}
	// Upon pool creation, we can safely set the nextGoroutine number to the "routines" parameter of this func
	atomic.StoreInt64(&pool.nextGoroutine, int64(config.Routines))
	// Start routineNum goroutines to perform the tasks
	for goroutine := 0; goroutine < int(config.Routines); goroutine++ {
		go pool.worker(goroutine)
	}
	// Return the newly created pool
	return pool, nil
}

// Resize attempts to resize the pool, adding or terminating goroutines as needed. It returns an error
// if resizing conditions aren't met.
func (p *Pool) Resize(newGoroutines int32) error {
	cgr := p.ConfiguredRoutines()
	if int(newGoroutines) == cgr {
		// Trying to resize to the SAME number of already configured goroutines,
		// let's not be too dramatic, just do nothing and don't return any error.
		return nil
	}
	// Check boundaries
	if newGoroutines < 1 || int(newGoroutines) > p.MaxTasks() {
		return errors.New("goroutine number out of boundaries")
	}
	// Check if a previous resize operation is still ongoing
	aw := p.ActiveWorkers()
	if aw != cgr {
		return errors.New("cannot resize while a previous resize operation is still ongoing")
	}
	// If we get here we can resize the pool
	// Do we need to shrink the pool?
	if int(newGoroutines) < cgr {
		diff := cgr - int(newGoroutines)
		for i := 0; i < diff; i++ {
			p.shrinkChannel <- true
		}
	} else {
		diff := int(newGoroutines) - cgr
		for goroutine := 0; goroutine < diff; goroutine++ {
			go p.worker(int(atomic.LoadInt64(&p.nextGoroutine)))
			atomic.AddInt64(&p.nextGoroutine, 1)
		}
	}
	// Last, set new number of configured routines atomically, and return nil (no error)
	atomic.StoreInt32(&p.cfg.Routines, newGoroutines)
	return nil
}

// Enqueue adds a task to the queue of tasks waiting to be executed.
// `task` can be any object that implements the pooler.Runnable interface (see types.go).
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
	if len(p.jobChannel) >= int(p.capacity) {
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

// ConfiguredRoutines returns the number of configured goroutines in a thread-safe way
func (p *Pool) ConfiguredRoutines() int {
	n := atomic.LoadInt32(&p.cfg.Routines)
	return int(n)
}

// MaxTasks returns the maximum number of queueable tasks in a thread-safe way
func (p *Pool) MaxTasks() int {
	n := atomic.LoadInt64(&p.cfg.MaxTasks)
	return int(n)
}

// QueueLen returns the number of tasks currently queued, and waiting to be executed.
func (p *Pool) QueueLen() int {
	return len(p.jobChannel)
}

// ActiveWorkers returns the number of running goroutines, including the ones that are idle.
func (p *Pool) ActiveWorkers() int {
	n := atomic.LoadInt32(&p.currentGoroutines)
	return int(n)
}

// ActiveTasks retuns the number of tasks that are REALLY being executed at this time.
func (p *Pool) ActiveTasks() int {
	n := atomic.LoadInt32(&p.currentLoad)
	return int(n)
}

// Shutdown stops all goroutines running all tasks, and shuts down the entire pool. Please note that
// this method could actually wait forever untill all pending tasks are done.
func (p *Pool) Shutdown() {
	// Atomically set shuttingDown to true
	atomic.StoreInt32(&p.shuttingDown, 1)
	// Close shutdownChannel and jobChannel, and wait for all goroutines to terminate
	close(p.shutdownChannel)
	close(p.jobChannel)
	p.jobChannel = nil
	p.shutdownWG.Wait()
}

// ShutdownWithTimeout stops all goroutines running all tasks, and shuts down the entire pool. It doesn't
// wait forever, and it always returns on or before `timeout`.
// It returns true if it times out, and false if it shuts down regularly (before timeout occurs).
// Please note that if this function returns true (a timeout has occurred) you may still have "orphan"
// goroutines running; it is, therefore, recommended that this is the among the last methods you call
// just before your program terminates.
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

// waitTimeout waits for the `wg` WaitGroup to return, for the specified maximum `timeout`.
// It returns true if it times out, and false if the WaitGroup returns regularly (i.e. before timeout occurs).
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

// goroutineUp is used internally to increase the atomic counter of current goroutines by 1.
func (p *Pool) goroutineUp() {
	atomic.AddInt32(&p.currentGoroutines, 1)
}

// goroutineDown is used internally to decrease the atomic counter of current goroutines by 1.
func (p *Pool) goroutineDown() {
	atomic.AddInt32(&p.currentGoroutines, -1)
}

// taskUp is used internally to increase the atomic counter of running tasks by 1.
func (p *Pool) taskUp() {
	atomic.AddInt32(&p.currentLoad, 1)
}

// taskDown is used internally to decrease the atomic counter of running tasks by 1.
func (p *Pool) taskDown() {
	atomic.AddInt32(&p.currentLoad, -1)
}

// worker is the actual "worker" goroutine (of which routineNum will be spawned upon pool creation).
func (p *Pool) worker(goroutine int) {
	// Add self to the shutdown WaitGroup and up 1 routine
	p.shutdownWG.Add(1)
	p.goroutineUp()
	// On exit, down 1 routine and leave the WaitGroup
	defer func() {
		p.goroutineDown()
		p.shutdownWG.Done()
	}()
	// If callback func is not nil, call it and notify goroutine has been created
	if p.cfg.WorkerCreatedCB != nil {
		p.cfg.WorkerCreatedCB(goroutine)
	}
	// Wait for something to do (or for shutdown signal)
	for {
		select {
		// Do we need to shutdown all "worker" goroutines?
		case <-p.shutdownChannel:
			// If callback func is not nil, call it and notify goroutine is going down
			if p.cfg.WorkerShutdownCB != nil {
				p.cfg.WorkerShutdownCB(goroutine)
			}
			// The following return will cause the above "defer" to trigger, ensuring consistent decrement of the shutdown WaitGroup
			return
			// Or maybe we need to shutdown this "worker" goroutine because of an ongoing .Resize?
		case <-p.shrinkChannel:
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

// safeDo calls the task's "Run" func in a controlled/recoverable way.
func (p *Pool) safeDo(routine int, task *Task) {
	// If callback func is not nil, call it and notify task has been started
	if p.cfg.TaskStartedCB != nil {
		p.cfg.TaskStartedCB(routine, task)
	}
	p.taskUp()
	defer p.taskDown()
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
