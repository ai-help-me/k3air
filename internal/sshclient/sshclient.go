package sshclient

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"time"

	"github.com/pkg/sftp"
	progressbar "github.com/schollz/progressbar/v3"
	"golang.org/x/crypto/ssh"
)

type Client struct {
	addr   string
	client *ssh.Client
	sftp   *sftp.Client
}

type Auth struct {
	Password string
	KeyPath  string
}

func New(host string, port int, username string, auth Auth) (*Client, error) {
	if username == "" {
		slog.Info("username is empty, use root")
		username = "root"
	}

	slog.Debug("establishing SSH connection", "host", host, "port", port, "user", username)

	var authMethods []ssh.AuthMethod
	var authMethod string
	if auth.Password != "" {
		authMethods = append(authMethods, ssh.Password(auth.Password))
		authMethod = "password"
	}
	if auth.KeyPath != "" {
		key, err := os.ReadFile(auth.KeyPath)
		if err != nil {
			return nil, err
		}
		signer, err := ssh.ParsePrivateKey(key)
		if err != nil {
			return nil, err
		}
		authMethods = append(authMethods, ssh.PublicKeys(signer))
		authMethod = "key"
	}

	cfg := &ssh.ClientConfig{
		User:            username,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         20 * time.Second,
	}
	addr := net.JoinHostPort(host, fmt.Sprintf("%d", port))

	c, err := ssh.Dial("tcp", addr, cfg)
	if err != nil {
		slog.Debug("SSH connection failed", "error", err)
		return nil, err
	}

	slog.Debug("SSH connection established", "auth", authMethod)

	s, err := sftp.NewClient(c)
	if err != nil {
		c.Close()
		return nil, err
	}
	return &Client{addr: addr, client: c, sftp: s}, nil
}

func (c *Client) Addr() string {
	return c.addr
}

func (c *Client) Close() {
	if c.sftp != nil {
		c.sftp.Close()
	}
	if c.client != nil {
		c.client.Close()
	}
}

func (c *Client) Run(cmd string) (string, string, error) {
	s, err := c.client.NewSession()
	if err != nil {
		return "", "", err
	}
	defer s.Close()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	s.Stdout = &stdout
	s.Stderr = &stderr
	err = s.Run(cmd)
	return stdout.String(), stderr.String(), err
}

func (c *Client) Upload(localPath, remotePath string, progress bool) error {
	lf, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer lf.Close()
	rf, err := c.sftp.Create(remotePath)
	if err != nil {
		return err
	}
	defer rf.Close()
	if progress {
		stat, e := lf.Stat()
		if e != nil {
			return e
		}
		bar := progressbar.NewOptions(int(stat.Size()),
			progressbar.OptionShowBytes(true),
			progressbar.OptionSetDescription("upload "+remotePath))
		_, err = io.Copy(io.MultiWriter(rf, bar), lf)
		fmt.Println() // Ensure newline after progress bar
	} else {
		_, err = io.Copy(rf, lf)
	}
	return err
}

func (c *Client) UploadBytes(data []byte, remotePath string) error {
	rf, err := c.sftp.Create(remotePath)
	if err != nil {
		return err
	}
	defer rf.Close()
	_, err = rf.Write(data)
	return err
}

func (c *Client) MkdirAll(remotePath string) error {
	return c.sftp.MkdirAll(remotePath)
}

func (c *Client) Download(remotePath, localPath string) error {
	rf, err := c.sftp.Open(remotePath)
	if err != nil {
		return err
	}
	defer rf.Close()
	lf, err := os.Create(localPath)
	if err != nil {
		return err
	}
	defer lf.Close()
	_, err = io.Copy(lf, rf)
	return err
}

func (c *Client) DownloadBytes(remotePath string) ([]byte, error) {
	rf, err := c.sftp.Open(remotePath)
	if err != nil {
		return nil, err
	}
	defer rf.Close()
	return io.ReadAll(rf)
}

// GetFileSize returns the size of a remote file
func (c *Client) GetFileSize(remotePath string) (int64, error) {
	rf, err := c.sftp.Open(remotePath)
	if err != nil {
		return 0, err
	}
	defer rf.Close()
	fi, err := rf.Stat()
	if err != nil {
		return 0, err
	}
	return fi.Size(), nil
}
