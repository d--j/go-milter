package integration

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/textproto"
	"os"
	"reflect"
	"regexp"
	"strings"
	"time"

	"github.com/d--j/go-milter/milterutil"
	"github.com/emersion/go-message/mail"
	msgTextproto "github.com/emersion/go-message/textproto"
	"github.com/emersion/go-smtp"
	"golang.org/x/text/transform"
)

type AddrArg struct {
	Addr, Arg string
}

func ToAddrArg(addr string, options *smtp.MailOptions) *AddrArg {
	var aa AddrArg
	aa.Addr = addr
	if options == nil {
		return &aa
	}
	var args []string
	if options.Body != "" {
		args = append(args, fmt.Sprintf("BODY=%s", options.Body))
	}
	if options.Size > 0 {
		args = append(args, fmt.Sprintf("SIZE=%d", options.Size))
	}
	if options.UTF8 {
		args = append(args, "SMTPUTF8")
	}
	if options.RequireTLS {
		args = append(args, "REQUIRETLS")
	}
	if options.Auth != nil {
		args = append(args, fmt.Sprintf("AUTH=<%s>", *options.Auth))
	}
	aa.Arg = strings.Join(args, " ")
	return &aa
}

func ToAddrArgRcpt(addr string, options *smtp.RcptOptions) *AddrArg {
	var aa AddrArg
	aa.Addr = addr
	if options == nil {
		return &aa
	}
	var args []string
	if len(options.Notify) > 0 {
		var list []string
		for _, s := range options.Notify {
			list = append(list, string(s))
		}
		args = append(args, fmt.Sprintf("NOTIFY=%s", strings.Join(list, ",")))
	}
	aa.Arg = strings.Join(args, " ")
	return &aa
}

type InputStep struct {
	What      string
	Addr, Arg string
	Data      []byte
}
type DecisionStep int

const (
	StepAny DecisionStep = iota
	StepHelo
	StepFrom
	StepTo
	StepData
	StepEOM
)

func (s DecisionStep) String() string {
	switch s {
	case StepAny:
		return "*"
	case StepHelo:
		return "HELO"
	case StepFrom:
		return "FROM"
	case StepTo:
		return "TO"
	case StepData:
		return "DATA"
	case StepEOM:
		return "EOM"
	}
	return fmt.Sprintf("<invalid step %d>", s)
}

type Decision struct {
	Code    int
	Message *string
	Step    DecisionStep
}

func (d Decision) Compare(code uint16, message string, step DecisionStep) bool {
	if d.Step != StepAny {
		if d.Step != step {
			return false
		}
	}
	if d.Code < 10 {
		return code/100 == uint16(d.Code)
	}
	if d.Code < 100 {
		return code/10 == uint16(d.Code)
	}
	if d.Message != nil {
		return code == uint16(d.Code) && message == *d.Message
	}
	return code == uint16(d.Code)
}

func (d Decision) String() string {
	if d.Code < 10 {
		return fmt.Sprintf("%dxx@%s", d.Code, d.Step)
	}
	if d.Code < 100 {
		return fmt.Sprintf("%dx@%s", d.Code, d.Step)
	}
	if d.Message != nil {
		return fmt.Sprintf("%d %q@%s", d.Code, *d.Message, d.Step)
	}
	return fmt.Sprintf("%d@%s", d.Code, d.Step)
}

type Output struct {
	From         *AddrArg
	To           []*AddrArg
	Header, Body []byte
}

func (o *Output) String() string {
	var b strings.Builder
	if o.From != nil {
		b.WriteString("FROM\n")
		b.WriteString(fmt.Sprintf("<%s> %s\n", o.From.Addr, o.From.Arg))
	}
	if o.To != nil {
		b.WriteString("TO\n")
		for _, t := range o.To {
			b.WriteString(fmt.Sprintf("<%s> %s\n", t.Addr, t.Arg))
		}
	}
	if o.Header != nil {
		b.WriteString("HEADER\n")
		b.WriteString(fmt.Sprintf("%q\n", o.Header))

	}
	if o.Body != nil {
		b.WriteString("BODY\n")
		b.WriteString(fmt.Sprintf("%q\n", o.Body))
	}
	return b.String()
}

