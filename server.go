package milter

import (
	"errors"
	"github.com/hashicorp/go-multierror"
	"net"
	"time"
)

// MaxServerProtocolVersion is the maximum Milter protocol version implemented by the server.
const MaxServerProtocolVersion uint32 = 6

// ErrServerClosed is returned by the [Server]'s [Server.Serve] method after a call to [Server.Close].
var ErrServerClosed = errors.New("milter: server closed")

// Milter is an interface for milter callback handlers.
// You need to implement this interface to create a milter.
// You embed the [NoOpMilter] struct in your own milter implementation to only implement the methods you need.
// One [Milter] will handle one SMTP message/transaction. If the MTA gets multiple messages in one connection,
// we will create a new [Milter] instance for each message.
type Milter interface {
	// Connect is called to provide SMTP connection data for incoming message.
	// Suppress with OptNoConnect.
	//
	// If this method returns an error the error will be logged and the connection will be closed.
	// If there is a [Response] (and we did not negotiate [OptNoConnReply]) this response will be sent before closing the connection.
	Connect(host string, family string, port uint16, addr string, m *Modifier) (*Response, error)

	// Helo is called to process any HELO/EHLO related filters. Suppress with [OptNoHelo].
	//
	// If this method returns an error the error will be logged and the connection will be closed.
	// If there is a [Response] (and we did not negotiate [OptNoHeloReply]) this response will be sent before closing the connection.
	Helo(name string, m *Modifier) (*Response, error)

	// MailFrom is called to process filters on envelope FROM address. Suppress with [OptNoMailFrom].
	//
	// If this method returns an error the error will be logged and the connection will be closed.
	// If there is a [Response] (and we did not negotiate [OptNoMailReply]) this response will be sent before closing the connection.
	MailFrom(from string, esmtpArgs string, m *Modifier) (*Response, error)

	// RcptTo is called to process filters on envelope TO address. Suppress with [OptNoRcptTo].
	//
	// If this method returns an error the error will be logged and the connection will be closed.
	// If there is a [Response] (and we did not negotiate [OptNoRcptReply]) this response will be sent before closing the connection.
	RcptTo(rcptTo string, esmtpArgs string, m *Modifier) (*Response, error)

	// Data is called at the beginning of the DATA command (after all RCPT TO commands). Suppress with [OptNoData].
	//
	// If this method returns an error the error will be logged and the connection will be closed.
	// If there is a [Response] (and we did not negotiate [OptNoDataReply]) this response will be sent before closing the connection.
	Data(m *Modifier) (*Response, error)

	// Header is called once for each header in incoming message. Suppress with [OptNoHeaders].
	//
	// If this method returns an error the error will be logged and the connection will be closed.
	// If there is a [Response] (and we did not negotiate [OptNoHeaderReply]) this response will be sent before closing the connection.
	Header(name string, value string, m *Modifier) (*Response, error)

	// Headers gets called when all message headers have been processed. Suppress with [OptNoEOH].
	//
	// If this method returns an error the error will be logged and the connection will be closed.
	// If there is a [Response] (and we did not negotiate [OptNoEOHReply]) this response will be sent before closing the connection.
	Headers(m *Modifier) (*Response, error)

	// BodyChunk is called to process next message body chunk data (up to 64KB
	// in size). Suppress with [OptNoBody]. If you return [RespSkip] the MTA will stop
	// sending more body chunks. But older MTAs do not support this and in this case there are more calls to BodyChunk.
	// Your code should be able to handle this.
	//
	// If this method returns an error the error will be logged and the connection will be closed.
	// If there is a [Response] (and we did not negotiate [OptNoBodyReply]) this response will be sent before closing the connection.
	BodyChunk(chunk []byte, m *Modifier) (*Response, error)

	// EndOfMessage is called at the end of each message. All changes to message's
	// content & attributes must be done here.
	// The MTA can start over with another message in the same connection but that is handled in a new Milter instance.
	//
	// If this method returns an error the error will be logged and the connection will be closed.
	// If there is a [Response] this response will be sent before closing the connection.
	EndOfMessage(m *Modifier) (*Response, error)

	// Abort is called if the current message has been aborted. All message data
	// should be reset to the state prior to the [Milter.MailFrom] callback. Connection data should be
	// preserved. [Milter.Cleanup] is not called before or after Abort.
	// It is very likely that the next callback will be [Milter.MailFrom] again – the MTA will start over with a new message.
	Abort(m *Modifier) error

	// Unknown is called when the MTA got an unknown command in the SMTP connection.
	//
	// If this method returns an error the error will be logged and the connection will be closed.
	// If there is a [Response] (and we did not negotiate [OptNoUnknownReply]) this response will be sent before closing the connection.
	Unknown(cmd string, m *Modifier) (*Response, error)

	// Cleanup always gets called when the [Milter] is about to be discarded.
	// E.g. because the MTA closed the connection, one SMTP message was successful or there was an error.
	// Your [Milter] needs to keep track on the status of the current mail transaction
	Cleanup()
}

