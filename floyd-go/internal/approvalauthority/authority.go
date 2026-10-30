// Package approvalauthority stores deletion approvals behind an OS identity
// boundary. Only an administrator-pinned engine executable may access records.
package approvalauthority

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

const maxMessage = 4 << 20

// Executable is an administrator-pinned, immutable engine image.
type Executable struct {
	Path   string
	SHA256 string
}

// Config is service configuration, never workspace or model configuration.
type Config struct {
	SocketPath         string
	StateDir           string
	AllowedUIDs        []uint32
	AllowedExecutables []Executable
}

type challenge struct {
	Nonce                 string `json:"nonce"`
	AccountPolicyVerified bool   `json:"account_policy_verified"`
	AccountUID            uint32 `json:"account_uid"`
}

type request struct {
	Nonce     string `json:"nonce"`
	Operation string `json:"operation"`
	Namespace string `json:"namespace"`
	Session   string `json:"session"`
	Revision  uint64 `json:"revision"`
	Data      []byte `json:"data,omitempty"`
}
type response struct {
	AccountPolicyVerified bool   `json:"account_policy_verified"`
	AccountUID            uint32 `json:"account_uid"`
	Revision              uint64 `json:"revision"`
	Data                  []byte `json:"data,omitempty"`
	Error                 string `json:"error,omitempty"`
}
type storedRecord struct {
	Revision uint64 `json:"revision"`
	Data     []byte `json:"data"`
}

// Client verifies the server's kernel UID before sending any approval data.
type Client struct {
	SocketPath string
	OwnerUID   uint32
}

func NewClient(socketPath string, ownerUID uint32) *Client {
	return &Client{SocketPath: socketPath, OwnerUID: ownerUID}
}

func (c *Client) Read(ctx context.Context, namespace, session string) ([]byte, uint64, error) {
	result, err := c.exchange(ctx, request{Operation: "read", Namespace: namespace, Session: session})
	return result.Data, result.Revision, err
}

func (c *Client) CompareAndSwap(ctx context.Context, namespace, session string, revision uint64, data []byte) error {
	_, err := c.exchange(ctx, request{Operation: "write", Namespace: namespace, Session: session, Revision: revision, Data: data})
	return err
}

func (c *Client) exchange(ctx context.Context, req request) (response, error) {
	var result response
	if err := clientIdentityError; err != nil {
		return result, err
	}
	if c.OwnerUID == uint32(os.Getuid()) {
		return result, errors.New("approval authority must run as a separate OS identity")
	}
	localPolicy, err := validateLocalEngineAccount()
	if err != nil {
		return result, err
	}
	if err := validateSocket(c.SocketPath, c.OwnerUID); err != nil {
		return result, err
	}
	conn, err := (&net.Dialer{}).DialContext(ctx, "unix", c.SocketPath)
	if err != nil {
		return result, fmt.Errorf("connect approval authority: %w", err)
	}
	defer conn.Close()
	unixConn, ok := conn.(*net.UnixConn)
	if !ok {
		return result, errors.New("approval authority requires a Unix socket")
	}
	peer, err := peerIdentity(unixConn)
	if err != nil {
		return result, err
	}
	if peer.uid != c.OwnerUID || peer.uid == uint32(os.Getuid()) {
		return result, errors.New("approval authority server identity mismatch")
	}
	deadline := time.Now().Add(10 * time.Second)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return result, err
	}
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	return exchangeAuthenticated(conn, req, localPolicy)
}

// exchangeAuthenticated is called only after kernel root-peer verification.
func exchangeAuthenticated(conn net.Conn, req request, localPolicy localPolicyStatus) (response, error) {
	var result response
	var greeting challenge
	if err := json.NewDecoder(io.LimitReader(conn, maxMessage)).Decode(&greeting); err != nil {
		return result, err
	}
	if !greeting.AccountPolicyVerified || greeting.AccountUID != uint32(os.Getuid()) {
		if localPolicy == localPolicyDeferred {
			return response{}, errors.New("local account policy is unknown and the approval authority did not verify it")
		}
		return response{}, errors.New("approval authority did not verify account policy")
	}
	if len(greeting.Nonce) != 64 {
		return result, errors.New("approval authority challenge unavailable")
	}
	req.Nonce = greeting.Nonce
	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return result, err
	}
	if err := json.NewDecoder(io.LimitReader(conn, maxMessage)).Decode(&result); err != nil {
		return result, fmt.Errorf("read approval authority response: %w", err)
	}
	if result.Error != "" {
		return response{}, errors.New(result.Error)
	}
	if !result.AccountPolicyVerified || result.AccountUID != uint32(os.Getuid()) {
		return response{}, errors.New("approval authority response lacks verified account policy")
	}
	return result, nil
}

type server struct {
	config    Config
	mu        sync.Mutex
	authorize func(*net.UnixConn) error
}

// Serve runs the protected broker. The service account must differ from every
// allowed engine account; all pinned code and service paths must be immutable
// to those engine accounts. Unsupported peer-authentication platforms refuse.
func Serve(ctx context.Context, config Config) error {
	if err := validateConfig(config); err != nil {
		return err
	}
	unlock, err := lockStorage(config.StateDir)
	if err != nil {
		return err
	}
	defer unlock()
	if err := quarantineStaleSocket(config.SocketPath); err != nil {
		return err
	}
	s := &server{config: config}
	s.authorize = s.authenticate
	return s.serve(ctx)
}

