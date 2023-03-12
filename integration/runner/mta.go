package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path"
	"strings"
	"sync"
	"syscall"
	"time"
)

type MTA struct {
	path       string
	Port       uint16
	cmd        *exec.Cmd
	dir        string
	tags       []string
	config     *Config
	wg         sync.WaitGroup
	once       sync.Once
	m          sync.Mutex
	failedTest bool
}

func NewMTA(path string, port uint16, config *Config) (*MTA, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	tagsCmd := exec.CommandContext(ctx, path, "tags")
	out, err := tagsCmd.Output()
	cancel()
	if err != nil {
		return nil, fmt.Errorf("executing %s tags failed: %w", path, err)
	}
	tags := removeEmptyOrDuplicates(tagsSplit.Split(string(out), -1))
	if len(tags) == 0 {
		return nil, nil
	}
	return &MTA{
		path:   path,
		Port:   port,
		tags:   tags,
		config: config,
	}, nil
}

func (m *MTA) String() string {
	return fmt.Sprintf("%s (%s)", m.path, strings.Join(m.tags, ", "))
}

func (m *MTA) HasTag(tag string) bool {
	for _, t := range m.tags {
		if t == tag {
			return true
		}
	}
	return false
}

func (m *MTA) MarkFailedTest() {
	m.m.Lock()
	defer m.m.Unlock()
	m.failedTest = true
}

func (m *MTA) Start() error {
	m.dir = path.Join(m.config.ScratchDir, fmt.Sprintf("mta-%d", m.Port))
	err := os.Mkdir(m.dir, 0755)
	if err != nil && !os.IsExist(err) {
		return err
	}
	m.cmd = exec.Command(m.path, "start",
		"-mtaPort", fmt.Sprintf("%d", m.Port),
		"-receiverPort", fmt.Sprintf("%d", m.config.ReceiverPort),
		"-milterPort", fmt.Sprintf("%d", m.config.MilterPort),
		"-scratchDir", m.dir,
	)
	for _, t := range m.tags {
		if strings.HasPrefix(t, "sleep-") {
			d, err := time.ParseDuration(t[6:])
			if err != nil {
				return err
			}
			defer func(d time.Duration) { time.Sleep(d) }(d)
			break
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.wg.Add(1)
	go func() {
		b, err := m.cmd.CombinedOutput()
		failed := !IsExpectedExitErr(err)
		if failed {
			LevelTwoLogger.Print(err)
		}
		m.m.Lock()
		failedTest := m.failedTest
		m.m.Unlock()
		if failed || failedTest {
			LevelTwoLogger.Printf("MTA %s\n%s", m.path, b)
		}
		m.wg.Done()
		cancel()
	}()
	err = WaitForPort(ctx, m.Port)
	cancel()
	if err != nil {
		m.Stop()
		return err
	}
	LevelTwoLogger.Printf("MTA %s ready", m.path)
	return nil
}

func (m *MTA) Stop() {
	m.once.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		b, _ := exec.CommandContext(ctx, m.path, "stop",
			"-mtaPort", fmt.Sprintf("%d", m.Port),
			"-receiverPort", fmt.Sprintf("%d", m.config.ReceiverPort),
			"-milterPort", fmt.Sprintf("%d", m.config.MilterPort),
			"-scratchDir", m.dir,
		).CombinedOutput()
		cancel()
		if m.cmd != nil && m.cmd.Process != nil {
			_ = m.cmd.Process.Signal(syscall.SIGTERM)
			m.wg.Wait()
			m.cmd = nil
		}
		m.m.Lock()
		failedTest := m.failedTest
		m.m.Unlock()
		if failedTest {
			LevelTwoLogger.Printf("MTA %s stop\n%s", m.path, b)
		}
	})
}
