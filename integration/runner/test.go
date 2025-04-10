package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/smtp"
	"net/textproto"
	"os"
	"os/exec"
	"path"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/d--j/go-milter/integration"
)

var ErrTestSkipped = errors.New("test skipped")

type TestDir struct {
	Index      int
	Path       string
	Config     *Config
	MTA        *MTA
	Tests      []*TestCase
	cmd        *exec.Cmd
	wg         sync.WaitGroup
	once       sync.Once
	m          sync.Mutex
	startErr   error
	failedTest bool
}

func (t *TestDir) Start() error {
	p := path.Join(t.Config.ScratchDir, fmt.Sprintf("test-%d", t.Index))
	err := os.Mkdir(p, 0700)
	if err != nil && !os.IsExist(err) {
		return err
	}
	exe := path.Join(p, "test.exe")
	if err := Build(t.Path, exe); err != nil {
		return err
	}
	expectedTransactions := len(t.Tests)
	// mock smtp creates a new connection after STARTTLS
	if t.MTA.HasTag("mta-mock") {
		for _, test := range t.Tests {
			for _, trx := range test.TestCase.Transactions {
				for _, step := range trx.InputSteps {
					if step.What == "STARTTLS" {
						expectedTransactions++
					}
				}
			}
		}
	}
	t.cmd = exec.Command(exe, "-network", "tcp", "-address", fmt.Sprintf(":%d", t.Config.MilterPort), "-tags", strings.Join(t.MTA.tags, " "), "-expected-backends", fmt.Sprintf("%d", expectedTransactions))
	ctx, cancel := context.WithCancel(context.Background())
	t.wg.Add(1)
	go func() {
		b, err := t.cmd.CombinedOutput()
		t.m.Lock()
		t.startErr = err
		failedTest := t.failedTest
		t.m.Unlock()
		failed := !IsExpectedExitErr(err)
		if failed {
			LevelTwoLogger.Printf("DIR %s exit error: %s", t.Path, err.Error())
			t.MarkFailedTest()
		}
		if failed || failedTest {
			LevelTwoLogger.Printf("DIR %s output\n%s", t.Path, b)
		}
		t.wg.Done()
		cancel()
	}()
	time.Sleep(time.Second)
	t.m.Lock()
	err = t.startErr
	t.m.Unlock()
	if err != nil {
		var e *exec.ExitError
		if errors.As(err, &e) {
			if e.ExitCode() == integration.ExitSkip {
				return ErrTestSkipped
			}
		}
		return err
	}
	err = WaitForPort(ctx, t.Config.MilterPort)
	cancel()
	if err != nil {
		t.Stop()
		return err
	}
	return nil
}

func (t *TestDir) Stop() {
	t.once.Do(func() {
		if t.cmd != nil && t.cmd.Process != nil {
			_ = t.cmd.Process.Signal(syscall.SIGTERM)
			t.cmd = nil
			t.wg.Wait()
		}
	})
}

func (t *TestDir) MarkFailedTest() {
	t.m.Lock()
	defer t.m.Unlock()
	t.failedTest = true
	t.MTA.MarkFailedTest()
}

type TestState int

const (
	TestReady TestState = iota
	TestOk
	TestSkipped
	TestFailed
)

type TestCase struct {
	Index      int
	Path       string
	Filename   string
	TestCase   *integration.TestCase
	smtpData   *strings.Builder
	disableLog bool
	Config     *Config
	parent     *TestDir
	State      TestState
	Client     *smtp.Client
}

func (t *TestCase) MarkFailed(format string, v ...any) {
	t.parent.MarkFailedTest()
	t.State = TestFailed
	LevelThreeLogger.Printf(format, v...)
	if t.smtpData != nil {
		LevelThreeLogger.Printf("SMTP transaction:\n%s", t.smtpData.String())
	}
}

func (t *TestCase) MarkSkipped(format string, v ...any) {
	LevelThreeLogger.Printf(format, v...)
	t.State = TestSkipped
}

func (t *TestCase) MarkOk(format string, v ...any) {
	LevelThreeLogger.Printf(format, v...)
	t.State = TestOk
}

type logConn struct {
	conn       net.Conn
	disableLog *bool
	buf        *strings.Builder
}

func (l *logConn) log(dir rune, p []byte, n int, err error) {
	if *l.disableLog {
		return
	}
	l.buf.WriteString(fmt.Sprintf("%c %q\n", dir, string(p[:n])))
	if err != nil {
		l.buf.WriteRune(dir)
		l.buf.WriteRune(' ')
		if errors.Is(err, io.EOF) {
			l.buf.WriteString("EOF")
		} else {
			l.buf.WriteString("ERR: ")
			l.buf.WriteString(err.Error())
		}
		l.buf.WriteRune('\n')
	}
}

func (l *logConn) Read(p []byte) (n int, err error) {
	n, err = l.conn.Read(p)
	l.log('<', p, n, err)
	return
}

func (l *logConn) Write(p []byte) (n int, err error) {
	n, err = l.conn.Write(p)
	l.log('>', p, n, err)
	return
}

func (l *logConn) Close() error {
	return l.conn.Close()
}

