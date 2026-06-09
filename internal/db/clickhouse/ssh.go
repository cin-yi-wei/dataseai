package clickhouse

import (
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
	"time"

	gossh "golang.org/x/crypto/ssh"

	"github.com/conray/dataseai/internal/db"
)

func (CH) RegisterSSHDialer(cfg db.SSHConfig, in db.DSNInput) (string, func(), error) {
	if cfg.IsZero() {
		return "", nil, fmt.Errorf("ssh: host/user required")
	}
	client, err := dialSSH(cfg)
	if err != nil {
		return "", nil, err
	}

	target := net.JoinHostPort(in.Host, strconv.Itoa(in.Port))
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		_ = client.Close()
		return "", nil, fmt.Errorf("local listener: %w", err)
	}
	localAddr := listener.Addr().String()

	key := chSSHReg.nextKey()
	chSSHReg.put(key, &chSSHEntry{client: client, listener: listener})

	go func() {
		for {
			local, err := listener.Accept()
			if err != nil {
				return
			}
			go func(local net.Conn) {
				remote, err := client.Dial("tcp", target)
				if err != nil {
					_ = local.Close()
					return
				}
				go func() { _, _ = io.Copy(remote, local) }()
				go func() {
					_, _ = io.Copy(local, remote)
					_ = local.Close()
					_ = remote.Close()
				}()
			}(local)
		}
	}()

	return localAddr, func() { closeChSSH(key) }, nil
}

func dialSSH(cfg db.SSHConfig) (*gossh.Client, error) {
	port := cfg.Port
	if port == 0 {
		port = 22
	}
	var auth gossh.AuthMethod
	if cfg.PrivateKey != "" {
		var signer gossh.Signer
		var err error
		if cfg.KeyPassphrase != "" {
			signer, err = gossh.ParsePrivateKeyWithPassphrase([]byte(cfg.PrivateKey), []byte(cfg.KeyPassphrase))
		} else {
			signer, err = gossh.ParsePrivateKey([]byte(cfg.PrivateKey))
		}
		if err != nil {
			return nil, fmt.Errorf("ssh key parse: %w", err)
		}
		auth = gossh.PublicKeys(signer)
	} else {
		auth = gossh.Password(cfg.Password)
	}
	clientCfg := &gossh.ClientConfig{
		User:            cfg.User,
		Auth:            []gossh.AuthMethod{auth},
		HostKeyCallback: gossh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}
	addr := net.JoinHostPort(cfg.Host, strconv.Itoa(port))
	client, err := gossh.Dial("tcp", addr, clientCfg)
	if err != nil {
		return nil, fmt.Errorf("ssh dial %s: %w", addr, err)
	}
	return client, nil
}

type chSSHEntry struct {
	client   *gossh.Client
	listener net.Listener
}

type chSSHRegistry struct {
	mu sync.Mutex
	m  map[string]*chSSHEntry
	n  uint64
}

func (r *chSSHRegistry) put(key string, e *chSSHEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.m[key] = e
}

func (r *chSSHRegistry) take(key string) *chSSHEntry {
	r.mu.Lock()
	defer r.mu.Unlock()
	e := r.m[key]
	delete(r.m, key)
	return e
}

func (r *chSSHRegistry) nextKey() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.n++
	return strconv.FormatUint(r.n, 10)
}

var chSSHReg = &chSSHRegistry{m: map[string]*chSSHEntry{}}

func closeChSSH(key string) {
	e := chSSHReg.take(key)
	if e == nil {
		return
	}
	_ = e.listener.Close()
	_ = e.client.Close()
}
