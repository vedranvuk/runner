package runner

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func EngineCallback(name string, start bool, err error) {
	var verbose bool
	for _, v := range os.Args {
		if strings.HasPrefix(v, "-test.v") {
			verbose = true
			break
		}
	}
	if !verbose {
		return
	}
	if start {
		if err != nil {
			fmt.Printf("Callback(Start) '%s' returned an error: %v.\n", name, err)
		} else {
			fmt.Printf("Callback(Start) '%s' finished OK.\n", name)
		}
	} else {
		if err != nil {
			fmt.Printf("Callback(Stop)  '%s' returned an error: %v.\n", name, err)
		} else {
			fmt.Printf("Callback(Stop)  '%s' finished OK.\n", name)
		}
	}
}

func TestEmptyRunner(t *testing.T) {
	r := New(EngineCallback)
	if err := r.Start(nil); err != ErrNoEngines {
		t.Fatal("TestEmptyRunner failed.")
	}
}

func TestRunnerCompletion(t *testing.T) {
	r := New(EngineCallback)
	r.Register("sloth 1", NewSloth("sloth 1", 1*time.Millisecond, 1*time.Millisecond, nil, nil))
	r.Register("sloth 2", NewSloth("sloth 2", 2*time.Millisecond, 1*time.Millisecond, nil, nil))
	r.Register("sloth 3", NewSloth("sloth 3", 3*time.Millisecond, 1*time.Millisecond, nil, nil))
	r.Register("sloth 4", NewSloth("sloth 4", 4*time.Millisecond, 1*time.Millisecond, nil, nil))
	r.Register("sloth 5", NewSloth("sloth 5", 5*time.Millisecond, 1*time.Millisecond, nil, nil))
	r.Register("sloth 6", NewSloth("sloth 6", 6*time.Millisecond, 1*time.Millisecond, nil, nil))
	r.Register("sloth 7", NewSloth("sloth 7", 7*time.Millisecond, 1*time.Millisecond, nil, nil))
	r.Register("sloth 8", NewSloth("sloth 8", 8*time.Millisecond, 1*time.Millisecond, nil, nil))
	r.Register("sloth 9", NewSloth("sloth 9", 9*time.Millisecond, 1*time.Millisecond, nil, nil))
	r.Register("sloth 10", NewSloth("sloth 10", 10*time.Millisecond, 1*time.Millisecond, nil, nil))
	if err := r.Start(nil); err != nil {
		t.Fatal(err)
	}
}

var errStartError = errors.New("start error")

func TestRunnerStartError(t *testing.T) {
	r := New(EngineCallback)
	r.Register("sloth 1", NewSloth("sloth 1", 1*time.Millisecond, 1*time.Millisecond, nil, nil))
	r.Register("sloth 2", NewSloth("sloth 2", 2*time.Millisecond, 1*time.Millisecond, nil, nil))
	r.Register("sloth 3", NewSloth("sloth 3", 3*time.Millisecond, 1*time.Millisecond, nil, nil))
	r.Register("sloth 4", NewSloth("sloth 4", 4*time.Millisecond, 1*time.Millisecond, nil, nil))
	r.Register("sloth 5", NewSloth("sloth 5", 5*time.Millisecond, 1*time.Millisecond, nil, nil))
	r.Register("sloth 6", NewSloth("sloth 6", 6*time.Millisecond, 1*time.Millisecond, errStartError, nil))
	r.Register("sloth 7", NewSloth("sloth 7", 7*time.Millisecond, 1*time.Millisecond, nil, nil))
	r.Register("sloth 8", NewSloth("sloth 8", 8*time.Millisecond, 1*time.Millisecond, nil, nil))
	r.Register("sloth 9", NewSloth("sloth 9", 9*time.Millisecond, 1*time.Millisecond, nil, nil))
	r.Register("sloth 10", NewSloth("sloth 10", 10*time.Millisecond, 1*time.Millisecond, nil, nil))
	if err := r.Start(nil); err != errStartError {
		t.Fatal(err)
	}
}

