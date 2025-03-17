module github.com/d--j/go-milter/integration

go 1.24.0

require (
	github.com/d--j/go-milter v0.8.5
	github.com/emersion/go-message v0.18.2
	github.com/emersion/go-sasl v0.0.0-20241020182733-b788ff22d5a6
	github.com/emersion/go-smtp v0.21.3
	golang.org/x/text v0.23.0
	golang.org/x/tools v0.31.0
)

require (
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	golang.org/x/net v0.37.0 // indirect
)

replace github.com/d--j/go-milter => ../
