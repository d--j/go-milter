package milter

import (
	"context"
	"errors"
	"fmt"
	"net"
	"syscall"
	"testing"
	"testing/synctest"
	"time"
)

type acceptFuncListener struct {
	accept func() (net.Conn, error)
}

func (l *acceptFuncListener) Accept() (net.Conn, error) { return l.accept() }
func (l *acceptFuncListener) Close() error              { return nil }
func (l *acceptFuncListener) Addr() net.Addr            { return &net.TCPAddr{} }

func TestServer_AcceptResourceError(t *testing.T) {
	testServerAcceptResourceError(t, syscall.EMFILE)
}

func testServerAcceptResourceError(t *testing.T, errno error) {
	t.Helper()
	synctest.Test(t, func(t *testing.T) {
		serverConn, clientConn := net.Pipe()
		defer serverConn.Close()
		defer clientConn.Close()
		stopErr := errors.New("stop accepting")
		calls := 0
		ln := &acceptFuncListener{accept: func() (net.Conn, error) {
			calls++
			switch calls {
			case 1:
				return nil, &net.OpError{Op: "accept", Net: "tcp", Err: fmt.Errorf("wrapped: %w", errno)}
			case 2:
				return serverConn, nil
			default:
				return nil, stopErr
			}
		}}
		server := NewServer(WithMilter(func() Milter { return &NoOpMilter{} }))
		defer server.Close()
		done := make(chan error, 1)
		go func() { done <- server.Serve(ln) }()
		synctest.Wait()
		select {
		case err := <-done:
			t.Fatalf("Serve() stopped on a resource error: %v", err)
		default:
		}

		client := NewClient("tcp", "unused")
		session, err := client.session(clientConn, nil)
		if err != nil {
			t.Fatalf("session after resource error: %v", err)
		}
		defer session.Close()
		if _, err := session.Conn("localhost", FamilyInet, 25, "127.0.0.1"); err != nil {
			t.Fatalf("Conn() after resource error: %v", err)
		}
		if err := <-done; err != stopErr {
			t.Fatalf("Serve() = %v, want %v", err, stopErr)
		}
	})
}

func TestServer_AcceptRetryBackoff(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		serverConn, clientConn := net.Pipe()
		defer serverConn.Close()
		_ = clientConn.Close()
		stopErr := errors.New("stop accepting")
		// The delay grows, is capped, and resets after a successful Accept.
		wantDelays := []time.Duration{
			0, 5, 10, 20, 40, 80, 160, 320, 640, 1000, 1000, 0, 5,
		}
		calls := 0
		lastCall := time.Now()
		ln := &acceptFuncListener{accept: func() (net.Conn, error) {
			if calls >= len(wantDelays) {
				t.Fatal("Serve() retried a non-resource error")
			}
			now := time.Now()
			if got, want := now.Sub(lastCall), wantDelays[calls]*time.Millisecond; got != want {
				t.Errorf("Accept call %d: delay = %v, want %v", calls+1, got, want)
			}
			lastCall = now
			calls++
			switch calls {
			case 11:
				return serverConn, nil
			case 13:
				return nil, stopErr
			default:
				return nil, syscall.EMFILE
			}
		}}
		server := NewServer(WithMilter(func() Milter { return &NoOpMilter{} }))
		defer server.Close()
		if err := server.Serve(ln); err != stopErr {
			t.Fatalf("Serve() = %v, want %v", err, stopErr)
		}
		if calls != len(wantDelays) {
			t.Fatalf("Accept calls = %d, want %d", calls, len(wantDelays))
		}
	})
}

func TestServer_AcceptPermanentError(t *testing.T) {
	for _, want := range []error{net.ErrClosed, syscall.EACCES, errors.New("listener failure")} {
		t.Run(want.Error(), func(t *testing.T) {
			calls := 0
			ln := &acceptFuncListener{accept: func() (net.Conn, error) {
				calls++
				if calls > 1 {
					t.Fatal("Serve() retried a non-resource error")
				}
				return nil, want
			}}
			server := NewServer(WithMilter(func() Milter { return &NoOpMilter{} }))
			defer server.Close()
			if err := server.Serve(ln); err != want {
				t.Fatalf("Serve() = %v, want %v", err, want)
			}
		})
	}
}

func TestServer_StopDuringAcceptRetry(t *testing.T) {
	for _, shutdown := range []bool{false, true} {
		name := "Close"
		if shutdown {
			name = "Shutdown"
		}
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ln := &acceptFuncListener{accept: func() (net.Conn, error) {
					return nil, syscall.EMFILE
				}}
				server := NewServer(WithMilter(func() Milter { return &NoOpMilter{} }))
				defer server.Close()
				done := make(chan error, 1)
				go func() { done <- server.Serve(ln) }()
				synctest.Wait()
				select {
				case err := <-done:
					t.Fatalf("Serve() returned before shutdown: %v", err)
				default:
				}

				start := time.Now()
				var err error
				if shutdown {
					ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
					defer cancel()
					err = server.Shutdown(ctx)
				} else {
					err = server.Close()
				}
				if err != nil {
					t.Fatal(err)
				}
				if elapsed := time.Since(start); elapsed != 0 {
					t.Errorf("shutdown waited %v for the retry timer", elapsed)
				}
				if err := <-done; err != nil {
					t.Fatalf("Serve() after shutdown: %v", err)
				}
			})
		})
	}
}
