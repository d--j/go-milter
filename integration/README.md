# go-milter integration tests

## How it works

The integration test runner starts a receiving SMTP server and test milter servers. It then configures different MTAs to
use the test milter servers and send all emails to the receiving SMTP server. When all this is set up and running,
the test runner send the testcases as SMTP transactions to the MTA and checks if the right filter decision at the right
time was made and whether the outgoing SMTP message is as expected.

## Testcases

A testcase is a text file that has three parts: input steps, the expected milter decision (accept, reject etc.) and
optional output data (mail from, header etc.) that gets compared with the actual output of the MTA.

### Input steps

You can omit input steps. Necessary input steps get automatically added to the testcase.

#### `HELO [hello-hostname]`

Sends a HELO/EHLO to the SMTP server

#### `STARTTLS`

Start TLS encryption of connection

#### `AUTH [user1@example.com|user2@example.com]`

Authenticates SMTP connection. There are only two users hard-coded user1@example.com (password `password1`) and user2@example.com (password `password2`).

#### `FROM <addr> args`

Sends a `MAIL FROM` SMTP command.

#### `TO <addr> args`

Sends a `RCPT TO` SMTP command.

#### `RESET`

Sends a `RSET` SMTP command.

#### `HEADER`

Sends the `DATA` SMTP command and then the header. The header to send follows the `HEADER` line. The end of
the header is marked with a single `.` in a line (like in SMTP connections)

#### `BODY`

Sends the body part of the DATA. The end of the body part is also marked with a single `.`.

### `DECISION [decision]@[step]`

Every testcase needs to have a `DECISION`. Valid `decision`s are: `ACCEPT`, `TEMPFAIL`, `REJECT`, `DISCARD-OR-QUARANTINE` and `CUSTOM`.
If you specify `CUSTOM` then the lines after the `DECISION` line get parsed as a SMTP response and the mitler should 
set this SMTP response.

The `step` can be `HELO`, `FROM`, `TO`, `DATA`, `EOM` and `*`. If the step is omitted `*` is assumed.
`*` means that the decision can happen after any step.

### Output

If you specified `ACCEPT` as decision you can add `FROM`, `TO`, `HEADER` and `BODY` lines (see syntax above) after the `DECISION` line.
These values get compared with the actual result the MTA send to our receiving SMTP server.

## How to add integration tests to your go-milter based mail filter

You need docker since the test are run inside a docker container.

Add a Makefile
```makefile
GO_MILTER_INTEGRATION_DIR := $(shell cd integration && go list -f '{{.Dir}}' github.com/d--j/go-milter/integration)

integration:
	docker build -q -t go-milter-integration "$(GO_MILTER_INTEGRATION_DIR)/docker" && \
	docker run --rm -w /usr/src/root/integration -v $(PWD):/usr/src/root go-milter-integration \
	go run github.com/d--j/go-milter/integration/runner -filter '.*' ./tests

.PHONY: integration
```

Add an `integration` directory. Execute the following inside:
```shell
go mod init
go mod edit -require github.com/d--j/go-milter
go mod edit -require github.com/d--j/go-milter/integration
go mod edit -replace $(cd .. && go list '{{.Path}}')=..
mkdir tests
```

Tests consist of a test milter and testcases that get feed into an MTA that is configured to use the test milter.

A test milter can look something like this:

```go
package main

import (
	"context"

	"github.com/d--j/go-milter/integration"
	"github.com/d--j/go-milter/mailfilter"
)

func main() {
	integration.RequiredTags("auth-plain", "auth-no", "tls-starttls", "tls-no")
	integration.Test(func(ctx context.Context, trx mailfilter.Trx) (mailfilter.Decision, error) {
		return mailfilter.CustomErrorResponse(501, "Test"), nil
	}, mailfilter.WithDecisionAt(mailfilter.DecisionAtMailFrom))
}
```

A testcase for this milter would be:
```
DECISION CUSTOM
501 Test
```

## How to handle dynamic data

If your milter is time dependent or relies on external data you can use monkey pathing to make the output of your milter
static. E.g. the following sets a constant time for `time.Now` and mocks the SPF checks of your milter to static values:

```go
package patches

import (
	"net"
	"strings"
	"time"

	"blitiri.com.ar/go/spf"
	"github.com/agiledragon/gomonkey/v2"
)

var ConstantDate = time.Date(2023, time.January, 1, 12, 0, 0, 0, time.UTC)

func Apply() *gomonkey.Patches {
	return gomonkey.
		ApplyFuncReturn(time.Now, ConstantDate).
		ApplyFunc(spf.CheckHostWithSender, func(_ net.IP, helo, sender string, _ ...spf.Option) (spf.Result, error) {
			if strings.HasSuffix(sender, "@example.com") || helo == "example.com" {
				return spf.Pass, nil
			}
			if strings.HasSuffix(sender, "@example.net") || helo == "example.net" {
				return spf.Fail, nil
			}
			return spf.None, nil
		})
}
```

The `Received` line that the MTA add contains dynamic data (date, queue id). Your test milter will see this dynamic header,
but before comparing the SMTP message with the testcase output data the test runner replaces the first 
`Recieved` header with the static header `Received: placeholder`.
