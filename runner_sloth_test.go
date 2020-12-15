package runner

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)


// ErrNotRunning is returned by Sloth when Stop is called on stopped Sloth.
var ErrNotRunning = errors.New("sloth: cannot stop, not running")

// Sloth is an Engine implementor that acts like it's working on something
// for some time.
type Sloth struct {
	mu       sync.Mutex
	running  bool
	stopreq  chan struct{}
	stopdone chan struct{}
	name     string

	startduration time.Duration // duration for which start will normally run.
	stopduration time.Duration // duration for which stop will normally take.
	starterror, stoperror       error // errors returned by start and stop.
}

// NewSloth returns a new *Sloth instance.
// Name is Sloth's name.
// StartDuration specifies how long to remain in Start. If 0, infinitely.
// StartDuration specifies how long to remain in Stop. If 0, infinitely.
// StartError if not nil, will be returned by Start.
// StopError if not nil, will be returned by Stop.
func NewSloth(Name string, startDuration, stopDuration time.Duration, startError, stopError error) *Sloth {
	return &Sloth{
		sync.Mutex{},
		false,
		make(chan struct{}),
		make(chan struct{}),
		Name,
		startDuration,
		stopDuration,
		startError,
		stopError,
	}
}

// Start starts the Sloth. If not stopped before StartDuration returns 
// startError else returns nil.
func (s *Sloth) Start(ctx context.Context) error {
	s.mu.Lock()
	s.running = true
	s.mu.Unlock()
	var stoptimer *time.Timer
	var stopchan <-chan time.Time
	for {
		select {
		case <-time.After(s.startduration):
			s.mu.Lock()
			s.running = false
			if stoptimer != nil {
				if !stoptimer.Stop() {
					<-stoptimer.C
				}
			}
			s.mu.Unlock()
			return s.starterror
		case <-stopchan:
			s.mu.Lock()
			s.running = false
			s.mu.Unlock()
			s.stopdone <- struct{}{}
			return nil
		case <-s.stopreq:
			stoptimer = time.NewTimer(s.stopduration)
			stopchan = stoptimer.C
		}
	}
}

// Stop stops the Sloth. It waits stopDuration before returning. It always 
// returns stopError.
func (s *Sloth) Stop(ctx context.Context) error {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return ErrNotRunning
	}
	s.mu.Unlock()
	s.stopreq <- struct{}{}
	<-s.stopdone
	return s.stoperror
}

var errStartComplete = errors.New("start completed")
var errStopComplete = errors.New("stop complete")

func TestSlothStartComplete(t *testing.T) {
	sloth := NewSloth("sloth", 5*time.Millisecond, 1*time.Millisecond, errStartComplete, nil)
	go func() {
		var err = sloth.Start(nil)
		if !errors.Is(err, errStartComplete) {
			t.Fatal(err)
		}
	}()
	time.Sleep(10 * time.Millisecond)
}

func TestSlothStopAfterStartComplete(t *testing.T) {
	sloth := NewSloth("sloth", 3*time.Millisecond, 10*time.Millisecond, nil, nil)
	go func() {
		var err error
		if err = sloth.Start(nil); err != nil {
			fmt.Println(err)
		}
	}()
	time.Sleep(5 * time.Millisecond)
	var err = sloth.Stop(nil)
	if !errors.Is(err, ErrNotRunning) {
		t.Fatal(err)
	}
}

func TestSlothStopStart(t *testing.T) {
	sloth := NewSloth("sloth", 5*time.Millisecond, 2*time.Millisecond, errStartComplete, errStopComplete)
	go func() {
		var err error
		if err = sloth.Start(nil); err != nil {
			t.Fatal(err)
		}
	}()
	time.Sleep(1 * time.Millisecond)
	var err = sloth.Stop(nil)
	if !errors.Is(err, errStopComplete) {
		t.Fatal(err)
	}
}
