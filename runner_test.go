package runner

import (
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

func TestRunnerCompletion(t *testing.T) {
	r := New(EngineCallback)
	r.Register("sloth 1", NewSloth("sloth 1", 5*time.Millisecond, 1*time.Millisecond, nil, nil), nil)
	r.Register("sloth 2", NewSloth("sloth 2", 10*time.Millisecond, 1*time.Millisecond, nil, nil), nil)
	go func() {
		if err := r.Start(nil, nil); err != nil {
			t.Fatal(err)
		}
	}()
	time.Sleep(15 * time.Millisecond)
	if err := r.Stop(); !errors.Is(err, ErrAlreadyStopped) {
		t.Fatal(err)
	}
}

func TestRunnerStopRequest(t *testing.T) {
	r := New(EngineCallback)
	r.Register("sloth 1", NewSloth("sloth 1", 1*time.Millisecond, 1*time.Millisecond, nil, nil), nil)
	r.Register("sloth 2", NewSloth("sloth 2", 2*time.Millisecond, 1*time.Millisecond, nil, nil), nil)
	r.Register("sloth 3", NewSloth("sloth 3", 3*time.Millisecond, 1*time.Millisecond, nil, nil), nil)
	r.Register("sloth 4", NewSloth("sloth 4", 4*time.Millisecond, 1*time.Millisecond, nil, nil), nil)
	r.Register("sloth 5", NewSloth("sloth 5", 5*time.Millisecond, 1*time.Millisecond, nil, nil), nil)
	r.Register("sloth 6", NewSloth("sloth 6", 6*time.Millisecond, 1*time.Millisecond, nil, nil), nil)
	r.Register("sloth 7", NewSloth("sloth 7", 7*time.Millisecond, 1*time.Millisecond, nil, nil), nil)
	r.Register("sloth 8", NewSloth("sloth 8", 8*time.Millisecond, 1*time.Millisecond, nil, nil), nil)
	r.Register("sloth 9", NewSloth("sloth 9", 9*time.Millisecond, 1*time.Millisecond, nil, nil), nil)
	r.Register("sloth 10", NewSloth("sloth 10", 10*time.Millisecond, 1*time.Millisecond, nil, nil), nil)
	go func() {
		if err := r.Start(nil, nil); err != nil {
			t.Fatal(err)
		}
	}()
	time.Sleep(5 * time.Millisecond)
	if err := r.Stop(); err != nil {
		t.Fatal(err)
	}
}
