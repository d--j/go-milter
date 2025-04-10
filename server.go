package milter

import (
	"context"
	"errors"
	"math/rand"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// MaxServerProtocolVersion is the maximum Milter protocol version implemented by the server.
const MaxServerProtocolVersion uint32 = 6

// ErrServerClosed is returned by the [Server]'s [Server.Serve] method after a call to [Server.Close].
var ErrServerClosed = errors.New("milter: server closed")

// Milter is an interface for milter callback handlers.
// You need to implement this interface to create a milter.
// You embed the [NoOpMilter] struct in your own milter implementation to only implement the methods you need.
// One [Milter] will handle one MTA milter connection.
// Normally an MTA will create one milter connection for one SMTP connection.
// Your [Milter] implementation needs to be able to handle getting multiple messages in one connection.
// I.e. after you send a terminating response to the MTA
// (RespAccept, RespReject, RespDiscard, RespTempFail, RejectWithCodeAndReason) you need to be able to handle
// getting (a) a new MailFrom command from the MTA for the next message in the same SMTP connection,
// or (b) a call to NewConnection when the MTA re-uses the milter connection for a new SMTP connection.
type Milter interface {
	// NewConnection gets called when a new SMTP connection was opened.
	// You can use this method to initialize your milter.
	// Normally an MTA will use one milter connection only for one SMTP connection so this method will only be called
	// one time. The MTA can also re-use a milter connection for another SMTP connection (wire.CodeQuitNewConn).
	//
	// NewConnection might be called without ever actually calling the other
	// methods of the [Milter] interface (Connect, Helo). Then Cleanup gets called immediately after NewConnection.
	//
	// You can use m to examine the milter protocol options/actions that were negotiated with the MTA.
	// If you return an error, the milter connection will break. If you want to send a response to the SMTP client
	// from this command (including calling Modifier.Progress), you need to defer the response to
	// the next command (Connect, Helo, MailFrom) that gets called.
	NewConnection(m Modifier) error

	// Connect is called to provide SMTP connection data for incoming message.
	// Suppress with OptNoConnect.
	//
	// m is read-only+progress. You can call Modifier.Progress on it but no
	// other modifier actions (e.g. Modifier.AddHeader).
	//
	// If this method returns an error the error will be logged and the connection will be closed.
	// If there is a [Response] (and we did not negotiate [OptNoConnReply]) this response will be sent before closing the connection.
	Connect(host string, family string, port uint16, addr string, m Modifier) (*Response, error)

	// Helo is called to process any HELO/EHLO related filters. Suppress with [OptNoHelo].
	//
	// name is the hostname that the STMP client provided.
	//
	// m is read-only+progress. You can call Modifier.Progress on it but no
	// other modifier actions (e.g. Modifier.AddHeader).
	//
	// You can get multiple Helo calls in one connection.
	// This normally means that the SMTP client issued a STARTTLS command.
	//
	// If this method returns an error the error will be logged and the connection will be closed.
	// If there is a [Response] (and we did not negotiate [OptNoHeloReply]) this response will be sent before closing the connection.
	Helo(name string, m Modifier) (*Response, error)

	// MailFrom is called to process filters on envelope FROM address. Suppress with [OptNoMailFrom].
	//
	// from is the from address/envelope sender of the message (without <>).
	// esmtpArgs are the ESMTP arguments that were passed in the MAIL FROM command.
	//
	// m is read-only+progress. You can call Modifier.Progress on it but no
	// other modifier actions (e.g. Modifier.AddHeader).
	//
	// If this method returns an error the error will be logged and the connection will be closed.
	// If there is a [Response] (and we did not negotiate [OptNoMailReply]) this response will be sent before closing the connection.
	MailFrom(from string, esmtpArgs string, m Modifier) (*Response, error)

	// RcptTo is called to process filters on envelope TO addresses. Suppress with [OptNoRcptTo].
	// It may be called multiple times.
	//
	// rcptTo is the current recipient address (without <>).
	// esmtpArgs are the ESMTP arguments that were passed in the RCPT TO command.
	//
	// m is read-only+progress. You can call Modifier.Progress on it but no
	// other modifier actions (e.g. Modifier.AddHeader).
	//
	// The Response you return from this method determines the action the MTA will take for this recipient only.
	//   - RespAccept: accept this recipient
	//   - RespReject: reject this recipient (5xx error code, permanent error)
	//   - RespTempFail: reject this recipient (4xx error code, temporary error)
	//   - RejectWithCodeAndReason: reject this recipient (4xx or 5xx error code) with a custom error message
	//   - RespContinue: accept this recipient (the same as RespAccept)
	//   - RespSkip: accept this recipient but do not send more RCPT TO events to the milter.
	//     This response is only available in version 6 Milter protocol connections.
	//   - RespDiscard: discards the whole message/transaction for all recipients.
	//     This is the only Response that will affect the whole message, not only this recipient.
	//
	// If this method returns an error the error will be logged and the connection will be closed.
	// If there is a [Response] (and we did not negotiate [OptNoRcptReply]) this response will be sent before closing the connection.
	RcptTo(rcptTo string, esmtpArgs string, m Modifier) (*Response, error)

	// Data is called at the beginning of the DATA command (after all RCPT TO commands).
	// Suppress with [OptNoData].
	//
	// m is read-only+progress. You can call Modifier.Progress on it but no
	// other modifier actions (e.g. Modifier.AddHeader).
	//
	// If this method returns an error the error will be logged and the connection will be closed.
	// If there is a [Response] (and we did not negotiate [OptNoDataReply]) this response will be sent before closing the connection.
	Data(m Modifier) (*Response, error)

	// Header is called once for each header in incoming message. Suppress with [OptNoHeaders].
	//
	// name is the header name (without ":").
	// value is the header value (everything after ":", without the terminating "\r\n").
	//  Depending on OptHeaderLeadingSpace the MTA might have swallowed the first space after the colon.
	//
	// m is read-only+progress. You can call Modifier.Progress on it but no
	// other modifier actions (e.g. Modifier.AddHeader).
	//
	// If you return [RespSkip] the MTA will stop sending more Header events.
	// This response is only available in version 6 Milter protocol connections.
	//
	// If this method returns an error the error will be logged and the connection will be closed.
	// If there is a [Response] (and we did not negotiate [OptNoHeaderReply]) this response will be sent before closing the connection.
	Header(name string, value string, m Modifier) (*Response, error)

	// Headers gets called when all message headers have been processed. Suppress with [OptNoEOH].
	//
	// m is read-only+progress. You can call Modifier.Progress on it but no
	// other modifier actions (e.g. Modifier.AddHeader).
	//
	// If this method returns an error the error will be logged and the connection will be closed.
	// If there is a [Response] (and we did not negotiate [OptNoEOHReply]) this response will be sent before closing the connection.
	Headers(m Modifier) (*Response, error)

	// BodyChunk is called to process next message body chunk data (up to 64KB
	// in size). Suppress with [OptNoBody].
	//
	// m is read-only+progress. You can call Modifier.Progress on it but no
	// other modifier actions (e.g. Modifier.AddHeader).
	//
	// If you return [RespSkip] the MTA will stop sending more BodyChunk events.
	// This response is only available in version 6 Milter protocol connections.
	//
	// If this method returns an error the error will be logged and the connection will be closed.
	// If there is a [Response] (and we did not negotiate [OptNoBodyReply]) this response will be sent before closing the connection.
	BodyChunk(chunk []byte, m Modifier) (*Response, error)

	// EndOfMessage is called at the end of each message. All changes to message's
	// content & attributes must be done here.
	//
	// m is read-write. You can call modifiers (e.g. Modifier.AddHeader) and Modifier.Progress on it.
	//
	// If this method returns an error the error will be logged and the connection will be closed.
	// If there is a [Response] this response will be sent before closing the connection.
	EndOfMessage(m Modifier) (*Response, error)

	// Abort is called if the current message has been aborted. All message data
	// should be reset to the state prior to the [Milter.MailFrom] callback. Connection data should be
	// preserved. [Milter.Cleanup] is not called before or after Abort.
	//
	// It is very likely that the next callback will be [Milter.MailFrom] again –
	// the MTA will start over with a new message.
	// Some MTAs also call Abort (possibly multiple times) after EndOfMessage.
	//
	// m is read-only. You cannot call any modifiers on it (including Modifier.Progress).
	Abort(m Modifier) error

	// Unknown is called when the MTA got an unknown command in the SMTP connection.
	//
	// If this method returns an error the error will be logged and the connection will be closed.
	// If there is a [Response] (and we did not negotiate [OptNoUnknownReply]) this response will be sent before closing the connection.
	Unknown(cmd string, m Modifier) (*Response, error)

	// Cleanup always gets called when the [Milter] is about to be discarded.
	// A Milter gets discarded when the network connection to the MTA was closed.
	//
	// m is read-only. You cannot call any modifiers on it (including Modifier.Progress).
	Cleanup(m Modifier)
}

// NoOpMilter is a dummy [Milter] implementation that does nothing.
// You can embed this milter in your own [Milter] implementation, when you only want/need to implement
// some methods of the interface.
type NoOpMilter struct{}

var _ Milter = (*NoOpMilter)(nil)

func (NoOpMilter) NewConnection(m Modifier) error {
	return nil
}

func (NoOpMilter) Connect(host string, family string, port uint16, addr string, m Modifier) (*Response, error) {
	return RespContinue, nil
}

func (NoOpMilter) Helo(name string, m Modifier) (*Response, error) {
	return RespContinue, nil
}

func (NoOpMilter) MailFrom(from string, esmtpArgs string, m Modifier) (*Response, error) {
	return RespContinue, nil
}

func (NoOpMilter) RcptTo(rcptTo string, esmtpArgs string, m Modifier) (*Response, error) {
	if m.Protocol()&OptSkip != 0 {
		return RespSkip, nil
	}
	return RespContinue, nil
}

func (NoOpMilter) Data(m Modifier) (*Response, error) {
	return RespContinue, nil
}

func (NoOpMilter) Header(name string, value string, m Modifier) (*Response, error) {
	if m.Protocol()&OptSkip != 0 {
		return RespSkip, nil
	}
	return RespContinue, nil
}

func (NoOpMilter) Headers(m Modifier) (*Response, error) {
	return RespContinue, nil
}

func (NoOpMilter) BodyChunk(chunk []byte, m Modifier) (*Response, error) {
	if m.Protocol()&OptSkip != 0 {
		return RespSkip, nil
	}
	return RespContinue, nil
}

func (NoOpMilter) EndOfMessage(m Modifier) (*Response, error) {
	return RespAccept, nil
}

func (NoOpMilter) Abort(_ Modifier) error {
	return nil
}

func (NoOpMilter) Unknown(cmd string, m Modifier) (*Response, error) {
	return RespContinue, nil
}

func (NoOpMilter) Cleanup(m Modifier) {
}

// Server is a milter server.
type Server struct {
	options        options
	listeners      map[*net.Listener]struct{}
	listenerGroup  sync.WaitGroup
	activeSessions map[*serverSession]struct{}
	mu             sync.Mutex
	inShutdown     atomic.Bool
	milterCount    atomic.Uint64
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
		readTimeout:  0,
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

// onceCloseListener wraps a net.Listener, protecting it from multiple Close calls.
type onceCloseListener struct {
	net.Listener
	once     sync.Once
	closeErr error
}

func (oc *onceCloseListener) Close() error {
	oc.once.Do(oc.close)
	return oc.closeErr
}

func (oc *onceCloseListener) close() { oc.closeErr = oc.Listener.Close() }

// Serve starts the server.
// You can call this function multiple times to serve on multiple listeners.
// The server will accept connections until it is closed or shutdown.
// The function will return ErrServerClosed when the server is closed.
func (s *Server) Serve(ln net.Listener) error {
	localLn := &onceCloseListener{Listener: ln}
	if !s.trackListener(localLn, true) {
		return ErrServerClosed
	}
	defer s.trackListener(localLn, false)

	for {
		conn, err := localLn.Accept()
		if err != nil {
			if s.shuttingDown() {
				return nil
			}
			return err
		}
		go func(conn net.Conn) {
			session := serverSession{}
			if !s.trackSession(&session, true) {
				_ = conn.Close()
				return
			}
			session.init(s, conn, s.options.maxVersion, s.options.actions, s.options.protocol)
			session.HandleMilterCommands()
			s.trackSession(&session, false)
		}(conn)
	}
}

// MilterCount returns the number of milter backends that this server created in total.
// A Milter instance gets created for each new connection from the MTA (after successful negotiation).
// Use this function for logging purposes.
func (s *Server) MilterCount() uint64 {
	return s.milterCount.Load()
}

func (s *Server) newMilter(version uint32, action OptAction, protocol OptProtocol, maxData DataSize) (Milter, uint64) {
	return s.options.newMilter(version, action, protocol, maxData), s.milterCount.Add(1)
}

// closeListenersLocked closes all listeners.
// It returns all errors that occurred while closing the listeners.
func (s *Server) closeListenersLocked() error {
	var errs []error
	for ln := range s.listeners {
		errs = append(errs, (*ln).Close())
	}
	s.listeners = nil
	return errors.Join(errs...)
}

// closeActiveSessionsLocked forcefully closes all net.Conn objects of active sessions
func (s *Server) closeActiveSessionsLocked() {
	for sess := range s.activeSessions {
		sess.closeConn()
	}
	s.activeSessions = nil
}

// Close closes the server and all its listeners.
func (s *Server) Close() error {
	s.inShutdown.Store(true)
	s.mu.Lock()
	defer s.mu.Unlock()
	err := s.closeListenersLocked()
	s.mu.Unlock()
	s.listenerGroup.Wait()
	s.mu.Lock()
	s.closeActiveSessionsLocked()
	return err
}

func (s *Server) shuttingDown() bool {
	return s.inShutdown.Load()
}

const shutdownPollIntervalMax = 500 * time.Millisecond

// Shutdown stops the server gracefully.
func (s *Server) Shutdown(ctx context.Context) error {
	s.inShutdown.Store(true)
	s.mu.Lock()
	lnerr := s.closeListenersLocked()
	s.mu.Unlock()
	s.listenerGroup.Wait()

	pollIntervalBase := time.Millisecond
	nextPollInterval := func() time.Duration {
		// Add 10% jitter.
		interval := pollIntervalBase + time.Duration(rand.Intn(int(pollIntervalBase/10)))
		// Double and clamp for next time.
		pollIntervalBase *= 2
		if pollIntervalBase > shutdownPollIntervalMax {
			pollIntervalBase = shutdownPollIntervalMax
		}
		return interval
	}

	timer := time.NewTimer(nextPollInterval())
	defer timer.Stop()
	for {
		s.mu.Lock()
		activeCount := len(s.activeSessions)
		s.mu.Unlock()
		if activeCount == 0 {
			return lnerr
		}
		select {
		case <-ctx.Done():
			s.mu.Lock()
			s.closeActiveSessionsLocked()
			s.mu.Unlock()
			return ctx.Err()
		case <-timer.C:
			timer.Reset(nextPollInterval())
		}
	}
}

func (s *Server) trackListener(ln net.Listener, add bool) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listeners == nil {
		s.listeners = make(map[*net.Listener]struct{})
	}
	if add {
		if s.shuttingDown() {
			return false
		}
		s.listeners[&ln] = struct{}{}
		s.listenerGroup.Add(1)
	} else {
		delete(s.listeners, &ln)
		s.listenerGroup.Done()
	}
	return true
}

func (s *Server) trackSession(c *serverSession, add bool) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.activeSessions == nil {
		s.activeSessions = make(map[*serverSession]struct{})
	}
	if add {
		if s.shuttingDown() {
			return false
		}
		s.activeSessions[c] = struct{}{}
	} else {
		delete(s.activeSessions, c)
	}
	return true
}
