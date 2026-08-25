package provider_test

import (
	"bufio"
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"net"
	"os"
	"sync"
	"testing"

	"golang.org/x/crypto/ssh"
)

const (
	genericSSHUsername = "tfacc"
	genericSSHPassword = "tfacc-password"
)

// startGenericSSHFixture starts a transport-valid SSH endpoint whose command
// output intentionally cannot match an installed native discovery signature.
// Authentication and channel establishment succeed, so automatic discovery
// must select the SSH fallback rather than report an unreachable target.
func startGenericSSHFixture(t *testing.T) (host string, port int, username, password string) {
	t.Helper()

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating generic SSH fixture host key: %v", err)
	}
	signer, err := ssh.NewSignerFromKey(privateKey)
	if err != nil {
		t.Fatalf("creating generic SSH fixture signer: %v", err)
	}

	serverConfig := &ssh.ServerConfig{
		PasswordCallback: func(metadata ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
			if metadata.User() != genericSSHUsername || string(pass) != genericSSHPassword {
				return nil, fmt.Errorf("invalid fixture credentials")
			}
			return nil, nil
		},
	}
	serverConfig.AddHostKey(signer)

	bindAddress := os.Getenv("MISTERSHELL_ACC_GENERIC_SSH_BIND_ADDRESS")
	if bindAddress == "" {
		bindAddress = "127.0.0.1:0"
	}
	listener, err := net.Listen("tcp", bindAddress)
	if err != nil {
		t.Fatalf("listening for generic SSH fixture: %v", err)
	}

	var wg sync.WaitGroup
	var connections sync.Map
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			conn, acceptErr := listener.Accept()
			if acceptErr != nil {
				return
			}
			connections.Store(conn, struct{}{})
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer connections.Delete(conn)
				serveGenericSSHConnection(conn, serverConfig)
			}()
		}
	}()

	t.Cleanup(func() {
		_ = listener.Close()
		connections.Range(func(key, _ any) bool {
			_ = key.(net.Conn).Close()
			return true
		})
		wg.Wait()
	})

	tcpAddress := listener.Addr().(*net.TCPAddr)
	advertisedHost := os.Getenv("MISTERSHELL_ACC_GENERIC_SSH_HOST")
	if advertisedHost == "" {
		if tcpAddress.IP.IsUnspecified() {
			t.Fatal("MISTERSHELL_ACC_GENERIC_SSH_HOST is required when the generic SSH fixture binds to an unspecified address")
		}
		advertisedHost = tcpAddress.IP.String()
	}
	return advertisedHost, tcpAddress.Port, genericSSHUsername, genericSSHPassword
}

func serveGenericSSHConnection(conn net.Conn, config *ssh.ServerConfig) {
	defer conn.Close()

	_, channels, requests, err := ssh.NewServerConn(conn, config)
	if err != nil {
		return
	}
	go ssh.DiscardRequests(requests)

	for newChannel := range channels {
		if newChannel.ChannelType() != "session" {
			_ = newChannel.Reject(ssh.UnknownChannelType, "session channels only")
			continue
		}
		channel, channelRequests, err := newChannel.Accept()
		if err != nil {
			continue
		}
		go serveGenericSSHSession(channel, channelRequests)
	}
}

func serveGenericSSHSession(channel ssh.Channel, requests <-chan *ssh.Request) {
	shellStarted := false
	for request := range requests {
		switch request.Type {
		case "pty-req", "env", "window-change":
			_ = request.Reply(true, nil)
		case "exec":
			_ = request.Reply(true, nil)
			_, _ = channel.Write([]byte("unrecognized target\n"))
			_, _ = channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{Status: 0}))
			_ = channel.Close()
			return
		case "shell":
			if shellStarted {
				_ = request.Reply(false, nil)
				continue
			}
			shellStarted = true
			_ = request.Reply(true, nil)
			go serveGenericSSHShell(channel)
		default:
			_ = request.Reply(false, nil)
		}
	}
	_ = channel.Close()
}

func serveGenericSSHShell(channel ssh.Channel) {
	defer channel.Close()
	_, _ = channel.Write([]byte("generic> "))
	scanner := bufio.NewScanner(channel)
	scanner.Split(scanSSHCommands)
	for scanner.Scan() {
		_, _ = channel.Write([]byte("unrecognized target\r\ngeneric> "))
	}
}

func scanSSHCommands(data []byte, atEOF bool) (advance int, token []byte, err error) {
	for i, b := range data {
		if b == '\n' || b == '\r' {
			for i+advance < len(data) && (data[i+advance] == '\n' || data[i+advance] == '\r') {
				advance++
			}
			return i + advance, data[:i], nil
		}
	}
	if atEOF && len(data) > 0 {
		return len(data), data, nil
	}
	return 0, nil, nil
}
