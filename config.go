package pooler

// NewConfig creates and returns a basic/initial Config struct, with a specified numer of worker `routines` and a specified maximum number of queueable `maxTasks`.
func NewConfig(routines int64, maxTasks int64) *Config {
	cfg := &Config{}
	cfg.Routines.Store(routines)
	cfg.MaxTasks.Store(maxTasks)
	return cfg
}

// OnWorkerCreated sets the callback function that's called when a new "worker" goroutine is created.
func (c *Config) OnWorkerCreated(fn CallbackFuncWrk) *Config {
	c.WorkerCreatedCB = fn
	return c
}

// OnWorkerShutdown sets the callback function that's called when a new "worker" goroutine is shutdown.
func (c *Config) OnWorkerShutdown(fn CallbackFuncWrk) *Config {
	c.WorkerShutdownCB = fn
	return c
}

// OnTaskQueued sets the callback function that's called when a new task is successfully added to the pending queue.
func (c *Config) OnTaskQueued(fn CallbackFuncQueue) *Config {
	c.TaskQueuedCB = fn
	return c
}

// OnTaskQueuingError sets the callback function that's called when a "worker" goroutine is done running a task, but an error is returned.
func (c *Config) OnTaskQueuingError(fn CallbackFuncQueueErr) *Config {
	c.TaskQueuingErrorCB = fn
	return c
}

// OnTaskStarted sets the callback function that's called when a "worker" goroutine picks up a task from the queue and starts running it.
func (c *Config) OnTaskStarted(fn CallbackFuncTask) *Config {
	c.TaskStartedCB = fn
	return c
}

// OnTaskDone sets the callback function that's called when a "worker" goroutine is done running a task, and no error is returned.
func (c *Config) OnTaskDone(fn CallbackFuncTask) *Config {
	c.TaskDoneCB = fn
	return c
}

// OnTaskDoneWithError sets the callback function that's called when a "worker" goroutine is done running a task, but an error is returned.
func (c *Config) OnTaskDoneWithError(fn CallbackFuncTaskErr) *Config {
	c.TaskDoneWithErrorCB = fn
	return c
}

// OnTaskCrashed sets the callback function that's called when a "worker" goroutine suddenly crashed (panic) while running a task.
func (c *Config) OnTaskCrashed(fn CallbackFuncTaskErr) *Config {
	c.TaskCrashedCB = fn
	return c
}
