package milter

import (
	"net"
	"reflect"
	"testing"
	"time"
)

type optionsTestCase struct {
	name    string
	start   options
	options []Option
	want    options
}

func testOptions(t *testing.T, tests []optionsTestCase) {
	for _, tt_ := range tests {
		t.Run(tt_.name, func(t *testing.T) {
			tt := tt_
			t.Parallel()
			got := tt.start
			for _, f := range tt.options {
				f(&got)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestWithAction(t *testing.T) {
	testOptions(t, []optionsTestCase{
		{"set", options{}, []Option{WithAction(OptAddHeader)}, options{actions: OptAddHeader}},
		{"add", options{}, []Option{WithAction(OptAddHeader), WithAction(OptQuarantine)}, options{actions: OptAddHeader | OptQuarantine}},
		{"noop", options{actions: OptChangeHeader}, []Option{WithAction(OptChangeHeader)}, options{actions: OptChangeHeader}},
		{"keep", options{actions: OptChangeHeader}, []Option{WithAction(OptAddHeader)}, options{actions: OptChangeHeader | OptAddHeader}},
	})
}

func TestWithoutAction(t *testing.T) {
	testOptions(t, []optionsTestCase{
		{"noop", options{}, []Option{WithoutAction(OptAddHeader)}, options{}},
		{"remove", options{actions: OptAddHeader | OptQuarantine}, []Option{WithoutAction(OptAddHeader)}, options{actions: OptQuarantine}},
	})
}

func TestWithActions(t *testing.T) {
	testOptions(t, []optionsTestCase{
		{"set", options{}, []Option{WithActions(OptAddHeader)}, options{actions: OptAddHeader}},
		{"no-add", options{}, []Option{WithActions(OptAddHeader), WithActions(OptQuarantine)}, options{actions: OptQuarantine}},
		{"noop", options{actions: OptChangeHeader}, []Option{WithActions(OptChangeHeader)}, options{actions: OptChangeHeader}},
		{"remove", options{actions: OptChangeHeader}, []Option{WithActions(OptAddHeader)}, options{actions: OptAddHeader}},
	})
}

func TestWithProtocol(t *testing.T) {
	testOptions(t, []optionsTestCase{
		{"set", options{}, []Option{WithProtocol(OptNoData)}, options{protocol: OptNoData}},
		{"add", options{}, []Option{WithProtocol(OptNoData), WithProtocol(OptNoMailFrom)}, options{protocol: OptNoData | OptNoMailFrom}},
		{"noop", options{protocol: OptNoData}, []Option{WithProtocol(OptNoData)}, options{protocol: OptNoData}},
		{"keep", options{protocol: OptNoData}, []Option{WithProtocol(OptNoMailFrom)}, options{protocol: OptNoData | OptNoMailFrom}},
	})
}

func TestWithoutProtocol(t *testing.T) {
	testOptions(t, []optionsTestCase{
		{"noop", options{}, []Option{WithoutProtocol(OptSkip)}, options{}},
		{"remove", options{protocol: OptSkip | OptNoData}, []Option{WithoutProtocol(OptNoData)}, options{protocol: OptSkip}},
	})
}

func TestWithProtocols(t *testing.T) {
	testOptions(t, []optionsTestCase{
		{"set", options{}, []Option{WithProtocols(OptNoEOH)}, options{protocol: OptNoEOH}},
		{"no-add", options{}, []Option{WithProtocols(OptNoEOH), WithProtocols(OptSkip)}, options{protocol: OptSkip}},
		{"noop", options{protocol: OptNoEOH}, []Option{WithProtocols(OptNoEOH)}, options{protocol: OptNoEOH}},
		{"remove", options{protocol: OptNoEOH}, []Option{WithProtocols(OptSkip)}, options{protocol: OptSkip}},
	})
}

func TestWithMaximumVersion(t *testing.T) {
	testOptions(t, []optionsTestCase{
		{"set", options{}, []Option{WithMaximumVersion(12)}, options{maxVersion: 12}},
	})
}

func TestWithOfferedMaxData(t *testing.T) {
	testOptions(t, []optionsTestCase{
		{"set", options{}, []Option{WithOfferedMaxData(12)}, options{offeredMaxData: 12}},
	})
}

func TestWithUsedMaxData(t *testing.T) {
	testOptions(t, []optionsTestCase{
		{"set", options{}, []Option{WithUsedMaxData(12)}, options{usedMaxData: 12}},
	})
}
func TestWithReadTimeout(t *testing.T) {
	testOptions(t, []optionsTestCase{
		{"set", options{}, []Option{WithReadTimeout(time.Second)}, options{readTimeout: time.Second}},
	})
}

func TestWithWriteTimeout(t *testing.T) {
	testOptions(t, []optionsTestCase{
		{"set", options{}, []Option{WithWriteTimeout(time.Second)}, options{writeTimeout: time.Second}},
	})
}

func TestWithDialer(t *testing.T) {
	testOptions(t, []optionsTestCase{
		{"set", options{}, []Option{WithDialer(&net.Dialer{Timeout: time.Second})}, options{dialer: &net.Dialer{Timeout: time.Second}}},
	})
}

func TestWithMacroRequest(t *testing.T) {
	testOptions(t, []optionsTestCase{
		{"set", options{}, []Option{WithMacroRequest(StageRcpt, []MacroName{MacroRcptAddr})}, options{macrosByStage: macroRequests{nil, nil, nil, []MacroName{MacroRcptAddr}, nil, nil, nil}}},
	})
}

func TestWithoutDefaultMacros(t *testing.T) {
	testOptions(t, []optionsTestCase{
		{"noop", options{}, []Option{WithoutDefaultMacros()}, options{}},
		{"remove", options{macrosByStage: macroRequests{nil, nil, nil, []MacroName{MacroRcptAddr}, nil, nil, nil}}, []Option{WithoutDefaultMacros()}, options{}},
	})
}

func TestWithDynamicMilter(t *testing.T) {
	opt := options{}
	called := false
	WithDynamicMilter(func(uint32, OptAction, OptProtocol, DataSize) Milter {
		called = true
		return nil
	})(&opt)
	if opt.newMilter == nil {
		t.Fatalf("did not set newMilter")
	}
	opt.newMilter(0, 0, 0, 0)
	if !called {
		t.Fatalf("did not set the correct newMilter")
	}
}

func TestWithNegotiationCallback(t *testing.T) {
	opt := options{}
	called := false
	WithNegotiationCallback(func(mtaVersion, milterVersion uint32, mtaActions, milterActions OptAction, mtaProtocol, milterProtocol OptProtocol, offeredDataSize DataSize) (version uint32, actions OptAction, protocol OptProtocol, maxDataSize DataSize, err error) {
		called = true
		return 0, 0, 0, 0, nil
	})(&opt)
	if opt.negotiationCallback == nil {
		t.Fatalf("did not set negotiationCallback")
	}
	_, _, _, _, _ = opt.negotiationCallback(0, 0, 0, 0, 0, 0, 0)
	if !called {
		t.Fatalf("did not set the correct negotiationCallback")
	}
}