// NoOpMilter is a dummy [Milter] implementation that does nothing.
// You can embed this milter in your own [Milter] implementation, when you only want/need to implement
// some methods of the interface.
type NoOpMilter struct{}

var _ Milter = (*NoOpMilter)(nil)

func (NoOpMilter) Connect(host string, family string, port uint16, addr string, m *Modifier) (*Response, error) {
	return RespContinue, nil
}

func (NoOpMilter) Helo(name string, m *Modifier) (*Response, error) {
	return RespContinue, nil
}

func (NoOpMilter) MailFrom(from string, esmtpArgs string, m *Modifier) (*Response, error) {
	return RespContinue, nil
}

func (NoOpMilter) RcptTo(rcptTo string, esmtpArgs string, m *Modifier) (*Response, error) {
	return RespContinue, nil
}

func (NoOpMilter) Data(m *Modifier) (*Response, error) {
	return RespContinue, nil
}

func (NoOpMilter) Header(name string, value string, m *Modifier) (*Response, error) {
	return RespContinue, nil
}

func (NoOpMilter) Headers(m *Modifier) (*Response, error) {
	return RespContinue, nil
}

func (NoOpMilter) BodyChunk(chunk []byte, m *Modifier) (*Response, error) {
	return RespContinue, nil
}

func (NoOpMilter) EndOfMessage(m *Modifier) (*Response, error) {
	return RespAccept, nil
}

func (NoOpMilter) Abort(_ *Modifier) error {
	return nil
}

func (NoOpMilter) Unknown(cmd string, m *Modifier) (*Response, error) {
	return RespContinue, nil
}

func (NoOpMilter) Cleanup() {
}

// Server is a milter server.
type Server struct {
	options   options
	listeners []net.Listener
	closed    bool
}

// NewServer creates a new milter server.
//
// You need to at least specify the used [Milter] with the option [WithMilter].
// You should also specify the actions your [Milter] will do. Otherwise, you cannot do any message modifications.
// For performance reasons you should disable protocol stages that you do not need with [WithProtocol].
//
// This function will panic when you provide invalid options.
func NewServer(opts ...Option) *Server {
	options := options{
		maxVersion:   MaxServerProtocolVersion,
		actions:      0,
		protocol:     0,
		readTimeout:  10 * time.Second,
		writeTimeout: 10 * time.Second,
	}
	if len(opts) > 0 {
		for _, o := range opts {
			if o != nil {
				o(&options)
			}
		}
	}

	if options.newMilter == nil {
		panic("milter: you need to use WithMilter in NewServer call")
	}
	if options.maxVersion > MaxServerProtocolVersion || options.maxVersion < 2 {
		panic("milter: this library cannot handle this milter version")
	}
	if options.dialer != nil {
		panic("milter: WithDialer is a client only option")
	}
	if options.offeredMaxData > 0 {
		panic("milter: WithOfferedMaxData is a client only option")
	}
	if options.macrosByStage != nil {
		options.actions = options.actions | OptSetMacros
	}

	return &Server{options: options}
}

// Serve starts the server.
// You can call this function multiple times to serve on multiple listeners.
func (s *Server) Serve(ln net.Listener) error {
	s.listeners = append(s.listeners, ln)
	defer func(ln net.Listener, len int) {
		if s.listeners[len-1] != nil {
			_ = ln.Close()
			s.listeners[len-1] = nil
		}
	}(ln, len(s.listeners))

	for {
		conn, err := ln.Accept()
		if err != nil {
			if s.closed {
				return ErrServerClosed
			}
			return err
		}

		session := serverSession{
			server:   s,
			version:  s.options.maxVersion,
			actions:  s.options.actions,
			protocol: s.options.protocol,
			conn:     conn,
			macros:   newMacroStages(),
		}
		go session.HandleMilterCommands()
	}
}

// Close closes the server and all its listeners.
// It returns ErrServerClosed if the server is already closed.
func (s *Server) Close() error {
	if s.closed {
		return ErrServerClosed
	}
	s.closed = true
	var result *multierror.Error
	for _, ln := range s.listeners {
		if ln != nil {
			if err := ln.Close(); err != nil {
				result = multierror.Append(result, err)
			}
		}
	}
	return result.ErrorOrNil()
}