func TestRunnerStopRequest(t *testing.T) {
	r := New(EngineCallback)
	r.Register("sloth 1", NewSloth("sloth 1", 1*time.Millisecond, 1*time.Millisecond, nil, nil))
	r.Register("sloth 2", NewSloth("sloth 2", 2*time.Millisecond, 1*time.Millisecond, nil, nil))
	r.Register("sloth 3", NewSloth("sloth 3", 3*time.Millisecond, 1*time.Millisecond, nil, nil))
	r.Register("sloth 4", NewSloth("sloth 4", 4*time.Millisecond, 1*time.Millisecond, nil, nil))
	r.Register("sloth 5", NewSloth("sloth 5", 5*time.Millisecond, 1*time.Millisecond, nil, nil))
	r.Register("sloth 6", NewSloth("sloth 6", 6*time.Millisecond, 1*time.Millisecond, nil, nil))
	r.Register("sloth 7", NewSloth("sloth 7", 7*time.Millisecond, 1*time.Millisecond, nil, nil))
	r.Register("sloth 8", NewSloth("sloth 8", 8*time.Millisecond, 1*time.Millisecond, nil, nil))
	r.Register("sloth 9", NewSloth("sloth 9", 9*time.Millisecond, 1*time.Millisecond, nil, nil))
	r.Register("sloth 10", NewSloth("sloth 10", 10*time.Millisecond, 1*time.Millisecond, nil, nil))
	go func() {
		if err := r.Start(nil); err != nil {
			t.Fatal(err)
		}
	}()
	time.Sleep(5 * time.Millisecond)
	if err := r.Stop(nil); err != nil {
		t.Fatal(err)
	}
}

func TestRunnerError(t *testing.T) {
	r := New(EngineCallback)
	r.Register("sloth 1", NewSloth("sloth 1", 10*time.Millisecond, 1*time.Millisecond, nil, nil))
	r.Register("sloth 2", NewSloth("sloth 2", 5*time.Millisecond, 1*time.Millisecond, errors.New("start failed"), nil))
	go func() {
		if err := r.Start(nil); err == nil {
			t.Fatal("Start error test failed")
		}
	}()
	time.Sleep(15 * time.Millisecond)
	if err := r.Stop(nil); !errors.Is(err, ErrIdle) {
		t.Fatal(err)
	}
}

// RudeEngine is an Engine whose Stop returns before Start.
type RudeEngine struct {
	stop chan struct{}
}

// NewRudeEngine returns a new RudeEngine instance.
func NewRudeEngine() *RudeEngine { return &RudeEngine{make(chan struct{})} }

// Start help.
func (re *RudeEngine) Start(ctx context.Context) error {
	<-re.stop
	time.Sleep(24 * time.Hour)
	return nil
}

// Stop help.
func (re *RudeEngine) Stop(ctx context.Context) error {
	defer func() { re.stop <- struct{}{} }()
	return nil
}

func TestStopReturnBeforeStart(t *testing.T) {
	re := New(EngineCallback)
	if err := re.Register("RudeEngine", NewRudeEngine()); err != nil {
		t.Fatal(err)
	}
	go func() {
		if err := re.Start(nil); err != nil {
			t.Fatal(err)
		}
	}()
	time.Sleep(1 * time.Millisecond)
	if err := re.Stop(nil); err != nil {
		t.Fatal(err)
	}
	if re.RunningEngines() > 0 {
		t.Fatal("Failed to discard a rude engine.")
	}
}

const ONE_MILLION_ITERATIONS = 1e6

func Test_ONE_MILLION_ITERATIONS(t *testing.T) {
	re := New(nil)
	for i := 0; i < ONE_MILLION_ITERATIONS; i++ {
		name := fmt.Sprintf("Sloth %d", i)
		re.Register(name, NewSloth(name, time.Duration(i)*time.Nanosecond, time.Duration(i)*time.Nanosecond, nil, nil))
	}
	if err := re.Start(nil); err != nil {
		t.Fatal(err)
	}
}
