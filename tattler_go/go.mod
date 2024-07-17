module tattler_go

go 1.22

replace github.com/tattler-community/tattler-client-go/fscache => ../fscache

require (
	github.com/kataras/golog v0.1.8
	github.com/tattler-community/tattler-client-go/fscache v0.0.0-00010101000000-000000000000
)

require (
	github.com/kataras/pio v0.0.11 // indirect
	golang.org/x/sys v0.4.0 // indirect
	golang.org/x/tools v0.5.1-0.20230111220935-a7f7db3f17fc // indirect
	golang.org/x/tools/cmd/cover v0.1.0-deprecated // indirect
)
