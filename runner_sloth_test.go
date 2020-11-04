package runner

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

// Sloth is an Engine implementor that acts like it's
// working on something for some time.
type Sloth struct {
	mu       sync.Mutex
	running  bool
	stopreq  chan struct{}
	stopdone chan struct{}
	name     string

	startduration, stopduration time.Duration
	starterror, stoperror       error
}

// NewSloth returns a new *Sloth instance.
// Name is SLoth's name.
// StartDuration specifies how long to remain in Start. If 0, infinitely.
// StartDuration specifies how long to remain in Stop. If 0, infinitely.
// StartError if not nil, will be returned, prefixed by Sloth error by Start.
// StopError if not nil, will be returned, prefixed by Sloth error by Stop.
func NewSloth(Name string, StartDuration, StopDuration time.Duration, StartError, StopError error) *Sloth {
	return &Sloth{
		sync.Mutex{},
		false,
		make(chan struct{}),
		make(chan struct{}),
		Name,
		StartDuration,
		StopDuration,
		StartError,
		StopError,
	}
}

// Start starts the sloth.
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

var errNotRunning = errors.New("sloth: cannot stop, not running")

// Stop stops the sloth.
func (s *Sloth) Stop(ctx context.Context) error {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return errNotRunning
	}
	s.mu.Unlock()
	s.stopreq <- struct{}{}
	<-s.stopdone
	return s.stoperror
}

var errStartComplete = errors.New("start completed")
var errStopComplete = errors.New("stop complete")

func TestSlothCompleteStart(t *testing.T) {
	sloth := NewSloth("sloth", 5*time.Millisecond, 1*time.Millisecond, errStartComplete, nil)
	go func() {
		err := sloth.Start(nil)
		if !errors.Is(err, errStartComplete) {
			t.Fatal(err)
		}
	}()
	time.Sleep(10 * time.Millisecond)
}

func TestSlothStopAfterStartComplete(t *testing.T) {
	sloth := NewSloth("sloth", 3*time.Millisecond, 10*time.Millisecond, nil, nil)
	go func() {
		if err := sloth.Start(nil); err != nil {
			fmt.Println(err)
		}
	}()
	time.Sleep(5 * time.Millisecond)
	err := sloth.Stop(nil)
	if !errors.Is(err, errNotRunning) {
		t.Fatal(err)
	}
}

func TestSlothStopStart(t *testing.T) {
	sloth := NewSloth("sloth", 5*time.Millisecond, 2*time.Millisecond, errStartComplete, errStopComplete)
	go func() {
		if err := sloth.Start(nil); err != nil {
			t.Fatal(err)
		}
	}()
	time.Sleep(1 * time.Millisecond)
	err := sloth.Stop(nil)
	if !errors.Is(err, errStopComplete) {
		t.Fatal(err)
	}
}
