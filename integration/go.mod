module github.com/d--j/go-milter/integration

go 1.18

require (
	github.com/d--j/go-milter v0.6.5
	github.com/emersion/go-message v0.16.0
	github.com/emersion/go-sasl v0.0.0-20200509203442-7bfe0ed36a21
	github.com/emersion/go-smtp v0.16.0
	golang.org/x/text v0.7.0
	golang.org/x/tools v0.1.12
)

require (
	github.com/emersion/go-textwrapper v0.0.0-20200911093747-65d896831594 // indirect
	golang.org/x/net v0.7.0 // indirect
)

replace github.com/d--j/go-milter => ../
