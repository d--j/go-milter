module github.com/d--j/go-milter/integration

go 1.22.0
toolchain go1.23.4

require (
	github.com/d--j/go-milter v0.8.4
	github.com/emersion/go-message v0.18.1
	github.com/emersion/go-sasl v0.0.0-20241020182733-b788ff22d5a6
	github.com/emersion/go-smtp v0.21.3
	golang.org/x/text v0.22.0
	golang.org/x/tools v0.29.0
)

require golang.org/x/net v0.36.0 // indirect

replace github.com/d--j/go-milter => ../
