package cgroup

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

const gb = 1024 * 1024 * 1024

type FSManager struct {
	// controllers is a list of cgroup controllers to enable for job cgroups
	controllers []string

	// these fields can be configured by passing an FSManagerOption
	rootPath          string
	memoryTargetBytes int
	serverCGroupName  string

	// groups is a map of cgroup names to their directories
	groups map[string]*CGroup
	mu     sync.Mutex

	// shutdownCtx is a context that is closed when the server is shutting down
	shutdownCtx context.Context
}

type CGroup struct {
	dir          *os.File
	cgEventsFile string
}

// FD returns the file descriptor of the cgroup directory
func (d *CGroup) FD() int {
	return int(d.dir.Fd())
}

func NewFSManager(shutdownCtx context.Context, options ...FSManagerOption) (*FSManager, error) {
	cfg := defaultFSManagerConfig()
	for _, opt := range options {
		opt(&cfg)
	}
	fsm := &FSManager{
		controllers:       []string{"cpu", "memory", "io"},
		rootPath:          cfg.rootPath,
		memoryTargetBytes: cfg.targetMaxMemoryBytes,
		serverCGroupName:  cfg.serverCGroupName,
		groups:            make(map[string]*CGroup),
		shutdownCtx:       shutdownCtx,
	}

	if err := fsm.init(); err != nil {
		return nil, fmt.Errorf("failed to initialize cgroup manager: %w", err)
	}

	return fsm, nil
}

func (m *FSManager) AddGroup(name string) (int, error) {
	dirPath := filepath.Join(m.rootPath, m.serverCGroupName, name)
	if err := os.Mkdir(dirPath, 0755); err != nil {
		return -1, fmt.Errorf("failed to create cgroup directory: %w", err)
	}
	dir, err := os.Open(dirPath)
	if err != nil {
		return -1, fmt.Errorf("failed to open cgroup directory: %w", err)
	}
	memMax, err := os.Open(filepath.Join(dirPath, "memory.max"))
	if err != nil {
		rErr := os.Remove(dirPath)
		if rErr != nil {
			err = fmt.Errorf("failed to remove cgroup directory: %w", rErr)
		}
		return -1, fmt.Errorf("failed to open memory.max file: %w", err)
	}
	_, err = memMax.WriteString(fmt.Sprintf("%d", m.memoryTargetBytes/5))
	if err != nil {
		rErr := os.Remove(dirPath)
		if rErr != nil {
			err = fmt.Errorf("failed to remove cgroup directory: %w", rErr)
		}
		return -1, fmt.Errorf("failed to write to memory.max file: %w", err)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.groups[name] = &CGroup{
		dir:          dir,
		cgEventsFile: filepath.Join(dirPath, "cgroup.events"),
	}
	return int(dir.Fd()), nil
}

func (m *FSManager) RemoveGroup(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cg, ok := m.groups[name]
	if !ok {
		return fmt.Errorf("cgroup %s not found", name)
	}
	if err := cg.dir.Close(); err != nil {
		return fmt.Errorf("failed to close cgroup directory: %w", err)
	}
	delete(m.groups, name)
	return nil
}

// Add the default controllers to the root cgroup subtree_control file like this:
// `echo "+cpu +memory +io" > /sys/fs/cgroup/cgroup.subtree_control`
// `mkdir /sys/fs/cgroup/jogger`
// `echo "+cpu +memory +io" > /sys/fs/cgroup/jogger/cgroup.subtree_control`
func (m *FSManager) init() error {
	// enable the controllers in the root cgroup
	cmdString := fmt.Sprintf("echo \"+%s\" > %s", strings.Join(m.controllers, " +"), filepath.Join(m.rootPath, "cgroup.subtree_control"))
	cmd := exec.CommandContext(m.shutdownCtx, cmdString)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to enable controllers in root cgroup: cmdString=[%s]: %w", cmdString, err)
	}

	// create the server cgroup
	cmdString = fmt.Sprintf("mkdir %s", filepath.Join(m.rootPath, m.serverCGroupName))
	cmd = exec.CommandContext(m.shutdownCtx, cmdString)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create server cgroup: cmdString=[%s]: %w", cmdString, err)
	}

	// enable the controllers in the server cgroup
	cmdString = fmt.Sprintf("echo \"+%s\" > %s", strings.Join(m.controllers, " +"), filepath.Join(m.rootPath, m.serverCGroupName, "cgroup.subtree_control"))
	cmd = exec.CommandContext(m.shutdownCtx, cmdString)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to enable controllers in server cgroup: cmdString=[%s]: %w", cmdString, err)
	}
	return nil
}

type FSManagerOption func(*fSManagerConfig)

var (
	defaultCgroupRootPath       = "/sys/fs/cgroup"
	defaultServerCGroupName     = "jogger"
	defaultTargetMaxMemoryBytes = 4 * gb
)

type fSManagerConfig struct {
	rootPath             string
	serverCGroupName     string
	targetMaxMemoryBytes int
}

func defaultFSManagerConfig() fSManagerConfig {
	return fSManagerConfig{
		rootPath:             defaultCgroupRootPath,
		serverCGroupName:     defaultServerCGroupName,
		targetMaxMemoryBytes: defaultTargetMaxMemoryBytes,
	}
}

func WithRootPath(rootPath string) FSManagerOption {
	return func(cfg *fSManagerConfig) {
		cfg.rootPath = rootPath
	}
}

func WithServerCGroupName(serverCGroupName string) FSManagerOption {
	return func(cfg *fSManagerConfig) {
		cfg.serverCGroupName = serverCGroupName
	}
}

func WithTargetMaxMemoryBytes(targetMaxMemoryBytes int) FSManagerOption {
	return func(cfg *fSManagerConfig) {
		cfg.targetMaxMemoryBytes = targetMaxMemoryBytes
	}
}
