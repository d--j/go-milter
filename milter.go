// Package milter provides an interface to implement milter mail filters and MTAs that can talk to milter programs.
package milter

// OptAction sets which actions the milter wants to perform.
// Multiple options can be set using a bitmask.
type OptAction uint32

// Set which actions the milter wants to perform.
const (
	OptAddHeader       OptAction = 1 << 0 // SMFIF_ADDHDRS
	OptChangeBody      OptAction = 1 << 1 // SMFIF_CHGBODY / SMFIF_MODBODY
	OptAddRcpt         OptAction = 1 << 2 // SMFIF_ADDRCPT
	OptRemoveRcpt      OptAction = 1 << 3 // SMFIF_DELRCPT
	OptChangeHeader    OptAction = 1 << 4 // SMFIF_CHGHDRS
	OptQuarantine      OptAction = 1 << 5 // SMFIF_QUARANTINE
	OptChangeFrom      OptAction = 1 << 6 // SMFIF_CHGFROM [v6]
	OptAddRcptWithArgs OptAction = 1 << 7 // SMFIF_ADDRCPT_PAR [v6]
	OptSetMacros       OptAction = 1 << 8 // SMFIF_SETSYMLIST [v6]
)

// OptProtocol masks out unwanted parts of the SMTP transaction.
// Multiple options can be set using a bitmask.
type OptProtocol uint32

// The options that the milter can send to the MTA during negotiation to tailor the communication.
const (
	OptNoConnect      OptProtocol = 1 << 0  // MTA does not send connect events. SMFIP_NOCONNECT
	OptNoHelo         OptProtocol = 1 << 1  // MTA does not send HELO/EHLO events. SMFIP_NOHELO
	OptNoMailFrom     OptProtocol = 1 << 2  // MTA does not send MAIL FROM events. SMFIP_NOMAIL
	OptNoRcptTo       OptProtocol = 1 << 3  // MTA does not send RCPT TO events. SMFIP_NORCPT
	OptNoBody         OptProtocol = 1 << 4  // MTA does not send message body data. SMFIP_NOBODY
	OptNoHeaders      OptProtocol = 1 << 5  // MTA does not send message header data. SMFIP_NOHDRS
	OptNoEOH          OptProtocol = 1 << 6  // MTA does not send end of header indication event. SMFIP_NOEOH
	OptNoHeaderReply  OptProtocol = 1 << 7  // Milter does not send a reply to header data. SMFIP_NR_HDR, SMFIP_NOHREPL
	OptNoUnknown      OptProtocol = 1 << 8  // MTA does not send unknown SMTP command events. SMFIP_NOUNKNOWN
	OptNoData         OptProtocol = 1 << 9  // MTA does not send the DATA start event. SMFIP_NODATA
	OptSkip           OptProtocol = 1 << 10 // MTA supports ActSkip. SMFIP_SKIP [v6]
	OptRcptRej        OptProtocol = 1 << 11 // Filter wants rejected RCPTs. SMFIP_RCPT_REJ [v6]
	OptNoConnReply    OptProtocol = 1 << 12 // Milter does not send a reply to connection event. SMFIP_NR_CONN [v6]
	OptNoHeloReply    OptProtocol = 1 << 13 // Milter does not send a reply to HELO/EHLO event. SMFIP_NR_HELO [v6]
	OptNoMailReply    OptProtocol = 1 << 14 // Milter does not send a reply to MAIL FROM event. SMFIP_NR_MAIL [v6]
	OptNoRcptReply    OptProtocol = 1 << 15 // Milter does not send a reply to RCPT TO event. SMFIP_NR_RCPT [v6]
	OptNoDataReply    OptProtocol = 1 << 16 // Milter does not send a reply to DATA start event. SMFIP_NR_DATA [v6]
	OptNoUnknownReply OptProtocol = 1 << 17 // Milter does not send a reply to unknown command event. SMFIP_NR_UNKN [v6]
	OptNoEOHReply     OptProtocol = 1 << 18 // Milter does not send a reply to end of header event. SMFIP_NR_EOH [v6]
	OptNoBodyReply    OptProtocol = 1 << 19 // Milter does not send a reply to body chunk event. SMFIP_NR_BODY [v6]

	// OptHeaderLeadingSpace lets the [Milter] request that the MTA does not swallow a leading space
	// when passing the header value to the milter.
	// Sendmail by default eats one space (not tab) after the colon. So the header line (spaces replaced with _):
	//   Subject:__Test
	// gets transferred as HeaderName "Subject" and HeaderValue "_Test". If the milter
	// sends OptHeaderLeadingSpace to the MTA it requests that it wants the header value as is.
	// So the MTA should send HeaderName "Subject" and HeaderValue "__Test".
	//
	// [Milter] that do e.g. DKIM signing may need the additional space to create valid DKIM signatures.
	//
	// The [Client] and [ClientSession] does not handle this option. It is the responsibility of the MTA to check if the milter
	// asked for this and obey this request. In the simplest case just never swallow the space.
	//
	// SMFIP_HDR_LEADSPC [v6]
	OptHeaderLeadingSpace OptProtocol = 1 << 20
)

const (
	// OptNoReplies combines all protocol flags that define that your milter does not send a reply
	// to the MTA. Use this if your [Milter] only decides at the [Milter.EndOfMessage] handler if the
	// email is acceptable or needs to be rejected.
	OptNoReplies OptProtocol = OptNoHeaderReply | OptNoConnReply | OptNoHeloReply | OptNoMailReply | OptNoRcptReply | OptNoDataReply | OptNoUnknownReply | OptNoEOHReply | OptNoBodyReply
)

const (
	optMds256K  uint32 = 1 << 28                       // SMFIP_MDS_256K
	optMds1M    uint32 = 1 << 29                       // SMFIP_MDS_1M
	optInternal        = optMds256K | optMds1M | 1<<30 // internal flags: only used between MTA and libmilter (bit 28,29,30). SMFI_INTERNAL
	optV2       uint32 = 0x0000007F                    // All flags that v2 defined (bit 0, 1, 2, 3, 4, 5, 6). SMFI_V2_PROT
)

// DataSize defines the maximum data size for milter or MTA to use.
//
// The DataSize does not include the one byte for the command byte.
// Only three sizes are defined in the milter protocol.
type DataSize uint32

const (
	// DataSize64K is 64KB - 1 byte (command-byte). This is the default buffer size.
	DataSize64K DataSize = 1024*64 - 1
	// DataSize256K is 256KB - 1 byte (command-byte)
	DataSize256K DataSize = 1024*256 - 1
	// DataSize1M is 1MB - 1 byte (command-byte)
	DataSize1M DataSize = 1024*1024 - 1
)

type ProtoFamily byte

const (
	FamilyUnknown ProtoFamily = 'U' // SMFIA_UNKNOWN
	FamilyUnix    ProtoFamily = 'L' // SMFIA_UNIX
	FamilyInet    ProtoFamily = '4' // SMFIA_INET
	FamilyInet6   ProtoFamily = '6' // SMFIA_INET6
)