type Transaction struct {
	Name       string
	InputSteps []*InputStep
	Decision   *Decision
	Output     *Output
}

func (c *Transaction) ExpectsOutput() bool {
	return c.Output != nil
}

type undoReader struct {
	*textproto.Reader
	peekLine string
	unread   bool
}

func (r *undoReader) ReadLine() (string, error) {
	if r.unread {
		r.unread = false
		return r.peekLine, nil
	}
	line, err := r.Reader.ReadLine()
	r.peekLine = line
	return line, err
}
func (r *undoReader) UnreadLine() {
	r.unread = true
}

func parseTransaction(r *undoReader, skipAutoHelo bool) (*Transaction, error) {
	steps := 0
	var inputs []*InputStep
	var decision *Decision
	var output *Output
loop:
	for {
		line, err := r.ReadLine()
		if err == io.EOF {
			if line != "" {
				return nil, fmt.Errorf("parsing error: dangling %q", line)
			}
			break
		}
		if err != nil {
			return nil, err
		}
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "HELO "):
			if decision != nil {
				return nil, errors.New("HELO after DECISION")
			}
			inputs, steps, err = inputHelo(line[5:], inputs, steps)
			if err != nil {
				return nil, err
			}
		case line == "STARTTLS":
			if decision != nil {
				return nil, errors.New("STARTTLS after DECISION")
			}
			if steps&stepFrom != 0 {
				return nil, errors.New("can only handle STARTTLS as first command after HELO")
			}
			if steps&stepStarttls != 0 {
				return nil, errors.New("multiple STARTTLS are invalid")
			}
			if !skipAutoHelo && steps&stepHelo == 0 {
				inputs, steps, err = inputHelo("", inputs, steps)
				if err != nil {
					return nil, err
				}
			}
			steps = steps | stepStarttls
			inputs = append(inputs, &InputStep{What: "STARTTLS"})
		case strings.HasPrefix(line, "AUTH "):
			if decision != nil {
				return nil, errors.New("AUTH after DECISION")
			}
			if steps&stepAuth != 0 {
				return nil, errors.New("only one AUTH")
			}
			if !skipAutoHelo && steps&stepHelo == 0 {
				inputs, steps, err = inputHelo("", inputs, steps)
				if err != nil {
					return nil, err
				}
			}
			steps = steps | stepAuth
			user := strings.TrimSpace(line[5:])
			switch user {
			case "user1@example.com", "user2@example.com":
				inputs = append(inputs, &InputStep{What: "AUTH", Arg: user})
			default:
				return nil, fmt.Errorf("unknown AUTH user %q", user)
			}
		case strings.HasPrefix(line, "FROM "):
			if decision != nil {
				if output == nil {
					output = &Output{}
				}
				if output.From != nil {
					return nil, errors.New("only one FROM line after DECISION")
				}
				addr, err := parseAddr(line[5:])
				if err != nil {
					return nil, err
				}
				output.From = addr
			} else {
				if !skipAutoHelo && steps&stepHelo == 0 {
					inputs, steps, err = inputHelo("", inputs, steps)
					if err != nil {
						return nil, err
					}
				}
				inputs, steps, err = inputFrom(line[5:], inputs, steps)
				if err != nil {
					return nil, err
				}
			}
		case strings.HasPrefix(line, "TO "):
			if decision != nil {
				if output == nil {
					output = &Output{}
				}
				addr, err := parseAddr(line[3:])
				if err != nil {
					return nil, err
				}
				output.To = append(output.To, addr)
			} else {
				if !skipAutoHelo && steps&stepHelo == 0 {
					inputs, steps, err = inputHelo("", inputs, steps)
					if err != nil {
						return nil, err
					}
				}
				if steps&stepFrom == 0 {
					inputs, steps, err = inputFrom("<from@example.com>", inputs, steps)
					if err != nil {
						return nil, err
					}
				}
				inputs, steps, err = inputRcpt(line[3:], inputs, steps)
				if err != nil {
					return nil, err
				}
			}
		case line == "RESET":
			if decision != nil {
				return nil, errors.New("RESET after DECISION")
			}
			if steps&stepHdr != 0 {
				return nil, errors.New("RESET after HEADER does not make sense")
			}
			// reset step back to before MAIL FROM
			steps = steps &^ stepRcpt &^ stepFrom
			inputs = append(inputs, &InputStep{What: "RESET"})
		case line == "HEADER":
			if decision != nil {
				if output == nil {
					output = &Output{}
				}
				if output.Header != nil {
					return nil, errors.New("only one HEADER line after DECISION")
				}
				output.Header, err = r.ReadDotBytes()
				if err != nil {
					return nil, err
				}
				output.Header = normalizeHeader(output.Header)
			} else {
				if !skipAutoHelo && steps&stepHelo == 0 {
					inputs, steps, err = inputHelo("", inputs, steps)
					if err != nil {
						return nil, err
					}
				}
				if steps&stepFrom == 0 {
					inputs, steps, err = inputFrom("<from@example.com>", inputs, steps)
					if err != nil {
						return nil, err
					}
				}
				if steps&stepRcpt == 0 {
					inputs, steps, err = inputRcpt("<to@example.com>", inputs, steps)
					if err != nil {
						return nil, err
					}
				}
				inputs, steps, err = inputHdr(r, inputs, steps)
				if err != nil {
					return nil, err
				}
			}
		case line == "BODY":
			if decision != nil {
				if output == nil {
					output = &Output{}
				}
				if output.Body != nil {
					return nil, errors.New("only one BODY line after DECISION")
				}
				output.Body, err = r.ReadDotBytes()
				if err != nil {
					return nil, err
				}
				output.Body = normalizeBody(output.Body)
			} else {
				if !skipAutoHelo && steps&stepHelo == 0 {
					inputs, steps, err = inputHelo("", inputs, steps)
					if err != nil {
						return nil, err
					}
				}
				if steps&stepFrom == 0 {
					inputs, steps, err = inputFrom("<from@example.com>", inputs, steps)
					if err != nil {
						return nil, err
					}
				}
				if steps&stepRcpt == 0 {
					inputs, steps, err = inputRcpt("<to@example.com>", inputs, steps)
					if err != nil {
						return nil, err
					}
				}
				if steps&stepHdr == 0 {
					inputs, steps, err = inputHdr(nil, inputs, steps)
					if err != nil {
						return nil, err
					}
				}
				inputs, steps, err = inputBody(r, inputs, steps)
				if err != nil {
					return nil, err
				}
			}
		case strings.HasPrefix(line, "DECISION "):
			if decision != nil {
				return nil, errors.New("only one DECISION line")
			}
			if !skipAutoHelo && steps&stepHelo == 0 {
				inputs, steps, err = inputHelo("", inputs, steps)
				if err != nil {
					return nil, err
				}
			}
			if steps&stepFrom == 0 {
				inputs, steps, err = inputFrom("<from@example.com>", inputs, steps)
				if err != nil {
					return nil, err
				}
			}
			if steps&stepRcpt == 0 {
				inputs, steps, err = inputRcpt("<to@example.com>", inputs, steps)
				if err != nil {
					return nil, err
				}
			}
			if steps&stepHdr == 0 {
				inputs, steps, err = inputHdr(nil, inputs, steps)
				if err != nil {
					return nil, err
				}
			}
			if steps&stepBody == 0 {
				inputs, steps, err = inputBody(nil, inputs, steps)
				if err != nil {
					return nil, err
				}
			}
			decision, err = parseDecision(line[9:], r)
			if err != nil {
				return nil, err
			}
		case strings.HasPrefix(line, "#"):
			r.UnreadLine()
			break loop
		default:
			return nil, fmt.Errorf("parsing error: unknown line %q", line)
		}
	}

	if decision == nil {
		return nil, errors.New("no DECISION line specified")
	}

	return &Transaction{
		InputSteps: inputs,
		Decision:   decision,
		Output:     output,
	}, nil
}

