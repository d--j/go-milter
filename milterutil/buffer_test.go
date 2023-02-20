package milterutil_test

import (
	"io"
	"reflect"
	"testing"

	"github.com/d--j/go-milter"
	"github.com/d--j/go-milter/milterutil"
)

func TestFixedBufferScanner(t *testing.T) {
	t.Parallel()
	type args struct {
		bufferSize uint32
		inputs     []string
	}
	tests := []struct {
		name    string
		args    args
		want    []string
		wantErr bool
	}{
		{"empty", args{uint32(milter.DataSize64K), []string{}}, nil, false},
		{"short", args{10, []string{"12345"}}, []string{"12345"}, false},
		{"two-in-one", args{10, []string{"12345678901234567890"}}, []string{"1234567890", "1234567890"}, false},
		{"two-in-three", args{10, []string{"12345", "678901", "234567890"}}, []string{"1234567890", "1234567890"}, false},
		{"one-and-half", args{10, []string{"12345", "678901", "2345"}}, []string{"1234567890", "12345"}, false},
	}
	for _, tt_ := range tests {
		t.Run(tt_.name, func(t *testing.T) {
			tt := tt_
			t.Parallel()
			r, w := io.Pipe()
			go func() {
				for _, s := range tt.args.inputs {
					if _, err := w.Write([]byte(s)); err != nil {
						_ = w.CloseWithError(err)
						return
					}
				}
				_ = w.Close()
			}()
			f := milterutil.GetFixedBufferScanner(tt.args.bufferSize, r)
			defer f.Close()
			var got []string
			for f.Scan() {
				got = append(got, string(f.Bytes()))
			}
			if (f.Err() != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", f.Err(), tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func doFixedBufferScannerBenchmark(b *testing.B, bufferSize uint32, writeSize int, writeCount int) {
	buff := make([]byte, writeSize)
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			r, w := io.Pipe()
			go func() {
				for i := 0; i < writeCount; i++ {
					if _, err := w.Write(buff); err != nil {
						w.CloseWithError(err)
						return
					}
				}
				w.Close()
			}()
			scanner := milterutil.GetFixedBufferScanner(bufferSize, r)
			for scanner.Scan() {
			}
			if scanner.Err() != nil {
				scanner.Close()
				b.Fatal(scanner.Err())
			}
			scanner.Close()
			b.SetBytes(int64(writeSize * writeCount))
		}
	})
}

func BenchmarkGetFixedBufferScanner_64K_1K_4096(b *testing.B) {
	doFixedBufferScannerBenchmark(b, uint32(milter.DataSize64K), 1024, 4096)
}
func BenchmarkGetFixedBufferScanner_64K_4K_1024(b *testing.B) {
	doFixedBufferScannerBenchmark(b, uint32(milter.DataSize64K), 4096, 1024)
}
func BenchmarkGetFixedBufferScanner_64K_8K_512(b *testing.B) {
	doFixedBufferScannerBenchmark(b, uint32(milter.DataSize64K), 8192, 512)
}
func BenchmarkGetFixedBufferScanner_64K_32K_128(b *testing.B) {
	doFixedBufferScannerBenchmark(b, uint32(milter.DataSize64K), 32*1024, 128)
}

func BenchmarkGetFixedBufferScanner_1M_1K_4096(b *testing.B) {
	doFixedBufferScannerBenchmark(b, uint32(milter.DataSize1M), 1024, 4096)
}
func BenchmarkGetFixedBufferScanner_1M_4K_1024(b *testing.B) {
	doFixedBufferScannerBenchmark(b, uint32(milter.DataSize1M), 4096, 1024)
}
func BenchmarkGetFixedBufferScanner_1M_8K_512(b *testing.B) {
	doFixedBufferScannerBenchmark(b, uint32(milter.DataSize1M), 8192, 512)
}
func BenchmarkGetFixedBufferScanner_1M_32K_128(b *testing.B) {
	doFixedBufferScannerBenchmark(b, uint32(milter.DataSize1M), 32*1024, 128)
}
