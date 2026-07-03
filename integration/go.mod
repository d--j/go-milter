module github.com/d--j/go-milter/integration

go 1.25.0

require (
	github.com/d--j/go-milter v0.10.1
	github.com/emersion/go-message v0.18.2
	github.com/emersion/go-sasl v0.0.0-20241020182733-b788ff22d5a6
	github.com/emersion/go-smtp v0.24.0
	golang.org/x/text v0.37.0
	golang.org/x/tools v0.44.0
)

require golang.org/x/net v0.55.0 // indirect

replace github.com/d--j/go-milter => ../