type TestCase struct {
	Transactions []*Transaction
}

func ParseTestCase(filename string) (*TestCase, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer func(f *os.File) {
		_ = f.Close()
	}(f)
	r := undoReader{Reader: textproto.NewReader(bufio.NewReader(f))}
	var transactions []*Transaction
	for {
		line, err := r.ReadLine()
		if err == io.EOF {
			if line != "" {
				return nil, fmt.Errorf("parsing error: dangling %q", line)
			}
			break
		}
		transactionName := ""
		if strings.HasPrefix(line, "#") {
			transactionName = strings.TrimSpace(strings.TrimPrefix(line, "#"))
			if transactionName == "" {
				transactionName = fmt.Sprintf("Transaction #%d", len(transactions)+1)
			}
		} else {
			if len(transactions) > 0 {
				return nil, fmt.Errorf("parsing error: a new transaction must start with a line `# transaction name`")
			}
			r.UnreadLine()
			transactionName = "Transaction #1"
		}
		transaction, err := parseTransaction(&r, len(transactions) > 0)
		if err != nil {
			return nil, fmt.Errorf("parsing %s: %w", transactionName, err)
		}
		transaction.Name = transactionName
		transactions = append(transactions, transaction)
	}

	if len(transactions) == 0 {
		return nil, errors.New("test case is empty")
	}

	return &TestCase{
		Transactions: transactions,
	}, nil
}

