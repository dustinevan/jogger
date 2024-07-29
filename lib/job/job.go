package job

import (
	"context"
	"errors"
	"fmt"
	jogv1 "github.com/dustinevan/jogger/pkg/gen/jogger/v1"
	"golang.org/x/sys/unix"
	"os/exec"
	"sync/atomic"
	"syscall"
	"time"
)

// CommandWaitDelay is the amount of time to wait for a canceled Job to shut down before sending a SIGKILL
const CommandWaitDelay = 10 * time.Second

type Job struct {
	cmd      *exec.Cmd
	streamer *OutputStreamer

	cancel context.CancelFunc
	status *atomic.Value

	// doneCtx is a context that is closed when the job is done
	// it is used to signal to the callers of Wait() that the job is done
	doneCtx    context.Context
	markAsDone context.CancelFunc
}

// StartNewJob creates a new job, starts it, and returns a reference to it
// If the underlying cmd.Start() call fails, an error is returned as well as
// a nil pointer to ensure that the job is thrown away. This ensures that
// callers cannot call exported methods on jobs that cannot be started.
func StartNewJob(shutdownCtx context.Context, cgroupFD int, name string, args ...string) (*Job, error) {
	j := newJob(shutdownCtx, cgroupFD, name, args...)
	err := j.start()
	if err != nil {
		j.markAsDone()
		return nil, err
	}
	j.status.Store(jogv1.Status_RUNNING)
	return j, nil
}

func newJob(shutdownCtx context.Context, cgroupFD int, name string, args ...string) *Job {
	streamer := NewOutputStreamer()

	// doneCtx is a context that is closed when the job is done
	// it is used to signal to the callers of Wait() that the job is done
	doneCtx, markAsDone := context.WithCancel(context.Background())

	ctx, cancel := context.WithCancel(shutdownCtx)

	cmd := exec.CommandContext(ctx, name, args...)

	cmd.Cancel = func() error {
		// Internally, exec.Cmd depends on the error returned by the Signal call.
		// Any error handling added here should be done with that in mind.
		return cmd.Process.Signal(unix.SIGTERM)
	}
	cmd.WaitDelay = CommandWaitDelay
	cmd.Stdout = streamer
	cmd.Stderr = streamer

	// Set the cgroup file descriptor on the command
	attrs := cmd.SysProcAttr
	attrs.UseCgroupFD = true
	attrs.CgroupFD = cgroupFD
	cmd.SysProcAttr = attrs

	return &Job{
		cmd:        cmd,
		streamer:   streamer,
		cancel:     cancel,
		status:     &atomic.Value{},
		doneCtx:    doneCtx,
		markAsDone: markAsDone,
	}
}

func (j *Job) start() error {
	err := j.cmd.Start()
	if err != nil {
		return err
	}

	go func() {
		defer j.streamer.CloseWriter()
		j.setDoneStatus(j.cmd.Wait())
	}()

	return nil
}

// Stop calls the cancel function on the exec.Cmd internal context. Jobs are stopped
// asynchronously, and will be sent a SIGKILL after the CommandWaitDelay has passed.
func (j *Job) Stop() {
	j.cancel()
}

// Status returns the current status of the job
func (j *Job) Status() jogv1.Status {
	s := j.status.Load()
	// panic if the status was not set or was set to an unexpected type
	// if this happens it is a bug.
	if s == nil {
		panic(fmt.Sprintf("no job status was set: %+v", j.cmd))
	}
	if jogStatus, ok := s.(jogv1.Status); !ok {
		panic(fmt.Sprintf("job status was not of type jogv1.Status: %T, %+v", s, s))
	} else {
		return jogStatus
	}
}

// Jogger tracks 4 end states
// Completed: The job completed successfully
// Failed: The job failed on its own
// Stopped: The job was stopped by the user. If the binary supports graceful shutdown, it was given that chance.
// Killed: The job was killed by the system. Depending on the software that was run, this may have created an inconsistent state.
// Jogger differentiates between Stopped and Killed to give the user a better understanding of what happened.
func (j *Job) setDoneStatus(err error) {
	defer j.markAsDone()
	if err == nil {
		j.status.Store(jogv1.Status_COMPLETED)
		return
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		// Internally, ExitError holds information about the last signal it received
		sig := exitErr.Sys().(syscall.WaitStatus).Signal()
		switch sig {
		case unix.SIGTERM:
			j.status.Store(jogv1.Status_STOPPED)
		case unix.SIGKILL:
			j.status.Store(jogv1.Status_KILLED)
		default:
			j.status.Store(jogv1.Status_FAILED)
		}
	} else {
		j.status.Store(jogv1.Status_FAILED)
	}
}

// OutputStream returns a channel that streams the output of the job
func (j *Job) OutputStream(ctx context.Context) <-chan []byte {
	return j.streamer.NewStream(ctx)
}

// Wait blocks until the job is done
func (j *Job) Wait() {
	<-j.doneCtx.Done()
}
