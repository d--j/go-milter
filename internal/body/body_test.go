package body

import (
	"bytes"
	"io"
	"os"
	"testing"
)

func getBody(maxMem int, data []byte) *Body {
	b := New(maxMem)
	_, _ = b.Write(data)
	return b
}

func TestBody_Close(t *testing.T) {
	fileAlreadyRemoved := getBody(2, []byte("test"))
	_ = os.Remove(fileAlreadyRemoved.file.Name())
	tests := []struct {
		name    string
		body    *Body
		wantErr bool
	}{
		{"noop", getBody(10, nil), false},
		{"mem", getBody(10, []byte("test")), false},
		{"file", getBody(2, []byte("test")), false},
		{"file-already-removed", fileAlreadyRemoved, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.body.Close(); (err != nil) != tt.wantErr {
				t.Errorf("Close() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestBody(t *testing.T) {
	t.Run("mem", func(t *testing.T) {
		b := getBody(10, []byte("test"))
		defer b.Close()
		_, err := b.Write([]byte("test"))
		if err != nil {
			t.Fatal("b.Write got error", err)
		}
		if b.file != nil {
			t.Fatal("b.file needs to be nil")
		}
		var buf [10]byte
		n, err := b.Read(buf[:])
		if err != nil {
			t.Fatal("b.Read got error", err)
		}
		if !bytes.Equal([]byte("testtest"), buf[:n]) {
			t.Fatalf("b.Read got %q expected %q", buf[:n], []byte("testtest"))
		}
		pos, err := b.Seek(0, io.SeekStart)
		if err != nil {
			t.Fatal("b.Seek got error", err)
		}
		if pos != 0 {
			t.Fatal("b.Seek got pos", pos)
		}
		n, err = b.Read(buf[:])
		if err != nil {
			t.Fatal("b.Read got error", err)
		}
		if !bytes.Equal([]byte("testtest"), buf[:n]) {
			t.Fatalf("b.Read got %q expected %q", buf[:n], []byte("testtest"))
		}
	})
	t.Run("file", func(t *testing.T) {
		b := getBody(2, []byte("test"))
		defer func() {
			if b != nil {
				b.Close()
			}
		}()
		if b.file == nil {
			t.Fatal("b.file is nil")
		}
		_, err := b.Write([]byte("test"))
		if err != nil {
			t.Fatal("b.Write got error", err)
		}
		var buf [10]byte
		n, err := b.Read(buf[:])
		if err != nil {
			t.Fatal("b.Read got error", err)
		}
		if !bytes.Equal([]byte("testtest"), buf[:n]) {
			t.Fatalf("b.Read got %q expected %q", buf[:n], []byte("testtest"))
		}
		pos, err := b.Seek(0, io.SeekStart)
		if err != nil {
			t.Fatal("b.Seek got error", err)
		}
		if pos != 0 {
			t.Fatal("b.Seek got pos", pos)
		}
		n, err = b.Read(buf[:])
		if err != nil {
			t.Fatal("b.Read got error", err)
		}
		if !bytes.Equal([]byte("testtest"), buf[:n]) {
			t.Fatalf("b.Read got %q expected %q", buf[:n], []byte("testtest"))
		}
		name := b.file.Name()
		err = b.Close()
		b = nil
		if err != nil {
			t.Fatal("b.Close got error", err)
		}
		_, err = os.Stat(name)
		if err == nil || !os.IsNotExist(err) {
			t.Fatalf("got %v expected to not find file", err)
		}
	})
	t.Run("panic on Write after Read", func(t *testing.T) {
		defer func() { _ = recover() }()
		b := getBody(10, []byte("test"))
		var buf [10]byte
		_, _ = b.Read(buf[:])
		_, _ = b.Write([]byte("test"))
		t.Errorf("did not panic")
	})
	t.Run("panic on Write after Seek", func(t *testing.T) {
		defer func() { _ = recover() }()
		b := getBody(10, []byte("test"))
		_, _ = b.Seek(0, io.SeekEnd)
		_, _ = b.Write([]byte("test"))
		t.Errorf("did not panic")
	})
	t.Run("temp file fail", func(t *testing.T) {
		tmpdir := os.Getenv("TMPDIR")
		tmp := os.Getenv("TMP")
		_ = os.Setenv("TMPDIR", "/this does not exist")
		_ = os.Setenv("TMP", "/this does not exist")
		defer func() {
			_ = os.Setenv("TMPDIR", tmpdir)
			_ = os.Setenv("TMP", tmp)
		}()
		b := getBody(6, []byte("test"))
		_, err := b.Write([]byte("test"))
		if err == nil {
			_ = b.Close()
			t.Fatal("b.Write got nil error")
		}
	})
	t.Run("file close fail", func(t *testing.T) {
		b := getBody(2, []byte("test"))
		_ = b.file.Close()
		err := b.Close()
		if err == nil {
			t.Fatal("b.Close got nil error")
		}
	})
}
