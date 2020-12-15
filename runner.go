package runner

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

var (
	// ErrRunner is the base Runner error.
	ErrRunner = errors.New("runner")

	// ErrRegister is the base Register error.
	ErrRegister = fmt.Errorf("%w: register", ErrRunner)
	// ErrEmptyName is returned by Register when Engine name is not specified.
	ErrEmptyName = fmt.Errorf("%w: engine name not specified", ErrRegister)
	// ErrDuplicateName is returned by Register if a duplicate name was specified.
	ErrDuplicateName = fmt.Errorf("%w: engine under specified name already registered", ErrRegister)
	// ErrNilEngine is returned by Register if no engine was specified.
	ErrNilEngine = fmt.Errorf("%w: engine is nil", ErrRegister)
	// ErrRegisterOnRunning is returned by Register if Runner is running.
	ErrRegisterOnRunning = fmt.Errorf("%w: cannot register engine, runner not idle", ErrRegister)

	// ErrStart is the base Start error.
	ErrStart = fmt.Errorf("%w: start error", ErrRunner)
	// ErrRunning is returned when Start was called and Runner was not idle.
	ErrRunning = fmt.Errorf("%w: cannot start, not idle", ErrStart)
	// ErrNoEngines is returned when running a Runner with no defined engines.
	ErrNoEngines = fmt.Errorf("%w: no engines defined", ErrStart)

	// ErrStop is the base Stop error.
	ErrStop = fmt.Errorf("%w: stop error", ErrRunner)
	// ErrIdle is returned when Stop was called and Runner was already idle.
	ErrIdle = fmt.Errorf("%w: already stopped", ErrStop)
	// ErrFailing is returned when Stop is called and Runner is shutting down due to an Engine Start error.
	ErrFailing = fmt.Errorf("%s: already stopping due to failure", ErrStop)
	// ErrShuttingDown is returned when Stop is called and Runner is shutting down.
	ErrShuttingDown = fmt.Errorf("%w: already stopping", ErrStop)
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
	// If Stop was called, Start should return before Stop,
	// if it does not it is immediately terminated.
	Start(context.Context) error
	// Stop must stop the Engine in a manner described for Start.
	// Errors returned by Stop do not affect Runner's behaviour. They are only
	// optionally reported in DoneCallback.
	Stop(context.Context) error
}

// DoneCallback is a prototype of a callback function to be called when an
// Engine Start or Stop methods return.
//
// It is intended to report Engine errors mainly to log them as they are
// all handled by Runner beforehand.
//
// Name will be the name of the Engine as registered.
// Start, if true will indicate the value is from the Engine Start method.
// Start, if false will indicate the value is from the Engine Stop method.
// Err will be the value returned by Engine's Start or Stop methods, depending.
type DoneCallback = func(name string, start bool, err error)

// Runner runs multiple Engine implementors simultaneously.
//
// Engines can be registered with Register then run simultaneously using Start.
// If any one Engine's Start methods return a non-nil error before all engines
// finish with a nil Start error or Runner Stop is called, all other running
// engines are stopped by calling their Stop method.
//
// If all Engines Start methods finish with a nil error Runner's Start method
// returns nil.
//
// Running engines can be stopped with Stop and will return when all Engines
// have stopped.
//
// Runner can be restarted multiple times regardless of success of the last
// Start result.
type Runner struct {
	donecb      DoneCallback       // cb is the optional callback function.
	startdone   chan error         // startdone is signal to Start to return.
	stopdone    chan error         // startdone is signal to Stop to return.
	failtimeout time.Duration      // Fail timeout duration calculated from startctx.
	startctx    context.Context    // startctx is the Start context.
	stopctx     context.Context    // stopctx is the Stop context.
	mu          sync.Mutex         // mu protects the following fields.
	engines     map[string]*engine // engines is a map of registered Engines.
	state       State              // state is the current Runner state.
	count       int                // count is the registered engine count.
	running     int                // running is the count of running Engines.
	err         error              // err stores an indicator error.
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

// engine stores registered Engine info.
type engine struct {
	running bool   // running marks if engine is running.
	engine  Engine // engine is this info's Engine reference.
}

// New returns a new *Runner instance with optional callback donecb.
func New(donecb DoneCallback) *Runner {
	return &Runner{
		donecb:    donecb,
		startdone: make(chan error),
		stopdone:  make(chan error),
		engines:   make(map[string]*engine),
	}
}

// Register registers engine under specified name. If an error occurs it is
// returned and the engine was not registered.
//
// Name specifies the name under which engine will be registered.
// Engines cannot be registered while the Runner is running and name must not
// be empty and must be unique.
func (r *Runner) Register(name string, e Engine) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if name == "" {
		return ErrEmptyName
	}
	if e == nil {
		return ErrNilEngine
	}
	if r.state != StateIdle {
		return ErrRegisterOnRunning
	}
	var exists bool
	if _, exists = r.engines[name]; exists {
		return fmt.Errorf("%w: '%s'", ErrDuplicateName, name)
	}
	r.engines[name] = &engine{
		engine: e,
	}
	r.count++
	return nil
}

// MustRegister is like Register but returns the Runner on success or panics
// if an error occurs.
func (r *Runner) MustRegister(name string, e Engine) *Runner {
	var err error
	if err = r.Register(name, e); err != nil {
		panic(err)
	}
	return r
}