var constantDate = time.Date(2023, time.January, 1, 12, 0, 0, 0, time.UTC)

const (
	stepHelo = 1 << iota
	stepStarttls
	stepAuth
	stepFrom
	stepRcpt
	stepHdr
	stepBody
)

func inputHelo(input string, inputs []*InputStep, steps int) ([]*InputStep, int, error) {
	if steps&stepFrom != 0 {
		return nil, steps, errors.New("cannot use HELO after FROM")
	}
	steps = steps | stepHelo
	helo := strings.TrimSpace(input)
	if helo == "" {
		helo = "localhost.local"
	}
	inputs = append(inputs, &InputStep{What: "HELO", Arg: helo})
	return inputs, steps, nil
}

var angleAddr = regexp.MustCompile("^\\s*<(.*?)>\\s*(.*?)\\s*$")

func parseAddr(input string) (*AddrArg, error) {
	matches := angleAddr.FindStringSubmatch(input)
	if matches == nil {
		return nil, fmt.Errorf("could not parse %q", input)
	}
	return &AddrArg{Addr: matches[1], Arg: matches[2]}, nil
}

func inputFrom(input string, inputs []*InputStep, steps int) ([]*InputStep, int, error) {
	if steps&stepFrom != 0 {
		return nil, steps, errors.New("cannot use FROM multiple times")
	}
	steps = steps | stepFrom
	addr, err := parseAddr(input)
	if err != nil {
		return nil, steps, err
	}
	inputs = append(inputs, &InputStep{What: "FROM", Addr: addr.Addr, Arg: addr.Arg})
	return inputs, steps, nil
}

func inputRcpt(input string, inputs []*InputStep, steps int) ([]*InputStep, int, error) {
	if steps&stepHdr != 0 {
		return nil, steps, errors.New("cannot use TO after HEADER, use RESET in-between")
	}
	steps = steps | stepRcpt
	addr, err := parseAddr(input)
	if err != nil {
		return nil, steps, err
	}
	inputs = append(inputs, &InputStep{What: "TO", Addr: addr.Addr, Arg: addr.Arg})
	return inputs, steps, nil
}

func normalizeHeader(in []byte) []byte {
	b, _, err := transform.Bytes(&milterutil.CrLfCanonicalizationTransformer{}, in)
	if err != nil {
		panic(err)
	}
	if len(b) < 4 || !bytes.Equal(b[len(b)-4:], []byte("\r\n\r\n")) {
		b = append(b, '\r', '\n')
	}
	return b
}

func normalizeBody(in []byte) []byte {
	b, _, err := transform.Bytes(&milterutil.CrLfCanonicalizationTransformer{}, in)
	if err != nil {
		panic(err)
	}
	return b
}