func (l *logConn) LocalAddr() net.Addr {
	return l.conn.LocalAddr()
}

func (l *logConn) RemoteAddr() net.Addr {
	return l.conn.RemoteAddr()
}

func (l *logConn) SetDeadline(t time.Time) error {
	return l.conn.SetDeadline(t)
}

func (l *logConn) SetReadDeadline(t time.Time) error {
	return l.conn.SetReadDeadline(t)
}

func (l *logConn) SetWriteDeadline(t time.Time) error {
	return l.conn.SetWriteDeadline(t)
}

var _ net.Conn = (*logConn)(nil)

func (t *TestCase) Connect(port uint16) (err error) {
	hostname := fmt.Sprintf(":%d", port)
	var rawConn net.Conn
	rawConn, err = net.Dial("tcp", hostname)
	if err != nil {
		return err
	}
	t.smtpData = &strings.Builder{}
	t.Client, err = smtp.NewClient(&logConn{rawConn, &t.disableLog, t.smtpData}, hostname)
	return err
}

func (t *TestCase) Quit() {
	_ = t.Client.Quit()
}

func (t *TestCase) Send(steps []*integration.InputStep) (uint16, string, integration.DecisionStep, error) {
	var err error
	var dataWriter io.WriteCloser
	for _, step := range steps {
		switch step.What {
		case "HELO":
			if err := t.Client.Hello(step.Arg); err != nil {
				return smtpErr(err, integration.StepHelo)
			}
		case "STARTTLS":
			// prevent us from logging encrypted binary data
			t.disableLog = true
			err = t.Client.StartTLS(&tls.Config{InsecureSkipVerify: true})
			if err != nil {
				return smtpErr(err, integration.StepAny)
			}
			if _, ok := t.Client.TLSConnectionState(); !ok {
				return 0, "", integration.StepAny, errors.New("could not start TLS connection with STARTTLS")
			}
		case "AUTH":
			password := "password1"
			if step.Arg == "user2@example.com" {
				password = "password2"
			}
			if err := t.Client.Auth(InsecurePlainAuth("", step.Arg, password)); err != nil {
				return smtpErr(err, integration.StepAny)
			}
		case "FROM":
			if err := t.Client.Mail(step.Addr); err != nil {
				return smtpErr(err, integration.StepFrom)
			}
		case "TO":
			if err := t.Client.Rcpt(step.Addr); err != nil {
				// do not stop test when milter rejects a recipient
				var sErr *textproto.Error
				if errors.As(err, &sErr) {
					continue
				}
				return smtpErr(err, integration.StepTo)
			}
		case "RESET":
			if err := t.Client.Reset(); err != nil {
				return smtpErr(err, integration.StepAny)
			}
		case "HEADER":
			dataWriter, err = t.Client.Data()
			if err != nil {
				return smtpErr(err, integration.StepData)
			}
			if _, err := dataWriter.Write(step.Data); err != nil {
				return smtpErr(err, integration.StepAny)
			}
		case "BODY":
			if dataWriter == nil {
				panic("dataWriter is nil")
			}
			if _, err := dataWriter.Write(step.Data); err != nil {
				return smtpErr(err, integration.StepAny)
			}
			if err := dataWriter.Close(); err != nil {
				return smtpErr(err, integration.StepEOM)
			}
			return 250, "OK: queued", integration.StepEOM, nil
		default:
			return 0, "", integration.StepAny, fmt.Errorf("unknown step %s", step.What)
		}
	}
	return 0, "", integration.StepEOM, errors.New("incomplete input sequence")
}

func smtpErr(err error, step integration.DecisionStep) (uint16, string, integration.DecisionStep, error) {
	var sErr *textproto.Error
	if errors.As(err, &sErr) {
		msg := sErr.Msg
		if strings.HasPrefix(msg, "(!!!)") && strings.HasSuffix(msg, "(!!!)") {
			// the mock milter encodes the error message in a special way
			msg = strings.TrimPrefix(msg, "(!!!)")
			msg = strings.TrimSuffix(msg, "(!!!)")
			msg = strings.ReplaceAll(msg, "\\r", "\r")
			msg = strings.ReplaceAll(msg, "\\n", "\n")
			r := textproto.NewReader(bufio.NewReader(strings.NewReader(msg)))
			_, msg, err = r.ReadResponse(sErr.Code)
			if err != nil {
				return uint16(sErr.Code), err.Error(), step, err
			}
		}
		return uint16(sErr.Code), msg, step, nil
	}
	return 0, "", step, err
}

type insecurePlainAuth struct {
	identity, username, password string
}

func InsecurePlainAuth(identity, username, password string) smtp.Auth {
	return &insecurePlainAuth{identity, username, password}
}

func (a *insecurePlainAuth) Start(_ *smtp.ServerInfo) (string, []byte, error) {
	resp := []byte(a.identity + "\x00" + a.username + "\x00" + a.password)
	return "PLAIN", resp, nil
}

func (a *insecurePlainAuth) Next(_ []byte, more bool) ([]byte, error) {
	if more {
		// We've already sent everything.
		return nil, errors.New("unexpected server challenge")
	}
	return nil, nil
}