func (s *server) serve(ctx context.Context) error {
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: s.config.SocketPath, Net: "unix"})
	if err != nil {
		return err
	}
	listener.SetUnlinkOnClose(false)
	defer listener.Close()
	// Connecting conveys no authority: each connection is authenticated using
	// kernel credentials and the exact pinned executable before parsing input.
	if err := os.Chmod(s.config.SocketPath, 0o666); err != nil {
		return err
	}
	stop := context.AfterFunc(ctx, func() { _ = listener.Close() })
	defer stop()
	var workers sync.WaitGroup
	defer workers.Wait()
	for {
		conn, err := listener.AcceptUnix()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		workers.Add(1)
		go func() { defer workers.Done(); defer conn.Close(); s.handle(conn) }()
	}
}

func (s *server) handle(conn *net.UnixConn) {
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	peer, err := peerIdentity(conn)
	if err != nil {
		return
	}
	if err := s.authorize(conn); err != nil {
		slog.Warn("Approval authority rejected peer", "uid", peer.uid, "pid", peer.pid, "error", err)
		_ = json.NewEncoder(conn).Encode(response{Error: "approval authority rejected peer identity"})
		return
	}
	verifiedPeer, err := peerIdentity(conn)
	if err != nil || verifiedPeer != peer {
		return
	}
	reader, err := authenticatedReader(conn, peer)
	if err != nil {
		return
	}
	// The challenge is generated only after the live executable is trusted.
	// Prequeued requests sent by a process before exec cannot know this value.
	var nonce [32]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return
	}
	expected := hex.EncodeToString(nonce[:])
	if json.NewEncoder(conn).Encode(challenge{Nonce: expected, AccountPolicyVerified: true, AccountUID: peer.uid}) != nil {
		return
	}
	var req request
	decoder := json.NewDecoder(io.LimitReader(reader, maxMessage))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		_ = json.NewEncoder(conn).Encode(response{Error: "invalid approval authority request"})
		return
	}
	after, err := peerIdentity(conn)
	if err != nil || after != peer || req.Nonce != expected {
		slog.Warn("Approval authority request sender changed", "uid", peer.uid, "pid", peer.pid, "error", err)
		_ = json.NewEncoder(conn).Encode(response{Error: "approval authority rejected request sender"})
		return
	}
	if err := s.authorize(conn); err != nil {
		slog.Warn("Approval authority rejected request sender", "uid", peer.uid, "pid", peer.pid, "error", err)
		_ = json.NewEncoder(conn).Encode(response{Error: "approval authority rejected request sender"})
		return
	}
	finalPeer, err := peerIdentity(conn)
	if err != nil || finalPeer != peer {
		return
	}
	result := s.apply(peer.uid, req)
	result.AccountPolicyVerified = true
	result.AccountUID = peer.uid
	_ = json.NewEncoder(conn).Encode(result)
}

func (s *server) apply(uid uint32, req request) response {
	if req.Namespace == "" || req.Session == "" || len(req.Namespace) > 4096 || len(req.Session) > 4096 {
		return response{Error: "invalid approval record identity"}
	}
	if req.Operation != "read" && req.Operation != "write" {
		return response{Error: "invalid approval authority operation"}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	// Both identifiers are hashed by the broker. No request can supply a path.
	ns := sha256.Sum256([]byte(strconv.FormatUint(uint64(uid), 10) + "\x00" + req.Namespace))
	sid := sha256.Sum256([]byte(req.Session))
	dir := filepath.Join(s.config.StateDir, hex.EncodeToString(ns[:]))
	file := filepath.Join(dir, hex.EncodeToString(sid[:])+".json")
	var record storedRecord
	bytes, err := os.ReadFile(file)
	if err == nil {
		if json.Unmarshal(bytes, &record) != nil || record.Revision == 0 {
			return response{Error: "invalid protected approval record"}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return response{Error: "cannot read protected approval record"}
	}
	if req.Operation == "read" {
		return response{Revision: record.Revision, Data: record.Data}
	}
	if req.Revision != record.Revision {
		return response{Error: "approval record revision conflict"}
	}
	if record.Revision == ^uint64(0) {
		return response{Error: "approval record revision exhausted"}
	}
	if len(req.Data) == 0 || len(req.Data) > maxMessage/2 {
		return response{Error: "invalid approval record data"}
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return response{Error: "cannot create protected approval record directory"}
	}
	next := storedRecord{Revision: record.Revision + 1, Data: req.Data}
	bytes, err = json.Marshal(next)
	if err != nil {
		return response{Error: "cannot encode protected approval record"}
	}
	tmp, err := os.CreateTemp(dir, ".approval-*")
	if err != nil {
		return response{Error: "cannot create protected approval record"}
	}
	tempName := tmp.Name()
	if _, err = tmp.Write(bytes); err == nil {
		err = tmp.Sync()
	}
	closeErr := tmp.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return response{Error: "cannot persist protected approval record"}
	}
	if err := os.Rename(tempName, file); err != nil {
		return response{Error: "cannot publish protected approval record"}
	}
	directory, err := os.Open(dir)
	if err != nil {
		return response{Error: "cannot open protected approval directory"}
	}
	syncErr := directory.Sync()
	closeErr = directory.Close()
	if syncErr != nil || closeErr != nil {
		return response{Error: "cannot sync protected approval directory"}
	}
	return response{Revision: next.Revision}
}