func inputHdr(r *undoReader, inputs []*InputStep, steps int) ([]*InputStep, int, error) {
	if steps&stepHdr != 0 {
		return nil, steps, errors.New("no multiple HEADER")
	}
	var b []byte
	var err error
	if r == nil {
		var to []*mail.Address
		for i := len(inputs) - 1; i > -1; i-- {
			if inputs[i].What == "TO" {
				to = append([]*mail.Address{{Address: inputs[i].Addr}}, to...)
			}
			if inputs[i].What == "FROM" {
				hdr := mail.HeaderFromMap(nil)
				hdr.SetMessageID("bogus-msg-id@example.com")
				hdr.SetDate(constantDate)
				hdr.SetText("Subject", "test")
				hdr.SetAddressList("To", to)
				hdr.SetAddressList("From", []*mail.Address{{Address: inputs[i].Addr}})
				buff := bytes.Buffer{}
				err = msgTextproto.WriteHeader(&buff, hdr.Header.Header)
				if err != nil {
					return nil, steps, err
				}
				b = buff.Bytes()
				break
			}
		}
	} else {
		raw, err := r.ReadDotBytes()
		if err != nil {
			return nil, steps, err
		}
		b = normalizeHeader(raw)
	}
	steps = steps | stepHdr
	inputs = append(inputs, &InputStep{What: "HEADER", Data: b})
	return inputs, steps, nil
}

func inputBody(r *undoReader, inputs []*InputStep, steps int) ([]*InputStep, int, error) {
	if steps&stepBody != 0 {
		return nil, steps, errors.New("no multiple BODY")
	}
	var b []byte
	if r == nil {
		b = []byte("a test message")
	} else {
		raw, err := r.ReadDotBytes()
		if err != nil {
			return nil, steps, err
		}
		b, _, err = transform.Bytes(&milterutil.CrLfCanonicalizationTransformer{}, raw)
		if err != nil {
			return nil, steps, err
		}
		b = normalizeBody(b)
	}
	steps = steps | stepBody
	inputs = append(inputs, &InputStep{What: "BODY", Data: b})
	return inputs, steps, nil
}

func parseDecision(decisionStr string, r *undoReader) (*Decision, error) {
	decisionStr = strings.TrimSpace(decisionStr)
	parts := strings.Split(decisionStr, "@")
	if len(parts) > 2 {
		return nil, fmt.Errorf("invalid decision %s", decisionStr)
	}
	at := StepAny
	if len(parts) == 2 {
		switch parts[1] {
		case "HELO":
			at = StepHelo
		case "FROM":
			at = StepFrom
		case "TO":
			at = StepTo
		case "DATA":
			at = StepData
		case "EOM":
			at = StepEOM
		case "*":
			at = StepAny
		default:
			return nil, fmt.Errorf("unkonwn step %s", parts[1])
		}
	}
	switch parts[0] {
	case "ACCEPT", "DISCARD-OR-QUARANTINE":
		if at != StepEOM && at != StepAny {
			return nil, fmt.Errorf("step can only be * or EOM here %s", decisionStr)
		}
		return &Decision{Code: 2, Step: at}, nil
	case "TEMPFAIL":
		return &Decision{Code: 4, Step: at}, nil
	case "REJECT":
		return &Decision{Code: 5, Step: at}, nil
	case "CUSTOM":
		code, message, err := r.ReadResponse(0)
		if err != nil {
			return nil, err
		}
		return &Decision{Code: code, Message: &message, Step: at}, nil
	default:
		return nil, fmt.Errorf("unknown decision %q", decisionStr)
	}
}

func addrEqual(expected, got *AddrArg) bool {
	if expected == nil && got == nil {
		return true
	}
	if (expected == nil) != (got == nil) {
		return false
	}
	//goland:noinspection GoDfaNilDereference
	if expected.Addr != got.Addr {
		return false
	}
	if expected.Arg != "*" && got.Arg != "*" {
		return expected.Addr == got.Arg
	}
	return true
}

func addrsEqual(expected, got []*AddrArg) bool {
	if expected == nil && got == nil {
		return true
	}
	if (expected == nil) != (got == nil) {
		return false
	}
	counter := 0
outer:
	for _, e := range expected {
		for _, g := range got {
			if addrEqual(e, g) {
				counter++
				continue outer
			}
		}
		return false
	}
	return counter == len(got)
}

