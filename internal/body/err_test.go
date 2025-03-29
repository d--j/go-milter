package body

import (
	"io"
	"testing"
)

func TestErrReader_Read(t *testing.T) {
	type fields struct {
		Err error
	}
	type args struct {
		p []byte
	}
	tests := []struct {
		name   string
		fields fields
		args   args
	}{
		{"Test1", fields{Err: io.ErrUnexpectedEOF}, args{p: []byte{}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := ErrReader{
				Err: tt.fields.Err,
			}
			gotN, err := e.Read(tt.args.p)
			if err == nil {
				t.Errorf("Read() error = %v, wantErr %v", err, true)
				return
			}
			if gotN != 0 {
				t.Errorf("Read() gotN = %v, want %v", gotN, 0)
			}
		})
	}
}