// Start starts the Runner by running all Engines registered with it
// simultaneously and returns a nil error if all Engines have finished work
// without an error or have been successfully stopped by Stop before finishing.
//
// If any one Engine Start method returns a non-nil error before all Engines
// have finished successfully with a nil error or have been stopped by Runner
// Start will return that error after first stopping any other running Engines.
// If any other Engines Start method returns a non-nil error during this
// "failure shutdown" it will be reported via Runner's callback.
//
// Startctx is a context passed to all registered Engines Start methods which
// is open to Engine's interpretation. If it carries a timeout its' duration is
// used to construct a new context with same timeout duration that will be
// passed to Engine Stop methods in case of the "failure shutdown".
//
// If all Engines finished successfully or return no errors before Stop was
// called result will be nil.
//
// If starting Runner with no registered engines an ErrNoEngines is returned.
func (r *Runner) Start(ctx context.Context) error {
	r.mu.Lock()
	if len(r.engines) == 0 {
		r.mu.Unlock()
		return ErrNoEngines
	}
	if r.state != StateIdle {
		r.mu.Unlock()
		return ErrRunning
	}
	r.startctx = ctx
	// If a start context is specified store its' timeout duration
	// for constructing a "failure shutdown" context passed to Engine
	// Stop methods.
	if ctx != nil {
		if deadline, ok := ctx.Deadline(); ok {
			r.failtimeout = time.Until(deadline)
		}
	}
	r.err = nil
	r.state = StateRunning
	var enginename string
	var enginedef *engine
	for enginename, enginedef = range r.engines {
		if !enginedef.running {
			go r.startEngine(enginename, enginedef)
		}
	}
	r.mu.Unlock()
	return <-r.startdone
}

// Stop stops the Runner by issuing a Stop request to all Engines. It waits
// for engines indefinitely.
//
// Context ctx is passed to Engine Stop methods and is open to their
// interpretation.
//
// If any Engines Stop methods return a non-nil error Stop will return that
// error. If all Engines stop without an error, Stop returns nil.
// If any additional Engine Stop methods return a non-nil error they can be
// examined via Runner's callback.
//
// Issuing Stop on a Runner that is not idle returns one of predefined errors.
func (r *Runner) Stop(ctx context.Context) error {
	r.mu.Lock()
	if r.state != StateRunning {
		r.mu.Unlock()
		switch r.state {
		case StateIdle:
			return ErrIdle
		case StateStoppingFail:
			return ErrShuttingDown
		case StateStoppingRequest:
			return ErrFailing
		}
	}
	r.stopctx = ctx
	r.state = StateStoppingRequest
	var enginename string
	var enginedef *engine
	for enginename, enginedef = range r.engines {
		if enginedef.running {
			go r.stopEngine(enginename, enginedef)
		}
	}
	r.mu.Unlock()
	return <-r.stopdone
}

// State reports current Runner state.
func (r *Runner) State() State {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.state
}

// RunningEngines returns the number of running engines.
func (r *Runner) RunningEngines() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.running
}

// startEngine starts an engine with specified name from definition e.
func (r *Runner) startEngine(name string, e *engine) {
	r.mu.Lock()
	if r.state != StateRunning {
		r.mu.Unlock()
		return
	}
	e.running = true
	r.running++
	var engineintf = e.engine
	var ctx context.Context = r.startctx
	r.mu.Unlock()
	r.onEngineDone(name, e, engineintf.Start(ctx))
}

// stopEngine stops an engine in a goroutine and calls onEngineDone when it
// finishes. It safely modifies all states in the process.
func (r *Runner) stopEngine(name string, e *engine) {
	r.mu.Lock()
	if !e.running {
		r.mu.Unlock()
		return
	}
	var engineintf = e.engine
	var ctx context.Context
	switch r.state {
	case StateStoppingRequest:
		ctx = r.stopctx
	case StateStoppingFail:
		if r.startctx != nil && r.failtimeout != 0 {
			failctx, failcancel := context.WithTimeout(r.startctx, r.failtimeout)
			defer failcancel()
			ctx = failctx
		}
	}
	r.mu.Unlock()
	r.routeError(engineintf.Stop(ctx), true, e)
}

// routeError acts upon an error returned from an Engine Start or Stop methods
// depending on Runner state. If it is an error returned from an engine Start
// method it puts the Runner in the shutdown state. If it is an error returned
// from an Engine Stop method, it records the error to later be returned by
// Runner Stop method.
func (r *Runner) routeError(err error, lock bool, d *engine) {
	if lock {
		r.mu.Lock()
		defer r.mu.Unlock()
	}
	// Discard Engines whose Stop returns before Start.
	if d.running {
		d.running = false
		r.running--
		r.updateState()
		return
	}
	if err == nil {
		return
	}
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

// onEngineDone processes the event of an Engine's Start method returning.
// Calls the callback and conditionally updates runner state.
func (r *Runner) onEngineDone(name string, d *engine, err error) {
	r.mu.Lock()
	// Ignore discarded engines.
	if !d.running {
		r.mu.Unlock()
		return
	}
	r.running--
	d.running = false
	if r.donecb != nil {
		switch r.state {
		case StateRunning, StateStoppingFail:
			r.donecb(name, true, err)
		case StateStoppingRequest, StateIdle:
			r.donecb(name, false, err)
		}
	}
	r.routeError(err, false, d)
	r.updateState()
	r.mu.Unlock()
}

// updateState updates the runner state.
func (r *Runner) updateState() {
	if r.running > 0 {
		return
	}
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
}
