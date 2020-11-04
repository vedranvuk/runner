package runner

import (
	"context"
	"errors"
	"sync"
)

var (
	// ErrEmptyName is returned by Register when Engine name is not specified.
	ErrEmptyName = errors.New("runner: engine name not specified")
	// ErrDuplicateName is returned by Register if a duplicate name was specified.
	ErrDuplicateName = errors.New("runner: engine under specified name already registered")
	// ErrNilEngine is returned by Register if no engine was specified.
	ErrNilEngine = errors.New("runner: engine is nil")
	// ErrRegisterOnRunning is returned by Register if Runner is running.
	ErrRegisterOnRunning = errors.New("runner: cannot register engine, runner not idle")
	// ErrRunning is returned when Start was called and Runner was not idle.
	ErrRunning = errors.New("runner: cannot start, not idle")
	// ErrAlreadyStopped is returned when Stop was called and Runner was already idle.
	ErrAlreadyStopped = errors.New("runner: already stopped")
	// ErrShuttingDown is returned when Stop is called and Runner is shutting down.
	ErrShuttingDown = errors.New("runner: cannot stop, shutting down")
	// ErrFailing is returned when Stop is called and Runner is shutting down due to an Engine Start error.
	ErrFailing = errors.New("runner: cannot stop, erroring")
)

// Engine defines an interface to a type that can be Started and Stopped by
// the Runner. Its' Start method can be longrunning and Stop must be able to
// stop it.
//
// Both methods receive a context with a timeout value which is free to
// interpretation of the implementor.
type Engine interface {
	// Start must run the Engine then return a nil error on success or an error
	// if it failed. Start can be longrunning and must be stoppable by Stop.
	// If Stop was called, Start should return before Stop returns.
	// If Stop is called and it returns before Start, Engine is terminated.
	Start(context.Context) error
	// Stop must stop the Run method which must return before stop.
	// Stop must return a nil error on success or an error if it failed.
	Stop(context.Context) error
}

// Runner runs multiple Stack Engines simultaneously.
type Runner struct {
	cb        DoneCallback    // cb is the optional callback function.
	startdone chan error      // startdone is signal to Start to return.
	stopdone  chan error      // startdone is signal to Stop to return.
	startctx  context.Context // startctx is the Start context.
	stopctx   context.Context // stopctx is the Stop context.
	mu        sync.Mutex      // mu protects the following fields.
	engines   map[string]*eng // engines is a map of registered Engines.
	state     State           // state is the current Runner state.
	count     int             // count is the registered engine count.
	running   int             // running is the count of running Engines.
	err       error           // err stores an indicator error.
}

// eng holds engine states and definitions.
type eng struct {
	running bool     // running marks if engine is running.
	engine  Engine   // engine is the Engine being run.
	options *Options // options are the engine specific options.
}

// DoneCallback is a prototype of a callback function to be called when an
// Engine Start or Stop methods return.
//
// It is intended to report handled errors, mainly to log them.
//
// Name will be the name of the Engine as registered.
// Start, if true will indicate the value is from the Engine Start method.
// Start, if false will indicate the value is from the Engine Stop method.
// Err will be the value returned by the method.
type DoneCallback = func(name string, start bool, err error)

// New returns a new *Runner instance with optional Engine callback cb.
func New(donecb DoneCallback) *Runner {
	return &Runner{
		cb:        donecb,
		engines:   make(map[string]*eng),
		startdone: make(chan error),
		stopdone:  make(chan error),
	}
}

// Options specify registration parameters for an Engine.
// For now, only Engine specific contexts are supported.
type Options struct {
	// startctx is a context to pass to the Engine's Start method.
	// If nil, startctx specified in the Start method will be used.
	startctx context.Context
	// stopctx is a context to pass to the Engine's Stop method.
	// If nil, stopctx specified in the Start method will be used.
	stopctx context.Context
}

// Register registers engine under specified name. If an error occurs it is
// returned and the engine was not registered.
//
// Name specifies the name under which engine, which must not be nil, will be
// registered and options, which are optional and can be nil, specify Engine
// specific options.
//
// Engines cannot be registered while the Runner is running and name must not
// be empty and must be unique.
func (r *Runner) Register(name string, engine Engine, options *Options) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if name == "" {
		return ErrEmptyName
	}
	if engine == nil {
		return ErrNilEngine
	}
	if r.state != StateIdle {
		return ErrRegisterOnRunning
	}
	if _, exists := r.engines[name]; exists {
		return ErrDuplicateName
	}
	p := &eng{
		engine:  engine,
		options: options,
	}
	r.engines[name] = p
	r.count++
	return nil
}

