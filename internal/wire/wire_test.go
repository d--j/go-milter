package wire

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"reflect"
	"runtime"
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
		{"Zero length", args{packets{{[]byte{0, 0, 0, 0}, 0}}, time.Second}, nil, true},
		{"Too big", args{packets{{[]byte{0x20, 0, 0, 1}, 0}}, time.Second}, nil, true},
		{"Short read", args{packets{{[]byte{0, 0, 0, 4, 't', 'e'}, 0}}, time.Second}, nil, true},
		{"Larger than initial buffer", args{packets{{[]byte{0, 2, 0, 0}, 0}, {bytes.Repeat([]byte{'x'}, 128*1024), 0}}, time.Second}, &Message{Code: 'x', Data: bytes.Repeat([]byte{'x'}, 128*1024-1)}, false},
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

func TestAppendUint16(t *testing.T) {
	type args struct {
		dest []byte
		val  uint16
	}
	tests := []struct {
		name string
		args args
		want []byte
	}{
		{"Empty", args{[]byte{}, 0x1234}, []byte{0x12, 0x34}},
		{"Nil", args{nil, 0x1234}, []byte{0x12, 0x34}},
		{"Append", args{[]byte{0, 0}, 0x1234}, []byte{0, 0, 0x12, 0x34}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := AppendUint16(tt.args.dest, tt.args.val); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("AppendUint16() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMessage_MacroCode(t *testing.T) {
	type fields struct {
		Code Code
		Data []byte
	}
	tests := []struct {
		name   string
		fields fields
		want   Code
	}{
		{"macro", fields{Code: CodeMacro, Data: []byte{byte(CodeRcpt)}}, CodeRcpt},
		{"non-macro", fields{Code: CodeRcpt, Data: []byte{}}, CodeRcpt},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &Message{
				Code: tt.fields.Code,
				Data: tt.fields.Data,
			}
			if got := m.MacroCode(); got != tt.want {
				t.Errorf("MacroCode() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestReadPacket_NoUpfrontAllocation(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	go func() {
		// announce the maximum packet size but never send the data
		_, _ = server.Write([]byte{0x20, 0, 0, 0})
		_ = server.Close()
	}()
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	_, err := ReadPacket(client, time.Second)
	runtime.ReadMemStats(&after)
	if err == nil {
		t.Fatal("ReadPacket() expected an error for truncated packet")
	}
	if allocated := after.TotalAlloc - before.TotalAlloc; allocated > 4*initialReadBufferSize {
		t.Errorf("ReadPacket() allocated %d bytes for a packet that never arrived", allocated)
	}
}

func TestReadPacket_BufferMatchesLength(t *testing.T) {
	for _, length := range []int{1, 1000, initialReadBufferSize, initialReadBufferSize + 1, 1024 * 1024} {
		t.Run(fmt.Sprint(length), func(t *testing.T) {
			client, server := net.Pipe()
			defer client.Close()
			go func() {
				_, _ = server.Write([]byte{byte(length >> 24), byte(length >> 16), byte(length >> 8), byte(length)})
				_, _ = server.Write(bytes.Repeat([]byte{'B'}, length))
				_ = server.Close()
			}()
			msg, err := ReadPacket(client, time.Second)
			if err != nil {
				t.Fatalf("ReadPacket() error = %v", err)
			}
			if len(msg.Data) != length-1 || cap(msg.Data) != length-1 {
				t.Errorf("ReadPacket() len(Data) = %d, cap(Data) = %d, want %d", len(msg.Data), cap(msg.Data), length-1)
			}
		})
	}
}

func TestReadPacket_Truncated(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		wantErr error
	}{
		{"no payload", []byte{0, 0, 0, 4}, io.EOF},
		{"partial payload", []byte{0, 0, 0, 4, 't', 'e'}, io.ErrUnexpectedEOF},
		{"partial payload after growing", append([]byte{0, 2, 0, 0}, bytes.Repeat([]byte{'x'}, initialReadBufferSize+1)...), io.ErrUnexpectedEOF},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, server := net.Pipe()
			defer client.Close()
			go func() {
				_, _ = server.Write(tt.data)
				_ = server.Close()
			}()
			_, err := ReadPacket(client, time.Second)
			if err != tt.wantErr {
				t.Errorf("ReadPacket() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestReadPacket_Consecutive(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	big := bytes.Repeat([]byte{'b'}, 3*initialReadBufferSize)
	go func() {
		_, _ = server.Write(append([]byte{0, 3, 0, 0}, big...))
		_, _ = server.Write([]byte{0, 0, 0, 2, 'x', 'y'})
		_ = server.Close()
	}()
	first, err := ReadPacket(client, time.Second)
	if err != nil || first.Code != 'b' || !bytes.Equal(first.Data, big[1:]) {
		t.Fatalf("ReadPacket() first = %v, %v", first, err)
	}
	second, err := ReadPacket(client, time.Second)
	if err != nil || second.Code != 'x' || !bytes.Equal(second.Data, []byte{'y'}) {
		t.Fatalf("ReadPacket() second = %+v, %v", second, err)
	}
}
