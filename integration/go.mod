module github.com/d--j/go-milter/integration

go 1.18

require (
	github.com/d--j/go-milter v0.8.3
	github.com/emersion/go-message v0.17.0
	github.com/emersion/go-sasl v0.0.0-20220912192320-0145f2c60ead
	github.com/emersion/go-smtp v0.18.1
	golang.org/x/text v0.13.0
	golang.org/x/tools v0.13.0
)

require (
	github.com/emersion/go-textwrapper v0.0.0-20200911093747-65d896831594 // indirect
	golang.org/x/net v0.16.0 // indirect
)

replace github.com/d--j/go-milter => ../