// Start starts the Runner by running all Engines registered with it
// simultaneously and returns a nil error if all Engines have finished work
// without an error or have been stopped by Stop. If an Engine fails before
// Stop is called Start will return its error after first stopping other
// Engines. Runner DoneCallback can report on handled errors.
//
// Start will return a non-nil error if one of the Engine Start methods returns
// an error before Stop request was issued and it will be the error of that
// Engine's Start method that caused Runner to abort.
//
// Startctx and stopctx are contexts to pass to all Engines Start methods if
// Engine-specific contexts were not specified during registration. It is not
// used by Runner and can be optional and nil.
//
// If all Engines finished successfully or return no errors before Stop was
// called result will be nil.
func (r *Runner) Start(startctx, stopctx context.Context) error {
	r.mu.Lock()
	if r.state != StateIdle {
		r.mu.Unlock()
		return ErrRunning
	}
	r.startctx = startctx
	r.stopctx = stopctx
	r.err = nil
	r.state = StateRunning
	for enginename, enginedef := range r.engines {
		if !enginedef.running {
			go r.runEngine(enginename, enginedef)
		}
	}
	r.mu.Unlock()
	return <-r.startdone
}

// Stop stops the Runner by issuing a Stop request to all Engines. Start will
// return a nil error and the first error reported by an Engine's Stop method
// will be returned by Stop. Handled errors are reported via DoneCallback.
//
// Stopctx specified in Start is used as described in Start help.
func (r *Runner) Stop() error {
	r.mu.Lock()
	if r.state != StateRunning {
		r.mu.Unlock()
		switch r.state {
		case StateIdle:
			return ErrAlreadyStopped
		case StateStoppingFail:
			return ErrShuttingDown
		case StateStoppingRequest:
			return ErrFailing
		}
	}
	r.state = StateStoppingRequest
	for enginename, enginedef := range r.engines {
		if enginedef.running {
			go r.stopEngine(enginename, enginedef)
		}
	}
	r.mu.Unlock()
	return <-r.stopdone
}

// runEngine help.
func (r *Runner) runEngine(name string, d *eng) {
	r.mu.Lock()
	if r.state != StateRunning {
		r.mu.Unlock()
		return
	}
	e := d.engine
	r.running++
	d.running = true
	var ctx context.Context = r.startctx
	if d.options != nil && d.options.startctx != nil {
		ctx = d.options.startctx
	}
	r.mu.Unlock()
	err := e.Start(ctx)
	r.onEngineDone(name, d, err)
}

// stopEngine help.
func (r *Runner) stopEngine(name string, d *eng) {
	r.mu.Lock()
	if !d.running {
		r.mu.Unlock()
		return
	}
	e := d.engine
	var ctx context.Context = r.stopctx
	if d.options != nil && d.options.stopctx != nil {
		ctx = d.options.stopctx
	}
	r.mu.Unlock()
	r.handleError(e.Stop(ctx), true)
}

// handleError help.
func (r *Runner) handleError(err error, lock bool) {
	if lock {
		r.mu.Lock()
		defer r.mu.Unlock()
	}
	if err != nil {
		switch r.state {
		case StateRunning:
			r.state = StateStoppingFail
			for enginename, enginedef := range r.engines {
				if enginedef.running {
					go r.stopEngine(enginename, enginedef)
				}
			}
			if r.err == nil {
				r.err = err
			}
		case StateStoppingRequest:
			if r.err == nil {
				r.err = err
			}
		}
	}
}

// onEngineDone help.
func (r *Runner) onEngineDone(name string, d *eng, err error) {

	r.mu.Lock()

	if !d.running {
		r.mu.Unlock()
		return
	}

	r.running--
	d.running = false

	if r.cb != nil {
		switch r.state {
		case StateRunning, StateStoppingFail:
			defer r.cb(name, true, err)
		case StateStoppingRequest, StateIdle:
			defer r.cb(name, false, err)
		}
	}

	if r.running == 0 {
		switch r.state {
		case StateStoppingFail:
			r.startdone <- r.err
		case StateRunning:
			r.startdone <- r.err
		case StateStoppingRequest:
			r.startdone <- nil
			r.stopdone <- r.err
		}
		r.state = StateIdle
		r.mu.Unlock()
		return
	}

	r.mu.Unlock()
}

// State is the Runner state.
type State uint8

const (
	// StateIdle indicates Runner is idle and can be Started.
	StateIdle State = iota
	// StateRunning indicates Runner is Running and can be Stopped.
	StateRunning
	// StateStoppingFail indicates Runner is stopping due to an Engine failure.
	StateStoppingFail
	// StateStoppingRequest indicates Runner is stopping due to a Stop call.
	StateStoppingRequest
)

// State reports current Runner state.
func (r *Runner) State() State {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.state
}
