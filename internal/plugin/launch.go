package plugin

import (
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

var errNoManifest = errors.New("plugin: no manifest file found")

func (m *Manager) launchPlugin(dirPath string) (*pluginInstance, net.Conn, error) {
	manifest, err := loadManifest(dirPath)
	if err != nil {
		return nil, nil, err
	}
	if len(manifest.Command) == 0 {
		return nil, nil, fmt.Errorf("manifest in %q has no command", dirPath)
	}

	pluginID := filepath.Base(dirPath)
	if manifest.Name != "" {
		pluginID = manifest.Name
	}

	socketPath := filepath.Join(os.TempDir(), "vdb-"+pluginID+".sock")
	_ = os.Remove(socketPath)

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, nil, fmt.Errorf("listen on %q: %w", socketPath, err)
	}

	env := buildEnv(manifest.Env, socketPath)
	cmd := exec.Command(manifest.Command[0], manifest.Command[1:]...)
	cmd.Env = env
	cmd.Dir = dirPath
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		listener.Close()
		_ = os.Remove(socketPath)
		return nil, nil, fmt.Errorf("start process: %w", err)
	}

	rawConn, err := acceptWithTimeout(listener, m.timeout)
	listener.Close()
	_ = os.Remove(socketPath)
	if err != nil {
		_ = cmd.Process.Kill()
		return nil, nil, fmt.Errorf("plugin %q did not connect within %v: %w", pluginID, m.timeout, err)
	}

	return &pluginInstance{
		id:       pluginID,
		manifest: *manifest,
		cmd:      cmd,
	}, rawConn, nil
}

func acceptWithTimeout(l net.Listener, timeout time.Duration) (net.Conn, error) {
	type result struct {
		conn net.Conn
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		conn, err := l.Accept()
		ch <- result{conn, err}
	}()
	select {
	case r := <-ch:
		return r.conn, r.err
	case <-time.After(timeout):
		l.Close()
		return nil, fmt.Errorf("timeout after %v", timeout)
	}
}
