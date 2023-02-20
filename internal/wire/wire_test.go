package wire

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"testing"
	"time"
)

func TestReadPacket(t *testing.T) {
	type packet struct {
		data  []byte
		sleep time.Duration
	}

	type packets []packet

	type args struct {
		data    packets
		timeout time.Duration
	}
	tests := []struct {
		name    string
		args    args
		want    *Message
		wantErr bool
	}{
		{"Error on bogus data", args{packets{{[]byte("bogus"), 0}}, time.Second}, nil, true},
		{"Simple", args{packets{{[]byte{0, 0, 0, 1}, 0}, {[]byte("b"), 0}}, time.Second}, &Message{Code: 'b'}, false},
		{"Timeout", args{packets{{[]byte{0, 0, 0, 1}, 2 * time.Second}, {[]byte("b"), 0}}, time.Second}, nil, true},
		{"Timeout2", args{packets{{[]byte{}, 2 * time.Second}, {[]byte{0, 0, 0, 1, 'b'}, 0}}, time.Second}, nil, true},
		{"With Data", args{packets{{[]byte{0, 0, 0, 4, 't', 'e', 's', 't'}, 0}}, time.Second}, &Message{Code: 't', Data: []byte{'e', 's', 't'}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ltt := tt
			t.Parallel()
			ln, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer ln.Close()
			serverChan := make(chan error)
			// Acceptor
			go func() {
				c, err := ln.Accept()
				if err != nil {
					serverChan <- err
					return
				}
				c.SetDeadline(time.Now().Add(time.Minute)) // Not intended to fire.
				for m := 0; m < len(ltt.args.data); m++ {
					if n, err := c.Write(ltt.args.data[m].data); err != nil || n != len(ltt.args.data[m].data) {
						if err == nil {
							err = fmt.Errorf("expected to write %d bytes but only wrote %d bytes", len(ltt.args.data[m].data), n)
						}
						serverChan <- err
						return
					}
					if ltt.args.data[m].sleep > 0 {
						time.Sleep(ltt.args.data[m].sleep)
					}
				}
				serverChan <- nil
			}()
			conn, err := net.Dial("tcp", ln.Addr().String())
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			got, err := ReadPacket(conn, ltt.args.timeout)
			if (err != nil) != ltt.wantErr {
				t.Errorf("ReadPacket() error = %v, wantErr %v", err, ltt.wantErr)
				return
			}
			if (got == nil && ltt.want != nil) || ((got != nil && ltt.want != nil) && (got.Code != ltt.want.Code || !bytes.Equal(got.Data, ltt.want.Data))) {
				t.Errorf("ReadPacket() got = %+v, want %+v", got, ltt.want)
			}
			if serverErr := <-serverChan; serverErr != nil {
				t.Fatal(serverErr)
			}
		})
	}
}

func TestWritePacket(t *testing.T) {
	type writeOp struct {
		msg      *Message
		onAfter  func(ln net.Listener, conn net.Conn)
		onBefore func(ln net.Listener, conn net.Conn)
	}
	type writeOps []writeOp
	tests := []struct {
		name     string
		writeOps writeOps
		want     []byte
		wantErr  bool
	}{
		{"Single", writeOps{{msg: &Message{Code: 'a'}}}, []byte{0, 0, 0, 1, 'a'}, false},
		{"Single2", writeOps{{msg: &Message{Code: 'a', Data: []byte{'a', 0}}}}, []byte{0, 0, 0, 3, 'a', 'a', 0}, false},
		{"Too big", writeOps{{msg: &Message{Code: 'a', Data: make([]byte, 513*(1024*1024))}}}, nil, true},
		{"Nil msg", writeOps{{msg: nil}}, nil, true},
		{"Multiple", writeOps{{msg: &Message{Code: 'a'}}, {msg: &Message{Code: 'b'}}}, []byte{0, 0, 0, 1, 'a', 0, 0, 0, 1, 'b'}, false},
		{"Multiple close in middle", writeOps{{msg: &Message{Code: 'a'}, onAfter: func(ln net.Listener, conn net.Conn) { _ = conn.Close() }}, {msg: &Message{Code: 'b'}}}, []byte{0, 0, 0, 1, 'a'}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ltt := tt
			t.Parallel()
			ln, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer ln.Close()
			type response struct {
				data []byte
				err  error
			}
			serverChan := make(chan response)
			// Acceptor
			go func() {
				c, err := ln.Accept()
				if err != nil {
					serverChan <- response{err: err}
					return
				}
				c.SetDeadline(time.Now().Add(time.Minute)) // Not intended to fire.
				data, err := io.ReadAll(c)
				if err != nil {
					serverChan <- response{err: err}
					return
				}
				serverChan <- response{data: data}
			}()
			conn, err := net.Dial("tcp", ln.Addr().String())
			if err != nil {
				t.Fatal(err)
			}
			defer func(conn net.Conn) {
				_ = conn.Close()
			}(conn)
			for _, op := range ltt.writeOps {
				if op.onBefore != nil {
					op.onBefore(ln, conn)
				}
				err = WritePacket(conn, op.msg, time.Minute)
				if err != nil {
					break
				}
				if op.onAfter != nil {
					op.onAfter(ln, conn)
				}
			}
			_ = conn.Close()
			if (err != nil) != ltt.wantErr {
				t.Errorf("WritePacket() error = %v, wantErr %v", err, ltt.wantErr)
				resp := <-serverChan
				if resp.err != nil {
					t.Fatal(resp.err)
				}
				t.Errorf("read data %v", resp.data)

				return
			}

			resp := <-serverChan
			if resp.err != nil {
				t.Fatal(resp.err)
			}
			if !bytes.Equal(resp.data, ltt.want) {
				t.Errorf("read data mismatch got = %+v, want %+v", resp.data, ltt.want)
			}
		})
	}
}