func DiffOutput(expected, got *Output) (string, bool) {
	if expected == nil && got == nil {
		return "", true
	}
	if expected != nil && got == nil {
		return "got nil output", false
	}
	if expected == nil {
		return "expected nil", false
	}
	var b strings.Builder
	ok := true
	if expected.From != nil && !addrEqual(expected.From, got.From) {
		ok = false
		b.WriteString("FROM\n")
		if expected.From == nil {
			b.WriteString("- [nil]\n")
		} else {
			b.WriteString(fmt.Sprintf("- <%s> %s\n", expected.From.Addr, expected.From.Arg))
		}
		if got.From == nil {
			b.WriteString("+ [nil]\n")
		} else {
			b.WriteString(fmt.Sprintf("+ <%s> %s\n", got.From.Addr, got.From.Arg))
		}
	}
	if expected.To != nil && !addrsEqual(expected.To, got.To) {
		ok = false
		b.WriteString("TO\n")
		if expected.To == nil {
			b.WriteString("- [nil]\n")
		} else {
			for _, t := range expected.To {
				b.WriteString(fmt.Sprintf("- <%s> %s\n", t.Addr, t.Arg))
			}
		}
		if got.To == nil {
			b.WriteString("+ [nil]\n")
		} else {
			for _, t := range got.To {
				b.WriteString(fmt.Sprintf("+ <%s> %s\n", t.Addr, t.Arg))
			}
		}
	}
	if expected.Header != nil && !reflect.DeepEqual(expected.Header, got.Header) {
		ok = false
		b.WriteString("HEADER\n")
		if expected.Header == nil {
			b.WriteString("- [nil]\n")
		} else {
			b.WriteString(fmt.Sprintf("- %q\n", expected.Header))
		}
		if got.Header == nil {
			b.WriteString("+ [nil]\n")
		} else {
			b.WriteString(fmt.Sprintf("+ %q\n", got.Header))
		}
	}
	if expected.Body != nil && !reflect.DeepEqual(expected.Body, got.Body) {
		ok = false
		b.WriteString("BODY\n")
		if expected.Body == nil {
			b.WriteString("- [nil]\n")
		} else {
			b.WriteString(fmt.Sprintf("- %q\n", expected.Body))
		}
		if got.Body == nil {
			b.WriteString("+ [nil]\n")
		} else {
			b.WriteString(fmt.Sprintf("+ %q\n", got.Body))
		}
	}
	return b.String(), ok
}

var unfoldRegex = regexp.MustCompile(`\r?\n\s*`)
var unfoldRegex2 = regexp.MustCompile(` \t`)

func unfold(in []byte) []byte {
	return unfoldRegex2.ReplaceAll(unfoldRegex.ReplaceAll(in, []byte(" ")), []byte(" "))
}

// CompareOutputSendmail is a relaxed compare function that does only check
// that the header values are all there – the order and folding do not matter.
func CompareOutputSendmail(expected, got *Output) bool {
	if expected == nil && got == nil {
		return true
	}
	if expected != nil && got == nil {
		return false
	}
	if expected == nil {
		return false
	}
	if expected.From != nil && !addrEqual(expected.From, got.From) {
		return false
	}
	if expected.To != nil && !addrsEqual(expected.To, got.To) {
		return false
	}
	if expected.Body != nil && !reflect.DeepEqual(expected.Body, got.Body) {
		return false
	}
	if expected.Header != nil {
		r, err := mail.CreateReader(bytes.NewReader(expected.Header))
		if err != nil {
			return false
		}
		exFields := r.Header.Fields()
		r, err = mail.CreateReader(bytes.NewReader(got.Header))
		if err != nil {
			return false
		}
		gotFields := r.Header.Fields()
		if exFields.Len() != gotFields.Len() {
			return false
		}
		expectedLines := make([][]byte, 0, exFields.Len())
		for exFields.Next() {
			b, err := exFields.Raw()
			if err != nil {
				return false
			}
			expectedLines = append(expectedLines, unfold(b[:len(b)-2]))
		}
		gotLines := make([][]byte, 0, gotFields.Len())
		for gotFields.Next() {
			b, err := gotFields.Raw()
			if err != nil {
				return false
			}
			gotLines = append(gotLines, unfold(b[:len(b)-2]))
		}
	outer:
		for _, e := range expectedLines {
			for _, g := range gotLines {
				if bytes.Equal(e, g) {
					continue outer
				}
			}
			return false
		}
	}
	return true
}
