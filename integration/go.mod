module github.com/d--j/go-milter/integration

go 1.18

require (
	github.com/d--j/go-milter v0.8.4
	github.com/emersion/go-message v0.17.0
	github.com/emersion/go-sasl v0.0.0-20231106173351-e73c9f7bad43
	github.com/emersion/go-smtp v0.20.0
	golang.org/x/text v0.14.0
	golang.org/x/tools v0.16.1
)

require (
	github.com/emersion/go-textwrapper v0.0.0-20200911093747-65d896831594 // indirect
	golang.org/x/net v0.19.0 // indirect
)

replace github.com/d--j/go-milter => ../
