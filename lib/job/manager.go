package job

import (
	"context"
	"fmt"
	"github.com/dustinevan/jogger/lib/cgroup"
	jogv1 "github.com/dustinevan/jogger/pkg/gen/jogger/v1"
	"github.com/google/uuid"
	"sync"
)

var ErrJobNotFound = fmt.Errorf("job not found")

// Manager is a job manager that keeps track of jobs by username and jobID.
// It also holds a context that the server uses to stop all jobs when during shut down
type Manager struct {
	// jobMap is a map[username]map[jobID]*Job
	jobMap map[string]*Job

	mu          sync.RWMutex
	shutdownCtx context.Context

	cgroupFSManager *cgroup.FSManager
}

// NewManager creates a new Manager
func NewManager(shutdownCtx context.Context) *Manager {
	return &Manager{
		jobMap:      make(map[string]*Job),
		shutdownCtx: shutdownCtx,
	}
}

// Start starts a new job and returns the jobID
func (m *Manager) Start(ctx context.Context, username string, cmd string, args ...string) (string, error) {
	jobID := uuid.NewString()

	// Add a new cgroup for the job
	cgroupFD, err := m.cgroupFSManager.AddGroup(jobID)
	if err != nil {
		return "", fmt.Errorf("starting job: %w", err)
	}
	defer m.scheduleCGroupCleanup(jobID)

	j, err := StartNewJob(m.shutdownCtx, cgroupFD, cmd, args...)
	if err != nil {
		return "", fmt.Errorf("starting job: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.jobMap[keyString(username, jobID)] = j

	return jobID, nil
}

// Stop sends a stop signal to a job that will eventually be respected
func (m *Manager) Stop(ctx context.Context, username string, jobID string) error {
	j, err := m.getJob(username, jobID)

	if err != nil {
		return fmt.Errorf("stopping job %s: %w", jobID, err)
	}
	j.Stop()

	return nil
}

// Status gets the status of a job
// Because stop signals are eventually respected, the internal state of a job process may not yet be
// reflected in the status. Eventually consistency is guaranteed, though, and delays mostly depend on
// the CommandWaitDelay constant in the job package.
func (m *Manager) Status(ctx context.Context, username string, jobID string) (jogv1.Status, error) {
	j, err := m.getJob(username, jobID)
	if err != nil {
		return jogv1.Status_STATUS_UNSPECIFIED, fmt.Errorf("getting job status: %w", err)
	}
	return j.Status(), nil
}

func (m *Manager) OutputStream(ctx context.Context, username string, jobID string) (<-chan []byte, error) {
	j, err := m.getJob(username, jobID)
	if err != nil {
		return nil, fmt.Errorf("streaming output: %w", err)
	}
	return j.OutputStream(ctx), nil
}

func (m *Manager) getJob(username, jobID string) (*Job, error) {
	var j *Job
	m.mu.RLock()
	j = m.jobMap[keyString(username, jobID)]
	m.mu.RUnlock()

	if j == nil {
		return nil, ErrJobNotFound
	}
	return j, nil
}

func keyString(username, jobID string) string {
	return jobID + "-" + username
}

// scheduleCGroupCleanup schedules the removal of a cgroup for a job
// cgroups can't be removed util the processes inside them have exited.
// at the system level, a cgroup is removed by removing the directory.
// before removing the directory the cgroup.events file must contain
// 'populated 0'. The RemoveGroup(jobID) method kicks off a goroutine
// the polls the cgroup.events file, and removes the directory once
// it reads populated 0. To reduce load, we don't kick off this
// goroutine until the job is done. This call kicks off a goroutine
// that Waits on the job, and then makes a call to RemoveGroup.
//
// Note that these goroutines don't need to also listen for a
// shutdown signal. This is because a shutdown of the system
// will trigger shutdown of all the jobs. There should be a buffer
// between CommandWaitDelay and the server shutdown timeout for all
// this cleanup to occur.
func (m *Manager) scheduleCGroupCleanup(jobID string) {
	m.cgroupFSManager.RemoveGroup(jobID)
}
